package apierrorlogs

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
	"satpam-go/internal/web"
)

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

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

	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	method := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("method")))
	fromDate := strings.TrimSpace(r.URL.Query().Get("fromDate"))
	toDate := strings.TrimSpace(r.URL.Query().Get("toDate"))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	statusCodeRaw := strings.TrimSpace(r.URL.Query().Get("statusCode"))

	if placeID != "" && !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
		return
	}
	if fromDate != "" && !datePattern.MatchString(fromDate) {
		web.WriteError(w, http.StatusBadRequest, "fromDate must use YYYY-MM-DD")
		return
	}
	if toDate != "" && !datePattern.MatchString(toDate) {
		web.WriteError(w, http.StatusBadRequest, "toDate must use YYYY-MM-DD")
		return
	}
	if fromDate != "" && toDate != "" && fromDate > toDate {
		web.WriteError(w, http.StatusBadRequest, "fromDate cannot be greater than toDate")
		return
	}

	statusCode := 0
	if statusCodeRaw != "" {
		parsed, err := strconv.Atoi(statusCodeRaw)
		if err != nil || parsed < 400 || parsed > 599 {
			web.WriteError(w, http.StatusBadRequest, "statusCode must be integer between 400 and 599")
			return
		}
		statusCode = parsed
	}

	if method != "" {
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		default:
			web.WriteError(w, http.StatusBadRequest, "method invalid. Allowed: GET, POST, PATCH, PUT, DELETE")
			return
		}
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

	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"occurredAt", "statusCode", "method", "path"},
		DefaultSortBy: "occurredAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     placeID,
		Method:      method,
		StatusCode:  statusCode,
		FromDate:    fromDate,
		ToDate:      toDate,
		Search:      search,
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load api error logs")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}
