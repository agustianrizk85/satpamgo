package visitors

import (
	"errors"
	"net/http"
	"strings"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
	"satpam-go/internal/web"
)

type Handler struct {
	repo     *Repository
	authRepo *auth.Repository
}

func NewHandler(repo *Repository, authRepo *auth.Repository) *Handler {
	return &Handler{repo: repo, authRepo: authRepo}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"createdAt", "updatedAt", "placeId", "userId", "nik", "nama"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     strings.TrimSpace(r.URL.Query().Get("placeId")),
		UserID:      strings.TrimSpace(r.URL.Query().Get("userId")),
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load visitors")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	var body struct {
		PlaceID string  `json:"placeId"`
		UserID  string  `json:"userId"`
		NIK     string  `json:"nik"`
		Nama    string  `json:"nama"`
		Tujuan  *string `json:"tujuan"`
		Catatan *string `json:"catatan"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.UserID = strings.TrimSpace(body.UserID)
	body.NIK = strings.TrimSpace(body.NIK)
	body.Nama = strings.TrimSpace(body.Nama)
	if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.UserID) || body.NIK == "" || body.Nama == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, nik, nama are required")
		return
	}
	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, body.PlaceID, nil)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}

	id, err := h.repo.Create(r.Context(), CreateInput{
		PlaceID: body.PlaceID,
		UserID:  body.UserID,
		NIK:     body.NIK,
		Nama:    body.Nama,
		Tujuan:  trimStringPtr(body.Tujuan),
		Catatan: trimStringPtr(body.Catatan),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrPlaceNotFound):
			web.WriteError(w, http.StatusBadRequest, "Place not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create visitor")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("visitorId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid visitorId")
		return
	}
	item, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Visitor not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load visitor")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("visitorId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid visitorId")
		return
	}

	currentItem, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Visitor not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load visitor")
		return
	}

	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, currentItem.PlaceID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}

	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	var input UpdateInput
	if raw, exists := body["placeId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "placeId must be valid uuid")
			return
		}
		value = strings.TrimSpace(value)
		input.PlaceID = &value
	}
	if raw, exists := body["userId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "userId must be valid uuid")
			return
		}
		value = strings.TrimSpace(value)
		input.UserID = &value
	}
	if raw, exists := body["nik"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "nik must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.NIK = &value
	}
	if raw, exists := body["nama"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "nama must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.Nama = &value
	}
	if raw, exists := body["tujuan"]; exists {
		input.Tujuan = parseNullableString(raw)
		if input.Tujuan == nil {
			web.WriteError(w, http.StatusBadRequest, "tujuan must be string or null")
			return
		}
	}
	if raw, exists := body["catatan"]; exists {
		input.Catatan = parseNullableString(raw)
		if input.Catatan == nil {
			web.WriteError(w, http.StatusBadRequest, "catatan must be string or null")
			return
		}
	}

	item, err := h.repo.Update(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrNotFound):
			web.WriteError(w, http.StatusNotFound, "Visitor not found")
		case errors.Is(err, ErrPlaceNotFound):
			web.WriteError(w, http.StatusBadRequest, "Place not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update visitor")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("visitorId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid visitorId")
		return
	}
	currentItem, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Visitor not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load visitor")
		return
	}
	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, currentItem.PlaceID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}
	out, err := h.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Visitor not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete visitor")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseNullableString(raw any) **string {
	if raw == nil {
		var out *string
		return &out
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	valuePtr := &trimmed
	return &valuePtr
}
