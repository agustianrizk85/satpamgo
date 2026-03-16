package spots

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
		AllowedSortBy: []string{"createdAt", "updatedAt", "spotCode", "spotName", "status", "placeId"},
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
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load spots")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var body struct {
		PlaceID   string   `json:"placeId"`
		SpotCode  string   `json:"spotCode"`
		SpotName  string   `json:"spotName"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		QRToken   *string  `json:"qrToken"`
		Status    string   `json:"status"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.SpotCode = strings.TrimSpace(body.SpotCode)
	body.SpotName = strings.TrimSpace(body.SpotName)
	status := normalizeStatus(body.Status)
	if !web.IsUUID(body.PlaceID) || body.SpotCode == "" || body.SpotName == "" || status == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, spotCode, spotName are required and status must be ACTIVE or INACTIVE")
		return
	}
	id, err := h.repo.Create(r.Context(), CreateInput{
		PlaceID:   body.PlaceID,
		SpotCode:  body.SpotCode,
		SpotName:  body.SpotName,
		Latitude:  body.Latitude,
		Longitude: body.Longitude,
		QRToken:   trimStringPtr(body.QRToken),
		Status:    status,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Spot already exists")
		case errors.Is(err, ErrPlaceNotFound):
			web.WriteError(w, http.StatusBadRequest, "Place not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create spot")
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
	id := r.PathValue("spotId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return
	}
	item, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Spot not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load spot")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("spotId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return
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
	if raw, exists := body["spotCode"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "spotCode must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.SpotCode = &value
	}
	if raw, exists := body["spotName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "spotName must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.SpotName = &value
	}
	if raw, exists := body["latitude"]; exists {
		if raw == nil {
			var nilValue *float64
			input.Latitude = &nilValue
		} else if value, ok := raw.(float64); ok {
			latitude := value
			valuePtr := &latitude
			input.Latitude = &valuePtr
		} else {
			web.WriteError(w, http.StatusBadRequest, "latitude must be number or null")
			return
		}
	}
	if raw, exists := body["longitude"]; exists {
		if raw == nil {
			var nilValue *float64
			input.Longitude = &nilValue
		} else if value, ok := raw.(float64); ok {
			longitude := value
			valuePtr := &longitude
			input.Longitude = &valuePtr
		} else {
			web.WriteError(w, http.StatusBadRequest, "longitude must be number or null")
			return
		}
	}
	if raw, exists := body["qrToken"]; exists {
		if raw == nil {
			var nilValue *string
			input.QRToken = &nilValue
		} else if value, ok := raw.(string); ok {
			value = strings.TrimSpace(value)
			valuePtr := &value
			input.QRToken = &valuePtr
		} else {
			web.WriteError(w, http.StatusBadRequest, "qrToken must be string or null")
			return
		}
	}
	if raw, exists := body["status"]; exists {
		value, ok := raw.(string)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "status must be string")
			return
		}
		value = normalizeStatus(value)
		if value == "" {
			web.WriteError(w, http.StatusBadRequest, "status must be ACTIVE or INACTIVE")
			return
		}
		input.Status = &value
	}

	item, err := h.repo.Update(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrNotFound):
			web.WriteError(w, http.StatusNotFound, "Spot not found")
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Spot already exists")
		case errors.Is(err, ErrPlaceNotFound):
			web.WriteError(w, http.StatusBadRequest, "Place not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update spot")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("spotId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return
	}
	out, err := h.repo.SoftDelete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Spot not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete spot")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
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

func normalizeStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "":
		return "ACTIVE"
	case "ACTIVE", "INACTIVE":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
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
