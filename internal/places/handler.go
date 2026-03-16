package places

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
		AllowedSortBy: []string{"createdAt", "updatedAt", "placeCode", "placeName", "status"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load places")
		return
	}

	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok || !auth.IsSuperUserRole(current.Role) {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	placeCode, ok := body["placeCode"].(string)
	if !ok || strings.TrimSpace(placeCode) == "" {
		web.WriteError(w, http.StatusBadRequest, "placeCode and placeName are required")
		return
	}
	placeName, ok := body["placeName"].(string)
	if !ok || strings.TrimSpace(placeName) == "" {
		web.WriteError(w, http.StatusBadRequest, "placeCode and placeName are required")
		return
	}

	address, _, valid := optionalString(body, "address")
	if !valid {
		web.WriteError(w, http.StatusBadRequest, "address must be string or null")
		return
	}
	latitude, _, valid := optionalFloat(body, "latitude")
	if !valid {
		web.WriteError(w, http.StatusBadRequest, "latitude must be number or null")
		return
	}
	longitude, _, valid := optionalFloat(body, "longitude")
	if !valid {
		web.WriteError(w, http.StatusBadRequest, "longitude must be number or null")
		return
	}

	status := "ACTIVE"
	if raw, exists := body["status"]; exists {
		value, ok := raw.(string)
		if ok && (value == "ACTIVE" || value == "INACTIVE") {
			status = value
		}
	}

	id, err := h.repo.Create(r.Context(), CreateInput{
		PlaceCode: placeCode,
		PlaceName: placeName,
		Address:   address,
		Latitude:  latitude,
		Longitude: longitude,
		Status:    status,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrPlaceCodeExists):
			web.WriteError(w, http.StatusConflict, "placeCode already exists")
		case errors.Is(err, ErrLatLngOutOfRange):
			web.WriteError(w, http.StatusBadRequest, "latitude/longitude out of range")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create place")
		}
		return
	}

	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	placeID := r.PathValue("placeId")
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
		return
	}

	item, err := h.repo.FindByID(r.Context(), placeID)
	if err != nil {
		if errors.Is(err, ErrPlaceNotFound) {
			web.WriteError(w, http.StatusNotFound, "Place not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to fetch place")
		return
	}

	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, placeID, nil)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}

	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok || !auth.IsSuperUserRole(current.Role) {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	placeID := r.PathValue("placeId")
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
		return
	}

	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	input := UpdateInput{}
	if raw, exists := body["placeCode"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "placeCode must be string")
			return
		}
		input.PlaceCode = &value
	}
	if raw, exists := body["placeName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "placeName must be string")
			return
		}
		input.PlaceName = &value
	}
	if _, exists := body["address"]; exists {
		value, _, valid := optionalString(body, "address")
		if !valid {
			web.WriteError(w, http.StatusBadRequest, "address must be string or null")
			return
		}
		input.Address = &value
	}
	if _, exists := body["latitude"]; exists {
		value, _, valid := optionalFloat(body, "latitude")
		if !valid {
			web.WriteError(w, http.StatusBadRequest, "latitude must be number or null")
			return
		}
		input.Latitude = &value
	}
	if _, exists := body["longitude"]; exists {
		value, _, valid := optionalFloat(body, "longitude")
		if !valid {
			web.WriteError(w, http.StatusBadRequest, "longitude must be number or null")
			return
		}
		input.Longitude = &value
	}
	if raw, exists := body["status"]; exists {
		value, ok := raw.(string)
		if !ok || (value != "ACTIVE" && value != "INACTIVE") {
			web.WriteError(w, http.StatusBadRequest, "status must be ACTIVE or INACTIVE")
			return
		}
		input.Status = &value
	}

	item, err := h.repo.Update(r.Context(), placeID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrPlaceNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrPlaceNotFound):
			web.WriteError(w, http.StatusNotFound, "Place not found")
		case errors.Is(err, ErrPlaceCodeExists):
			web.WriteError(w, http.StatusConflict, "placeCode already exists")
		case errors.Is(err, ErrLatLngOutOfRange):
			web.WriteError(w, http.StatusBadRequest, "latitude/longitude out of range")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update place")
		}
		return
	}

	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok || !auth.IsSuperUserRole(current.Role) {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	placeID := r.PathValue("placeId")
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
		return
	}

	id, err := h.repo.SoftDelete(r.Context(), placeID)
	if err != nil {
		if errors.Is(err, ErrPlaceNotFound) {
			web.WriteError(w, http.StatusNotFound, "Place not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete place")
		return
	}

	web.WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}

func optionalString(body map[string]any, key string) (*string, bool, bool) {
	raw, exists := body[key]
	if !exists {
		return nil, false, true
	}
	if raw == nil {
		return nil, true, true
	}
	value, ok := raw.(string)
	if !ok {
		return nil, false, false
	}
	return &value, true, true
}

func optionalFloat(body map[string]any, key string) (*float64, bool, bool) {
	raw, exists := body[key]
	if !exists {
		return nil, false, true
	}
	if raw == nil {
		return nil, true, true
	}
	value, ok := raw.(float64)
	if !ok {
		return nil, false, false
	}
	return &value, true, true
}
