package recentactivities

import (
	"net/http"
	"regexp"
	"strings"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
	"satpam-go/internal/web"
)

var dateOnlyPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var allowedActivityTypes = map[string]bool{
	"ATTENDANCE_CHECK_IN":  true,
	"ATTENDANCE_CHECK_OUT": true,
	"PATROL_SPOT_SCAN":     true,
	"PATROL_FACILITY_SCAN": true,
}

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
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	activityType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("activityType")))
	fromDate := strings.TrimSpace(r.URL.Query().Get("fromDate"))
	toDate := strings.TrimSpace(r.URL.Query().Get("toDate"))

	if placeID != "" && !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
		return
	}
	if userID != "" && !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid userId")
		return
	}
	if activityType != "" && !allowedActivityTypes[activityType] {
		web.WriteError(w, http.StatusBadRequest, "activityType invalid. Allowed: ATTENDANCE_CHECK_IN, ATTENDANCE_CHECK_OUT, PATROL_SPOT_SCAN, PATROL_FACILITY_SCAN")
		return
	}
	if fromDate != "" && !dateOnlyPattern.MatchString(fromDate) {
		web.WriteError(w, http.StatusBadRequest, "fromDate must use YYYY-MM-DD")
		return
	}
	if toDate != "" && !dateOnlyPattern.MatchString(toDate) {
		web.WriteError(w, http.StatusBadRequest, "toDate must use YYYY-MM-DD")
		return
	}
	if fromDate != "" && toDate != "" && fromDate > toDate {
		web.WriteError(w, http.StatusBadRequest, "fromDate cannot be greater than toDate")
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

	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"activityAt", "activityType", "userId", "placeId"},
		DefaultSortBy: "activityAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}

	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID:  current.UserID,
		ActorRole:    current.Role,
		PlaceID:      placeID,
		UserID:       userID,
		ActivityType: activityType,
		FromDate:     fromDate,
		ToDate:       toDate,
		Query:        query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load recent activities")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}
