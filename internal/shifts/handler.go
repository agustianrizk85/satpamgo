package shifts

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
		AllowedSortBy: []string{"createdAt", "updatedAt", "name", "startTime", "endTime", "isActive"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if placeID != "" {
		if !web.IsUUID(placeID) {
			web.WriteError(w, http.StatusBadRequest, "placeId is required")
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
	}

	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     placeID,
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load shifts")
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

	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	name, ok := body["name"].(string)
	startTime, okStart := body["startTime"].(string)
	endTime, okEnd := body["endTime"].(string)
	if !ok || !okStart || !okEnd || strings.TrimSpace(name) == "" || strings.TrimSpace(startTime) == "" || strings.TrimSpace(endTime) == "" {
		web.WriteError(w, http.StatusBadRequest, "name, startTime, endTime are required")
		return
	}

	var resolvedPlaceID string
	if raw, exists := body["placeId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "placeId is required")
			return
		}
		resolvedPlaceID = strings.TrimSpace(value)
	} else if auth.IsGlobalAdminRole(current.Role) {
		web.WriteError(w, http.StatusBadRequest, "placeId is required")
		return
	} else {
		placeIDs, err := h.authRepo.ListUserPlaceIDs(r.Context(), current.UserID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to resolve place access")
			return
		}
		if len(placeIDs) == 0 {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
		resolvedPlaceID = placeIDs[0]
	}

	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, resolvedPlaceID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}

	isActive := true
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		isActive = value
	}

	id, err := h.repo.Create(r.Context(), CreateInput{
		PlaceID:   resolvedPlaceID,
		Name:      name,
		StartTime: startTime,
		EndTime:   endTime,
		IsActive:  isActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrShiftAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Shift name already exists for this place")
		case errors.Is(err, ErrShiftPlaceNotFound):
			web.WriteError(w, http.StatusBadRequest, "placeId not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create shift")
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

	shiftID := r.PathValue("shiftId")
	if !web.IsUUID(shiftID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid shiftId")
		return
	}

	item, err := h.repo.FindByID(r.Context(), shiftID)
	if err != nil {
		if errors.Is(err, ErrShiftNotFound) {
			web.WriteError(w, http.StatusNotFound, "Shift not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to fetch shift")
		return
	}

	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, item.PlaceID, nil)
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
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	shiftID := r.PathValue("shiftId")
	if !web.IsUUID(shiftID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid shiftId")
		return
	}

	currentShift, err := h.repo.FindByID(r.Context(), shiftID)
	if err != nil {
		if errors.Is(err, ErrShiftNotFound) {
			web.WriteError(w, http.StatusNotFound, "Shift not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to fetch shift")
		return
	}

	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, currentShift.PlaceID, []string{auth.PlaceRoleAdmin})
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

	input := UpdateInput{}
	if raw, exists := body["placeId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "placeId must be uuid")
			return
		}
		nextPlaceID := strings.TrimSpace(value)
		if !auth.IsGlobalAdminRole(current.Role) && nextPlaceID != currentShift.PlaceID {
			ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, nextPlaceID, []string{auth.PlaceRoleAdmin})
			if err != nil {
				web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
				return
			}
			if !ok {
				web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
				return
			}
		}
		input.PlaceID = &nextPlaceID
	}
	if raw, exists := body["name"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "name must be string")
			return
		}
		input.Name = &value
	}
	if raw, exists := body["startTime"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "startTime must be string")
			return
		}
		input.StartTime = &value
	}
	if raw, exists := body["endTime"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "endTime must be string")
			return
		}
		input.EndTime = &value
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		input.IsActive = &value
	}

	item, err := h.repo.Update(r.Context(), shiftID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrShiftNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrShiftNotFound):
			web.WriteError(w, http.StatusNotFound, "Shift not found")
		case errors.Is(err, ErrShiftAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Shift name already exists for this place")
		case errors.Is(err, ErrShiftPlaceNotFound):
			web.WriteError(w, http.StatusBadRequest, "placeId not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update shift")
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

	shiftID := r.PathValue("shiftId")
	if !web.IsUUID(shiftID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid shiftId")
		return
	}

	item, err := h.repo.FindByID(r.Context(), shiftID)
	if err != nil {
		if errors.Is(err, ErrShiftNotFound) {
			web.WriteError(w, http.StatusNotFound, "Shift not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to fetch shift")
		return
	}

	if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, item.PlaceID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}

	id, err := h.repo.Delete(r.Context(), shiftID)
	if err != nil {
		switch {
		case errors.Is(err, ErrShiftNotFound):
			web.WriteError(w, http.StatusNotFound, "Shift not found")
		case errors.Is(err, ErrShiftStillInUse):
			web.WriteError(w, http.StatusConflict, "Shift is still in use")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to delete shift")
		}
		return
	}

	web.WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}
