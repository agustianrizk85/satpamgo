package roles

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

	if !auth.IsGlobalAdminRole(current.Role) {
		isPlaceAdmin, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !isPlaceAdmin {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
			return
		}
	}

	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"createdAt", "updatedAt", "name", "code"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	result, err := h.repo.List(r.Context(), query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load roles")
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

	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	body.Code = strings.TrimSpace(body.Code)
	body.Name = strings.TrimSpace(body.Name)
	if body.Code == "" || body.Name == "" {
		web.WriteError(w, http.StatusBadRequest, "code and name are required")
		return
	}

	id, err := h.repo.Create(r.Context(), body.Code, body.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrRoleCodeExists):
			web.WriteError(w, http.StatusConflict, "Role code already exists")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create role")
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
	if !auth.IsGlobalAdminRole(current.Role) {
		isPlaceAdmin, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !isPlaceAdmin {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
			return
		}
	}

	roleID := r.PathValue("roleId")
	if !web.IsUUID(roleID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid roleId")
		return
	}

	item, err := h.repo.FindByID(r.Context(), roleID)
	if err != nil {
		if errors.Is(err, ErrRoleNotFound) {
			web.WriteError(w, http.StatusNotFound, "Role not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to fetch role")
		return
	}

	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok || !auth.IsSuperUserRole(current.Role) {
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	roleID := r.PathValue("roleId")
	if !web.IsUUID(roleID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid roleId")
		return
	}

	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	input := UpdateInput{}
	if raw, exists := body["code"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "code must be string")
			return
		}
		input.Code = &value
	}
	if raw, exists := body["name"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "name must be string")
			return
		}
		input.Name = &value
	}

	item, err := h.repo.Update(r.Context(), roleID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrRoleNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrRoleNotFound):
			web.WriteError(w, http.StatusNotFound, "Role not found")
		case errors.Is(err, ErrRoleCodeExists):
			web.WriteError(w, http.StatusConflict, "Role code already exists")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update role")
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

	roleID := r.PathValue("roleId")
	if !web.IsUUID(roleID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid roleId")
		return
	}

	id, err := h.repo.Delete(r.Context(), roleID)
	if err != nil {
		switch {
		case errors.Is(err, ErrRoleNotFound):
			web.WriteError(w, http.StatusNotFound, "Role not found")
		case errors.Is(err, ErrRoleStillInUse):
			web.WriteError(w, http.StatusConflict, "Role is still in use")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to delete role")
		}
		return
	}

	web.WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}
