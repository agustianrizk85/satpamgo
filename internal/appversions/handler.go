package appversions

import (
	"errors"
	"net/http"
	"strings"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
	"satpam-go/internal/web"
)

type Handler struct {
	repo           *Repository
	authRepo       *auth.Repository
	storage        *Storage
	uploadMaxBytes int64
}

func NewHandler(repo *Repository, authRepo *auth.Repository, storage *Storage, uploadMaxBytes int64) *Handler {
	return &Handler{repo: repo, authRepo: authRepo, storage: storage, uploadMaxBytes: uploadMaxBytes}
}

func (h *Handler) requireSuperAdmin(w http.ResponseWriter, r *http.Request) bool {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return false
	}
	if auth.IsSuperUserRole(current.Role) {
		return true
	}
	web.WriteError(w, http.StatusForbidden, "Forbidden: super admin access required")
	return false
}

func (h *Handler) requirePlaceAdminAccess(w http.ResponseWriter, r *http.Request, placeID string) bool {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return false
	}
	if auth.IsGlobalAdminRole(current.Role) {
		return true
	}
	ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, placeID, []string{auth.PlaceRoleAdmin})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return false
	}
	if !ok {
		web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
		return false
	}
	return true
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "placeId is required")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"versionName", "createdAt", "updatedAt", "isActive", "isMandatory", "userId"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	isActiveRaw := strings.TrimSpace(r.URL.Query().Get("isActive"))
	var isActive *bool
	if isActiveRaw != "" {
		value := strings.EqualFold(isActiveRaw, "true")
		isActive = &value
	}
	result, err := h.repo.List(r.Context(), current.UserID, current.Role, placeID, userID, isActive, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load app versions")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("versionId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid versionId")
		return
	}
	item, err := h.repo.Get(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "App version not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load app version")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlaceID     string `json:"placeId"`
		UserID      string `json:"userId"`
		VersionName string `json:"versionName"`
		DownloadURL string `json:"downloadUrl"`
		IsMandatory *bool  `json:"isMandatory"`
		IsActive    *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.UserID = strings.TrimSpace(body.UserID)
	body.VersionName = strings.TrimSpace(body.VersionName)
	body.DownloadURL = strings.TrimSpace(body.DownloadURL)
	if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.UserID) || body.VersionName == "" || body.DownloadURL == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, versionName, and downloadUrl are required")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, body.PlaceID) {
		return
	}
	isMandatory := false
	if body.IsMandatory != nil {
		isMandatory = *body.IsMandatory
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	item, err := h.repo.Create(r.Context(), body.PlaceID, body.UserID, body.VersionName, body.DownloadURL, isMandatory, isActive)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlready):
			web.WriteError(w, http.StatusConflict, "App version already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/user not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create app version")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, item)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("versionId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid versionId")
		return
	}
	item, err := h.repo.Get(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "App version not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load app version")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, item.PlaceID) {
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var versionName, downloadURL *string
	var isMandatory, isActive *bool
	if raw, exists := body["versionName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "versionName must be string")
			return
		}
		v := strings.TrimSpace(value)
		versionName = &v
	}
	if raw, exists := body["downloadUrl"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "downloadUrl must be string")
			return
		}
		v := strings.TrimSpace(value)
		downloadURL = &v
	}
	if raw, exists := body["isMandatory"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isMandatory must be boolean")
			return
		}
		isMandatory = &value
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		isActive = &value
	}
	updated, err := h.repo.Update(r.Context(), id, versionName, downloadURL, isMandatory, isActive)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			web.WriteError(w, http.StatusNotFound, "App version not found")
		case errors.Is(err, ErrAlready):
			web.WriteError(w, http.StatusConflict, "App version already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/user not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update app version")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("versionId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid versionId")
		return
	}
	item, err := h.repo.Get(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "App version not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load app version")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, item.PlaceID) {
		return
	}
	out, err := h.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "App version not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete app version")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	versionName := strings.TrimSpace(r.URL.Query().Get("versionName"))
	if !web.IsUUID(placeID) || !web.IsUUID(userID) || versionName == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, and versionName are required")
		return
	}
	result, err := h.repo.Check(r.Context(), current.UserID, current.Role, placeID, userID, versionName)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to check app version")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CheckMaster(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	versionName := strings.TrimSpace(r.URL.Query().Get("versionName"))
	if versionName == "" {
		web.WriteError(w, http.StatusBadRequest, "versionName is required")
		return
	}
	result, err := h.repo.CheckMaster(r.Context(), versionName)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to check app version master")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) ListMasters(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"versionName", "createdAt", "updatedAt", "isActive", "isMandatory"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	isActiveRaw := strings.TrimSpace(r.URL.Query().Get("isActive"))
	var isActive *bool
	if isActiveRaw != "" {
		value := strings.EqualFold(isActiveRaw, "true")
		isActive = &value
	}
	result, err := h.repo.ListMasters(r.Context(), query, isActive)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load app version masters")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetMaster(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := r.PathValue("masterId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid masterId")
		return
	}
	item, err := h.repo.GetMaster(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrMasterNotFound) {
			web.WriteError(w, http.StatusNotFound, "App version master not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load app version master")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateMaster(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	var body struct {
		VersionName string `json:"versionName"`
		DownloadURL string `json:"downloadUrl"`
		IsMandatory *bool  `json:"isMandatory"`
		IsActive    *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.VersionName = strings.TrimSpace(body.VersionName)
	body.DownloadURL = strings.TrimSpace(body.DownloadURL)
	if body.VersionName == "" || body.DownloadURL == "" {
		web.WriteError(w, http.StatusBadRequest, "versionName and downloadUrl are required")
		return
	}
	isMandatory := false
	if body.IsMandatory != nil {
		isMandatory = *body.IsMandatory
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	item, err := h.repo.CreateMaster(r.Context(), body.VersionName, body.DownloadURL, isMandatory, isActive)
	if err != nil {
		if errors.Is(err, ErrMasterAlready) {
			web.WriteError(w, http.StatusConflict, "App version master already exists")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to create app version master")
		return
	}
	web.WriteJSON(w, http.StatusCreated, item)
}

func (h *Handler) PatchMaster(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := r.PathValue("masterId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid masterId")
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var versionName, downloadURL *string
	var isMandatory, isActive *bool
	if raw, exists := body["versionName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "versionName must be string")
			return
		}
		v := strings.TrimSpace(value)
		versionName = &v
	}
	if raw, exists := body["downloadUrl"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "downloadUrl must be string")
			return
		}
		v := strings.TrimSpace(value)
		downloadURL = &v
	}
	if raw, exists := body["isMandatory"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isMandatory must be boolean")
			return
		}
		isMandatory = &value
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		isActive = &value
	}
	item, err := h.repo.UpdateMaster(r.Context(), id, versionName, downloadURL, isMandatory, isActive)
	if err != nil {
		switch {
		case errors.Is(err, ErrMasterNotFound):
			web.WriteError(w, http.StatusNotFound, "App version master not found")
		case errors.Is(err, ErrMasterAlready):
			web.WriteError(w, http.StatusConflict, "App version master already exists")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update app version master")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteMaster(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}
	id := r.PathValue("masterId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid masterId")
		return
	}
	out, err := h.repo.DeleteMaster(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrMasterNotFound) {
			web.WriteError(w, http.StatusNotFound, "App version master not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete app version master")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func (h *Handler) UploadMaster(w http.ResponseWriter, r *http.Request) {
	if !h.requireSuperAdmin(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.uploadMaxBytes)
	if err := r.ParseMultipartForm(h.uploadMaxBytes); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid multipart body or file too large")
		return
	}

	versionName := strings.TrimSpace(r.FormValue("versionName"))
	if versionName == "" {
		web.WriteError(w, http.StatusBadRequest, "versionName is required")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		web.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	result, err := h.storage.SaveMasterAPK(versionName, header.Filename, file)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidAPK):
			web.WriteError(w, http.StatusBadRequest, "file must be apk")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to store apk file")
		}
		return
	}

	web.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.uploadMaxBytes)
	if err := r.ParseMultipartForm(h.uploadMaxBytes); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid multipart body or file too large")
		return
	}

	placeID := strings.TrimSpace(r.FormValue("placeId"))
	userID := strings.TrimSpace(r.FormValue("userId"))
	versionName := strings.TrimSpace(r.FormValue("versionName"))
	if !web.IsUUID(placeID) || !web.IsUUID(userID) || versionName == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, and versionName are required")
		return
	}

	if !h.requirePlaceAdminAccess(w, r, placeID) {
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		web.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	result, err := h.storage.SaveAPK(placeID, userID, versionName, header.Filename, file)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidAPK):
			web.WriteError(w, http.StatusBadRequest, "file must be apk")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to store apk file")
		}
		return
	}

	web.WriteJSON(w, http.StatusCreated, result)
}
