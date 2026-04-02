package tokenconfig

import (
	"errors"
	"net/http"

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
	item, err := h.repo.Get(r.Context())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Token config not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load token config")
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
		web.WriteError(w, http.StatusForbidden, "Forbidden: insufficient global role")
		return
	}

	var body struct {
		AccessTTLSeconds  int `json:"accessTtlSeconds"`
		RefreshTTLSeconds int `json:"refreshTtlSeconds"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if body.AccessTTLSeconds < 1 || body.RefreshTTLSeconds < 1 {
		web.WriteError(w, http.StatusBadRequest, "accessTtlSeconds and refreshTtlSeconds must be >= 1")
		return
	}

	item, err := h.repo.Upsert(r.Context(), body.AccessTTLSeconds, body.RefreshTTLSeconds)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to save token config")
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
