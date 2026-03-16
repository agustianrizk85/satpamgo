package userplaceroles

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
	if !auth.IsSuperUserRole(current.Role) {
		ok, err := h.authRepo.HasAnyPlaceRole(r.Context(), current.UserID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
			return
		}
	}

	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{
			"createdAt", "updatedAt", "userId", "placeId", "roleId", "isActive",
			"username", "fullName", "placeCode", "placeName", "roleCode", "roleName",
		},
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
		web.WriteError(w, http.StatusInternalServerError, "Failed to load user place roles")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	var body struct {
		UserID   string `json:"userId"`
		PlaceID  string `json:"placeId"`
		RoleID   string `json:"roleId"`
		IsActive *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	body.UserID = strings.TrimSpace(body.UserID)
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.RoleID = strings.TrimSpace(body.RoleID)
	if !web.IsUUID(body.UserID) || !web.IsUUID(body.PlaceID) || !web.IsUUID(body.RoleID) {
		web.WriteError(w, http.StatusBadRequest, "userId, placeId, and roleId must be valid uuid")
		return
	}

	if !auth.IsSuperUserRole(current.Role) {
		ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, body.PlaceID, []string{auth.PlaceRoleAdmin})
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
			return
		}
		if !ok {
			web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient place access")
			return
		}
	}

	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	id, err := h.repo.Upsert(r.Context(), UpsertInput{
		UserID:   body.UserID,
		PlaceID:  body.PlaceID,
		RoleID:   body.RoleID,
		IsActive: isActive,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related user/place/role not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to save user place role")
		}
		return
	}

	web.WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}
