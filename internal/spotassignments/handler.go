package spotassignments

import (
	"context"
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
		AllowedSortBy: []string{"createdAt", "updatedAt", "placeId", "userId", "shiftId", "isActive"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	var isActive *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("isActive")); raw != "" {
		value := strings.EqualFold(raw, "true")
		isActive = &value
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))

	canManageAll, err := h.canManageAssignments(r.Context(), current.UserID, current.Role)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return
	}
	if placeID != "" && !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "placeId must be valid uuid")
		return
	}
	if userID != "" && !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "userId must be valid uuid")
		return
	}
	if placeID != "" && !auth.IsGlobalAdminRole(current.Role) {
		hasAccess, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, placeID, nil)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !hasAccess {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}
	if !canManageAll {
		if userID != "" && userID != current.UserID {
			web.WriteError(w, http.StatusForbidden, "Forbidden: can only view your own assignments")
			return
		}
		userID = current.UserID
	}

	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     placeID,
		UserID:      userID,
		IsActive:    isActive,
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load spot assignments")
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
	id := r.PathValue("assignmentId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid assignmentId")
		return
	}
	item, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Spot assignment not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load spot assignment")
		return
	}
	canManageAll, err := h.canManageAssignments(r.Context(), current.UserID, current.Role)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return
	}
	if !auth.IsGlobalAdminRole(current.Role) {
		hasAccess, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, item.PlaceID, nil)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !hasAccess {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	}
	if !canManageAll && item.UserID != current.UserID {
		web.WriteError(w, http.StatusForbidden, "Forbidden: can only view your own assignments")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	var body struct {
		PlaceID  string `json:"placeId"`
		UserID   string `json:"userId"`
		ShiftID  string `json:"shiftId"`
		IsActive *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.UserID = strings.TrimSpace(body.UserID)
	body.ShiftID = strings.TrimSpace(body.ShiftID)
	if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.UserID) || !web.IsUUID(body.ShiftID) {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, and shiftId must be valid uuid")
		return
	}
	isSelfAssignment := body.UserID == current.UserID
	if isSelfAssignment {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, body.PlaceID, nil)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	} else if !h.requireAdmin(w, r) {
		return
	} else if !auth.IsGlobalAdminRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, body.PlaceID, []string{auth.PlaceRoleAdmin})
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
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	id, err := h.repo.Create(r.Context(), CreateInput{PlaceID: body.PlaceID, UserID: body.UserID, ShiftID: body.ShiftID, IsActive: isActive})
	if err != nil {
		if errors.Is(err, ErrForeignKey) {
			web.WriteError(w, http.StatusBadRequest, "Related place/user/shift not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to create spot assignment")
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("assignmentId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid assignmentId")
		return
	}
	currentAssignment, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Spot assignment not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load spot assignment")
		return
	}
	isSelfAssignment := currentAssignment.UserID == current.UserID
	if isSelfAssignment && !auth.IsGlobalAdminRole(current.Role) {
		hasAccess, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, currentAssignment.PlaceID, nil)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !hasAccess {
			web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
			return
		}
	} else {
		if !h.requireAdmin(w, r) {
			return
		}
		if !auth.IsGlobalAdminRole(current.Role) {
			hasAccess, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, currentAssignment.PlaceID, []string{auth.PlaceRoleAdmin})
			if err != nil {
				web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
				return
			}
			if !hasAccess {
				web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
				return
			}
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
		if isSelfAssignment && !auth.IsGlobalAdminRole(current.Role) && value != current.UserID {
			web.WriteError(w, http.StatusForbidden, "Forbidden: can only update your own assignment")
			return
		}
		input.UserID = &value
	}
	if raw, exists := body["shiftId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "shiftId must be valid uuid")
			return
		}
		value = strings.TrimSpace(value)
		input.ShiftID = &value
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		input.IsActive = &value
	}
	item, err := h.repo.Update(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrNotFound):
			web.WriteError(w, http.StatusNotFound, "Spot assignment not found")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/user/shift not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update spot assignment")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("assignmentId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid assignmentId")
		return
	}
	out, err := h.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Spot assignment not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete spot assignment")
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

func (h *Handler) canManageAssignments(ctx context.Context, userID, role string) (bool, error) {
	if auth.IsGlobalAdminRole(role) {
		return true, nil
	}
	return h.authRepo.HasAnyPlaceRole(ctx, userID, []string{auth.PlaceRoleAdmin})
}
