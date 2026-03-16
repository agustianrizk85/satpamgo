package attendanceconfig

import (
	"errors"
	"net/http"
	"strings"

	"satpam-go/internal/auth"
	"satpam-go/internal/web"
)

type Handler struct {
	repo     *Repository
	authRepo *auth.Repository
}

func NewHandler(repo *Repository, authRepo *auth.Repository) *Handler {
	return &Handler{repo: repo, authRepo: authRepo}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
		return
	}
	item, err := h.repo.GetByPlaceID(r.Context(), placeID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Attendance config not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load attendance config")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	if !auth.IsGlobalAdminRole(current.Role) {
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

	var body struct {
		PlaceID         string   `json:"placeId"`
		AllowedRadiusM  int      `json:"allowedRadiusM"`
		CenterLatitude  *float64 `json:"centerLatitude"`
		CenterLongitude *float64 `json:"centerLongitude"`
		RequirePhoto    *bool    `json:"requirePhoto"`
		IsActive        *bool    `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	if !web.IsUUID(body.PlaceID) || body.AllowedRadiusM < 0 {
		web.WriteError(w, http.StatusBadRequest, "placeId must be valid uuid and allowedRadiusM must be >= 0")
		return
	}
	requirePhoto := false
	if body.RequirePhoto != nil {
		requirePhoto = *body.RequirePhoto
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	item, err := h.repo.Upsert(r.Context(), UpsertInput{
		PlaceID:         body.PlaceID,
		AllowedRadiusM:  body.AllowedRadiusM,
		CenterLatitude:  body.CenterLatitude,
		CenterLongitude: body.CenterLongitude,
		RequirePhoto:    requirePhoto,
		IsActive:        isActive,
	})
	if err != nil {
		if errors.Is(err, ErrForeignKey) {
			web.WriteError(w, http.StatusBadRequest, "Place not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to save attendance config")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	_, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return false
	}
	return true
}
