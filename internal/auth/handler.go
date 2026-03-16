package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"satpam-go/internal/web"
)

type Handler struct {
	repo         *Repository
	tokenService *TokenService
}

func NewHandler(repo *Repository, tokenService *TokenService) *Handler {
	return &Handler{
		repo:         repo,
		tokenService: tokenService,
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	body.Username = strings.TrimSpace(body.Username)
	body.Password = strings.TrimSpace(body.Password)
	if body.Username == "" || body.Password == "" {
		web.WriteError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := h.repo.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			web.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
			return
		}

		web.WriteError(w, http.StatusInternalServerError, "Failed to login")
		return
	}

	accessToken, err := h.tokenService.Sign(user.ID, user.RoleCode)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to sign access token")
		return
	}

	placeAccesses, err := h.repo.ListUserPlaceAccess(r.Context(), user.ID)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load place access")
		return
	}

	var defaultPlaceID any
	if len(placeAccesses) > 0 {
		defaultPlaceID = placeAccesses[0].PlaceID
	}

	web.WriteJSON(w, http.StatusOK, map[string]any{
		"accessToken": accessToken,
		"user": map[string]any{
			"id":             user.ID,
			"fullName":       user.FullName,
			"username":       user.Username,
			"status":         user.Status,
			"role":           user.RoleCode,
			"defaultPlaceId": defaultPlaceID,
			"placeAccesses":  placeAccesses,
		},
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	current, ok := AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	user, err := h.repo.FindUserByID(r.Context(), current.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			web.WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		web.WriteError(w, http.StatusInternalServerError, "Failed to fetch current user")
		return
	}

	placeAccesses, err := h.repo.ListUserPlaceAccess(r.Context(), user.ID)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load place access")
		return
	}

	var defaultPlaceID any
	if len(placeAccesses) > 0 {
		defaultPlaceID = placeAccesses[0].PlaceID
	}

	web.WriteJSON(w, http.StatusOK, map[string]any{
		"id":             user.ID,
		"fullName":       user.FullName,
		"username":       user.Username,
		"status":         user.Status,
		"role":           user.RoleCode,
		"defaultPlaceId": defaultPlaceID,
		"placeAccesses":  placeAccesses,
	})
}
