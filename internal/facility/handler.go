package facility

import (
	"errors"
	"net/http"
	"strconv"
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

func (h *Handler) requirePlaceAdminAccess(w http.ResponseWriter, r *http.Request, placeID string) (auth.AuthContext, bool) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return auth.AuthContext{}, false
	}
	if auth.IsGlobalAdminRole(current.Role) {
		return current, true
	}
	ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, placeID, []string{auth.PlaceRoleAdmin})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return current, false
	}
	if !ok {
		web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
		return current, false
	}
	return current, true
}

func (h *Handler) getItemPlaceID(r *http.Request, itemID string) (string, error) {
	item, err := h.repo.GetItem(r.Context(), itemID)
	if err != nil {
		return "", err
	}
	spot, err := h.repo.GetSpot(r.Context(), item.SpotID)
	if err != nil {
		if errors.Is(err, ErrSpotNotFound) {
			return "", ErrItemNotFound
		}
		return "", err
	}
	return spot.PlaceID, nil
}

func (h *Handler) ListSpots(w http.ResponseWriter, r *http.Request) {
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
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"createdAt", "updatedAt", "spotCode", "spotName", "isActive", "placeId"}, DefaultSortBy: "createdAt"})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.ListSpots(r.Context(), current.UserID, current.Role, placeID, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load facility spots")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetSpot(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("spotId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return
	}
	item, err := h.repo.GetSpot(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrSpotNotFound) {
			web.WriteError(w, http.StatusNotFound, "Facility spot not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load facility spot")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateSpot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlaceID  string `json:"placeId"`
		SpotCode string `json:"spotCode"`
		SpotName string `json:"spotName"`
		IsActive *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || strings.TrimSpace(body.SpotCode) == "" || strings.TrimSpace(body.SpotName) == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, spotCode, and spotName are required")
		return
	}
	if _, ok := h.requirePlaceAdminAccess(w, r, strings.TrimSpace(body.PlaceID)); !ok {
		return
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	id, err := h.repo.CreateSpot(r.Context(), strings.TrimSpace(body.PlaceID), strings.TrimSpace(body.SpotCode), strings.TrimSpace(body.SpotName), isActive)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to create facility spot", "Facility spot already exists")
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) PatchSpot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("spotId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return
	}
	currentSpot, err := h.repo.GetSpot(r.Context(), id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to load facility spot", "")
		return
	}
	current, ok := h.requirePlaceAdminAccess(w, r, currentSpot.PlaceID)
	if !ok {
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var placeID, spotCode, spotName *string
	var isActive *bool
	if raw, exists := body["placeId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "placeId must be valid uuid")
			return
		}
		v := strings.TrimSpace(value)
		if !auth.IsGlobalAdminRole(current.Role) && v != currentSpot.PlaceID {
			_, ok := h.requirePlaceAdminAccess(w, r, v)
			if !ok {
				return
			}
		}
		placeID = &v
	}
	if raw, exists := body["spotCode"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "spotCode must be string")
			return
		}
		v := strings.TrimSpace(value)
		spotCode = &v
	}
	if raw, exists := body["spotName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "spotName must be string")
			return
		}
		v := strings.TrimSpace(value)
		spotName = &v
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		isActive = &value
	}
	item, err := h.repo.UpdateSpot(r.Context(), id, placeID, spotCode, spotName, isActive)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to update facility spot", "Facility spot already exists")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteSpot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("spotId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return
	}
	currentSpot, err := h.repo.GetSpot(r.Context(), id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to load facility spot", "")
		return
	}
	if _, ok := h.requirePlaceAdminAccess(w, r, currentSpot.PlaceID); !ok {
		return
	}
	out, err := h.repo.DeleteSpot(r.Context(), id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to delete facility spot", "")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	spotID := strings.TrimSpace(r.URL.Query().Get("spotId"))
	if !web.IsUUID(spotID) {
		web.WriteError(w, http.StatusBadRequest, "spotId is required")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"sortNo", "createdAt", "updatedAt", "itemName", "isRequired", "isActive"}, DefaultSortBy: "sortNo", DefaultSortOrder: listquery.SortAsc})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.ListItems(r.Context(), spotID, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load facility items")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("itemId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid itemId")
		return
	}
	item, err := h.repo.GetItem(r.Context(), id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to load facility item", "")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SpotID     string  `json:"spotId"`
		ItemName   string  `json:"itemName"`
		QRToken    *string `json:"qrToken"`
		IsRequired *bool   `json:"isRequired"`
		SortNo     *int    `json:"sortNo"`
		IsActive   *bool   `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.SpotID)) || strings.TrimSpace(body.ItemName) == "" {
		web.WriteError(w, http.StatusBadRequest, "spotId and itemName are required")
		return
	}
	spot, err := h.repo.GetSpot(r.Context(), strings.TrimSpace(body.SpotID))
	if err != nil {
		h.writeFacilityError(w, err, "Failed to load facility spot", "")
		return
	}
	if _, ok := h.requirePlaceAdminAccess(w, r, spot.PlaceID); !ok {
		return
	}
	isRequired := true
	if body.IsRequired != nil {
		isRequired = *body.IsRequired
	}
	sortNo := 1
	if body.SortNo != nil {
		sortNo = *body.SortNo
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	id, err := h.repo.CreateItem(r.Context(), strings.TrimSpace(body.SpotID), strings.TrimSpace(body.ItemName), trimStringPtr(body.QRToken), isRequired, sortNo, isActive)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to create facility item", "Facility item already exists")
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) PatchItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("itemId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid itemId")
		return
	}
	currentPlaceID, err := h.getItemPlaceID(r, id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to load facility item", "")
		return
	}
	current, ok := h.requirePlaceAdminAccess(w, r, currentPlaceID)
	if !ok {
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var spotID, itemName, qrToken *string
	var isRequired, isActive *bool
	var sortNo *int
	if raw, exists := body["spotId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "spotId must be valid uuid")
			return
		}
		v := strings.TrimSpace(value)
		if !auth.IsGlobalAdminRole(current.Role) {
			targetSpot, err := h.repo.GetSpot(r.Context(), v)
			if err != nil {
				h.writeFacilityError(w, err, "Failed to load facility spot", "")
				return
			}
			if targetSpot.PlaceID != currentPlaceID {
				if _, ok := h.requirePlaceAdminAccess(w, r, targetSpot.PlaceID); !ok {
					return
				}
			}
		}
		spotID = &v
	}
	if raw, exists := body["itemName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "itemName must be string")
			return
		}
		v := strings.TrimSpace(value)
		itemName = &v
	}
	if raw, exists := body["qrToken"]; exists {
		if raw == nil {
			qrToken = nil
		} else {
			value, ok := raw.(string)
			if !ok {
				web.WriteError(w, http.StatusBadRequest, "qrToken must be string or null")
				return
			}
			v := strings.TrimSpace(value)
			qrToken = &v
		}
	}
	if raw, exists := body["isRequired"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isRequired must be boolean")
			return
		}
		isRequired = &value
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		isActive = &value
	}
	if raw, exists := body["sortNo"]; exists {
		value, ok := raw.(float64)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "sortNo must be number")
			return
		}
		v := int(value)
		sortNo = &v
	}
	item, err := h.repo.UpdateItem(r.Context(), id, spotID, itemName, qrToken, isRequired, sortNo, isActive)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to update facility item", "Facility item already exists")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("itemId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid itemId")
		return
	}
	currentPlaceID, err := h.getItemPlaceID(r, id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to load facility item", "")
		return
	}
	if _, ok := h.requirePlaceAdminAccess(w, r, currentPlaceID); !ok {
		return
	}
	out, err := h.repo.DeleteItem(r.Context(), id)
	if err != nil {
		h.writeFacilityError(w, err, "Failed to delete facility item", "")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func (h *Handler) ListScans(w http.ResponseWriter, r *http.Request) {
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
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"scannedAt", "submitAt", "createdAt", "updatedAt", "status", "placeId", "spotId", "userId"}, DefaultSortBy: "scannedAt"})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.ListScans(r.Context(), current.UserID, current.Role, placeID, strings.TrimSpace(r.URL.Query().Get("spotId")), strings.TrimSpace(r.URL.Query().Get("userId")), query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load facility scans")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateScan(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	var body struct {
		PlaceID   string  `json:"placeId"`
		SpotID    string  `json:"spotId"`
		ItemID    *string `json:"itemId"`
		UserID    string  `json:"userId"`
		Status    string  `json:"status"`
		Note      *string `json:"note"`
		ScannedAt *string `json:"scannedAt"`
		SubmitAt  *string `json:"submitAt"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	status := normalizeScanStatus(body.Status)
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || !web.IsUUID(strings.TrimSpace(body.SpotID)) || !web.IsUUID(strings.TrimSpace(body.UserID)) || status == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, spotId, userId are required and status is invalid")
		return
	}
	id, err := h.repo.CreateScan(r.Context(), strings.TrimSpace(body.PlaceID), strings.TrimSpace(body.SpotID), trimUUIDPtr(body.ItemID), strings.TrimSpace(body.UserID), status, trimStringPtr(body.Note), trimStringPtr(body.ScannedAt), trimStringPtr(body.SubmitAt))
	if err != nil {
		h.writeFacilityError(w, err, "Failed to create facility scan", "Facility scan already exists")
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return false
	}
	if auth.IsGlobalAdminRole(current.Role) {
		return true
	}
	ok, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{auth.PlaceRoleAdmin})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return false
	}
	if !ok {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return false
	}
	return true
}

func (h *Handler) writeFacilityError(w http.ResponseWriter, err error, fallback, duplicate string) {
	switch {
	case errors.Is(err, ErrSpotNotFound), errors.Is(err, ErrItemNotFound):
		web.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrNoFieldsToUpdate):
		web.WriteError(w, http.StatusBadRequest, "No fields to update")
	case errors.Is(err, ErrAlreadyExists) && duplicate != "":
		web.WriteError(w, http.StatusConflict, duplicate)
	case errors.Is(err, ErrForeignKey):
		web.WriteError(w, http.StatusBadRequest, "Related row not found")
	default:
		web.WriteError(w, http.StatusInternalServerError, fallback)
	}
}

func normalizeScanStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "OK":
		return "OK"
	case "NOT_OK", "PARTIAL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
}

func trimUUIDPtr(value *string) *string {
	value = trimStringPtr(value)
	if value == nil || !web.IsUUID(*value) {
		return nil
	}
	return value
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

var _ = strconv.Itoa
