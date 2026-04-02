package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"satpam-go/internal/web"
)

type Handler struct {
	repo           *Repository
	tokenService   *TokenService
	resolveTokenTTL func(context.Context) (time.Duration, time.Duration, error)
}

func NewHandler(repo *Repository, tokenService *TokenService, resolveTokenTTL func(context.Context) (time.Duration, time.Duration, error)) *Handler {
	return &Handler{
		repo:            repo,
		tokenService:    tokenService,
		resolveTokenTTL: resolveTokenTTL,
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

	accessTTL, refreshTTL, err := h.resolveTokenTTLs(r)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load token config")
		return
	}

	accessToken, err := h.tokenService.SignWithTTL(user.ID, user.RoleCode, accessTTL)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to sign access token")
		return
	}
	refreshToken, err := h.tokenService.SignRefreshWithTTL(user.ID, user.RoleCode, refreshTTL)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to sign refresh token")
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
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
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

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}

	body.RefreshToken = strings.TrimSpace(body.RefreshToken)
	if body.RefreshToken == "" {
		web.WriteError(w, http.StatusBadRequest, "refreshToken is required")
		return
	}

	claims, err := h.tokenService.VerifyRefresh(body.RefreshToken)
	if err != nil {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	user, err := h.repo.FindUserByID(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			web.WriteError(w, http.StatusUnauthorized, "User not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to refresh token")
		return
	}

	accessTTL, refreshTTL, err := h.resolveTokenTTLs(r)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load token config")
		return
	}

	accessToken, err := h.tokenService.SignWithTTL(user.ID, user.RoleCode, accessTTL)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to sign access token")
		return
	}
	refreshToken, err := h.tokenService.SignRefreshWithTTL(user.ID, user.RoleCode, refreshTTL)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to sign refresh token")
		return
	}

	web.WriteJSON(w, http.StatusOK, map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	})
}

func (h *Handler) resolveTokenTTLs(r *http.Request) (time.Duration, time.Duration, error) {
	if h.resolveTokenTTL == nil {
		return 8 * time.Hour, 30 * 24 * time.Hour, nil
	}
	return h.resolveTokenTTL(r.Context())
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
