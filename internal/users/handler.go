package users

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

func (h *Handler) ensurePlaceAdminAccess(r *http.Request) (*auth.AuthContext, bool, error) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		return nil, false, nil
	}
	if auth.IsSuperUserRole(current.Role) {
		return &current, true, nil
	}

	hasPlaceRole, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{placeRoleAdmin})
	if err != nil {
		return &current, false, err
	}
	if !hasPlaceRole {
		return &current, false, nil
	}

	return &current, true, nil
}

func (h *Handler) ensureScopedTargetAccess(r *http.Request, current *auth.AuthContext, userID string) error {
	_, err := h.repo.FindByID(r.Context(), ListUsersParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
	}, userID)
	return err
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	isDirectPlaceAdmin := current.Role == placeRoleAdmin
	if !isGlobalAdminRole(current.Role) {
		hasPlaceRole, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{placeRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}

		if !isDirectPlaceAdmin && !hasPlaceRole {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
			return
		}
	}

	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"createdAt", "updatedAt", "fullName", "username", "status"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	result, err := h.repo.ListUsers(r.Context(), ListUsersParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load users")
		return
	}

	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	isDirectPlaceAdmin := current.Role == placeRoleAdmin
	if !isGlobalAdminRole(current.Role) {
		hasPlaceRole, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{placeRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !isDirectPlaceAdmin && !hasPlaceRole {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
			return
		}
	}

	userID := r.PathValue("userId")
	if !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid userId")
		return
	}

	item, err := h.repo.FindByID(r.Context(), ListUsersParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
	}, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			web.WriteError(w, http.StatusNotFound, "User not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load user")
		return
	}

	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	current, allowed, err := h.ensurePlaceAdminAccess(r)
	if current == nil {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return
	}
	if !allowed {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	var body struct {
		RoleID      string `json:"roleId"`
		FullName    string `json:"fullName"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		Status      string `json:"status"`
		PlaceID     string `json:"placeId"`
		PlaceRoleID string `json:"placeRoleId"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	body.RoleID = strings.TrimSpace(body.RoleID)
	body.FullName = strings.TrimSpace(body.FullName)
	body.Username = strings.TrimSpace(body.Username)
	body.Password = strings.TrimSpace(body.Password)
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.PlaceRoleID = strings.TrimSpace(body.PlaceRoleID)
	body.Status = normalizeUserStatus(body.Status)

	if !web.IsUUID(body.RoleID) || body.FullName == "" || body.Username == "" || body.Password == "" {
		web.WriteError(w, http.StatusBadRequest, "roleId, fullName, username, and password are required")
		return
	}
	if body.Status == "" {
		web.WriteError(w, http.StatusBadRequest, "status must be ACTIVE or INACTIVE")
		return
	}

	assignPlaceRole := body.PlaceID != "" || body.PlaceRoleID != ""
	if auth.IsSuperUserRole(current.Role) {
		if assignPlaceRole {
			if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.PlaceRoleID) {
				web.WriteError(w, http.StatusBadRequest, "placeId and placeRoleId must be valid uuid")
				return
			}
		}
	} else {
		if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.PlaceRoleID) {
			web.WriteError(w, http.StatusBadRequest, "placeId and placeRoleId are required for place admin")
			return
		}

		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, body.PlaceID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate place access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient place access")
			return
		}

		roleCode, err := h.repo.FindRoleCodeByID(r.Context(), body.RoleID)
		if err != nil {
			if errors.Is(err, ErrUserRoleNotFound) {
				web.WriteError(w, http.StatusBadRequest, "Role not found")
				return
			}
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate role")
			return
		}
		if isGlobalAdminRole(strings.ToUpper(strings.TrimSpace(roleCode))) {
			web.WriteError(w, http.StatusForbidden, "Forbidden: place admin cannot create global users")
			return
		}
	}

	var id string
	if assignPlaceRole {
		id, err = h.repo.CreateWithPlaceRole(r.Context(), CreateWithPlaceInput{
			RoleID:      body.RoleID,
			FullName:    body.FullName,
			Username:    body.Username,
			Password:    body.Password,
			Status:      body.Status,
			PlaceID:     body.PlaceID,
			PlaceRoleID: body.PlaceRoleID,
		})
	} else {
		id, err = h.repo.Create(r.Context(), CreateInput{
			RoleID:   body.RoleID,
			FullName: body.FullName,
			Username: body.Username,
			Password: body.Password,
			Status:   body.Status,
		})
	}
	if err != nil {
		switch {
		case errors.Is(err, ErrUsernameExists):
			web.WriteError(w, http.StatusConflict, "Username already exists")
		case errors.Is(err, ErrUserRoleNotFound):
			if assignPlaceRole {
				web.WriteError(w, http.StatusBadRequest, "Role or place assignment not found")
			} else {
				web.WriteError(w, http.StatusBadRequest, "Role not found")
			}
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create user")
		}
		return
	}

	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) PatchUser(w http.ResponseWriter, r *http.Request) {
	current, allowed, err := h.ensurePlaceAdminAccess(r)
	if current == nil {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return
	}
	if !allowed {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	userID := r.PathValue("userId")
	if !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid userId")
		return
	}

	if !auth.IsSuperUserRole(current.Role) {
		err := h.ensureScopedTargetAccess(r, current, userID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				web.WriteError(w, http.StatusNotFound, "User not found")
				return
			}
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
	}

	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	var input UpdateInput

	if raw, exists := body["roleId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "roleId must be valid uuid")
			return
		}
		value = strings.TrimSpace(value)
		input.RoleID = &value

		if !auth.IsSuperUserRole(current.Role) {
			roleCode, err := h.repo.FindRoleCodeByID(r.Context(), value)
			if err != nil {
				if errors.Is(err, ErrUserRoleNotFound) {
					web.WriteError(w, http.StatusBadRequest, "Role not found")
					return
				}
				web.WriteError(w, http.StatusInternalServerError, "Failed to validate role")
				return
			}
			if isGlobalAdminRole(strings.ToUpper(strings.TrimSpace(roleCode))) {
				web.WriteError(w, http.StatusForbidden, "Forbidden: place admin cannot assign global roles")
				return
			}
		}
	}
	if raw, exists := body["fullName"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "fullName must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.FullName = &value
	}
	if raw, exists := body["username"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "username must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.Username = &value
	}
	if raw, exists := body["password"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "password must be string")
			return
		}
		value = strings.TrimSpace(value)
		input.Password = &value
	}
	if raw, exists := body["status"]; exists {
		value, ok := raw.(string)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "status must be string")
			return
		}
		value = normalizeUserStatus(value)
		if value == "" {
			web.WriteError(w, http.StatusBadRequest, "status must be ACTIVE or INACTIVE")
			return
		}
		input.Status = &value
	}

	item, err := h.repo.Update(r.Context(), userID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrUserNotFound):
			web.WriteError(w, http.StatusNotFound, "User not found")
		case errors.Is(err, ErrUsernameExists):
			web.WriteError(w, http.StatusConflict, "Username already exists")
		case errors.Is(err, ErrUserRoleNotFound):
			web.WriteError(w, http.StatusBadRequest, "Role not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update user")
		}
		return
	}

	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	current, allowed, err := h.ensurePlaceAdminAccess(r)
	if current == nil {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return
	}
	if !allowed {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	userID := r.PathValue("userId")
	if !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid userId")
		return
	}

	if !auth.IsSuperUserRole(current.Role) {
		err := h.ensureScopedTargetAccess(r, current, userID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				web.WriteError(w, http.StatusNotFound, "User not found")
				return
			}
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
	}

	id, err := h.repo.SoftDelete(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			web.WriteError(w, http.StatusNotFound, "User not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	web.WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}

func normalizeUserStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "":
		return "ACTIVE"
	case "ACTIVE", "INACTIVE":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
}
