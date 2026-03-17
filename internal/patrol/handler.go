package patrol

import (
	"errors"
	"net/http"
	"strconv"
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

func (h *Handler) requirePlaceAdminAccess(w http.ResponseWriter, r *http.Request, placeID string) bool {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return false
	}
	if auth.IsGlobalAdminRole(current.Role) {
		return true
	}
	ok, err := h.authRepo.HasPlaceAccess(r.Context(), current.UserID, placeID, []string{auth.PlaceRoleAdmin})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
		return false
	}
	if !ok {
		web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
		return false
	}
	return true
}

func (h *Handler) ListRoutePoints(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "placeId is required")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"seq", "createdAt", "updatedAt", "spotId", "isActive"}, DefaultSortBy: "seq", DefaultSortOrder: listquery.SortAsc})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.ListRoutePoints(r.Context(), current.UserID, current.Role, placeID, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol route points")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateRoutePoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlaceID  string `json:"placeId"`
		SpotID   string `json:"spotId"`
		Seq      int    `json:"seq"`
		IsActive *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || !web.IsUUID(strings.TrimSpace(body.SpotID)) || body.Seq < 1 {
		web.WriteError(w, http.StatusBadRequest, "placeId, spotId, and seq are required")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, strings.TrimSpace(body.PlaceID)) {
		return
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	id, err := h.repo.CreateRoutePoint(r.Context(), strings.TrimSpace(body.PlaceID), strings.TrimSpace(body.SpotID), body.Seq, isActive)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol route point already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/spot not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create patrol route point")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) DeleteRoutePoint(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if !web.IsUUID(id) || !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "id and placeId are required")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, placeID) {
		return
	}
	out, err := h.repo.DeleteRoutePoint(r.Context(), id, placeID)
	if err != nil {
		if errors.Is(err, ErrRoutePointNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol route point not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete patrol route point")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func (h *Handler) ListScans(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "placeId is required")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"scannedAt", "submitAt", "placeId", "userId", "spotId", "patrolRunId"}, DefaultSortBy: "scannedAt"})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.ListScans(r.Context(), current.UserID, current.Role, placeID, strings.TrimSpace(r.URL.Query().Get("patrolRunId")), strings.TrimSpace(r.URL.Query().Get("userId")), query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol scans")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateScan(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	var body struct {
		PlaceID     string  `json:"placeId"`
		UserID      string  `json:"userId"`
		SpotID      string  `json:"spotId"`
		PatrolRunID string  `json:"patrolRunId"`
		ScannedAt   *string `json:"scannedAt"`
		SubmitAt    *string `json:"submitAt"`
		PhotoURL    *string `json:"photoUrl"`
		Note        *string `json:"note"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || !web.IsUUID(strings.TrimSpace(body.UserID)) || !web.IsUUID(strings.TrimSpace(body.SpotID)) || strings.TrimSpace(body.PatrolRunID) == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, spotId, and patrolRunId are required")
		return
	}
	id, err := h.repo.CreateScan(r.Context(), strings.TrimSpace(body.PlaceID), strings.TrimSpace(body.UserID), strings.TrimSpace(body.SpotID), strings.TrimSpace(body.PatrolRunID), trimStringPtr(body.ScannedAt), trimStringPtr(body.SubmitAt), trimStringPtr(body.PhotoURL), trimStringPtr(body.Note))
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol scan already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/user/spot not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create patrol scan")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

var _ = strconv.Itoa
