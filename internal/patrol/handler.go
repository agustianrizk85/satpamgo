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

func isDateOnly(v string) bool {
	if len(v) != 10 {
		return false
	}
	for i, r := range v {
		if i == 4 || i == 7 {
			if r != '-' {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

func (h *Handler) ListRoundMasters(w http.ResponseWriter, r *http.Request) {
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
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"roundNo", "isActive", "createdAt", "updatedAt"}, DefaultSortBy: "roundNo", DefaultSortOrder: listquery.SortAsc})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.ListRoundMasters(r.Context(), current.UserID, current.Role, placeID, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol round masters")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateRoundMaster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlaceID  string `json:"placeId"`
		RoundNo  int    `json:"roundNo"`
		IsActive *bool  `json:"isActive"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || body.RoundNo < 1 {
		web.WriteError(w, http.StatusBadRequest, "placeId and roundNo are required")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, strings.TrimSpace(body.PlaceID)) {
		return
	}
	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	item, err := h.repo.CreateRoundMaster(r.Context(), strings.TrimSpace(body.PlaceID), body.RoundNo, isActive)
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol round master already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create patrol round master")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, item)
}

func (h *Handler) PatchRoundMaster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("roundMasterId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid roundMasterId")
		return
	}
	currentMaster, err := h.repo.GetRoundMaster(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPatrolRoundMasterNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol round master not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol round master")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, currentMaster.PlaceID) {
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var roundNo *int
	var isActive *bool
	if raw, exists := body["roundNo"]; exists {
		value, ok := raw.(float64)
		if !ok || int(value) < 1 {
			web.WriteError(w, http.StatusBadRequest, "roundNo must be number >= 1")
			return
		}
		v := int(value)
		roundNo = &v
	}
	if raw, exists := body["isActive"]; exists {
		value, ok := raw.(bool)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "isActive must be boolean")
			return
		}
		isActive = &value
	}
	item, err := h.repo.UpdateRoundMaster(r.Context(), id, roundNo, isActive)
	if err != nil {
		switch {
		case errors.Is(err, ErrPatrolRoundMasterNotFound):
			web.WriteError(w, http.StatusNotFound, "Patrol round master not found")
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol round master already exists")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update patrol round master")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteRoundMaster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("roundMasterId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid roundMasterId")
		return
	}
	currentMaster, err := h.repo.GetRoundMaster(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPatrolRoundMasterNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol round master not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol round master")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, currentMaster.PlaceID) {
		return
	}
	out, err := h.repo.DeleteRoundMaster(r.Context(), id, currentMaster.PlaceID)
	if err != nil {
		if errors.Is(err, ErrPatrolRoundMasterNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol round master not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete patrol round master")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func (h *Handler) ListRoundStatuses(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	shiftID := strings.TrimSpace(r.URL.Query().Get("shiftId"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	fromDate := strings.TrimSpace(r.URL.Query().Get("fromDate"))
	toDate := strings.TrimSpace(r.URL.Query().Get("toDate"))
	if !web.IsUUID(placeID) || !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "placeId and userId are required")
		return
	}
	if shiftID != "" && !web.IsUUID(shiftID) {
		web.WriteError(w, http.StatusBadRequest, "shiftId must be valid UUID")
		return
	}
	if date != "" && !isDateOnly(date) {
		web.WriteError(w, http.StatusBadRequest, "date must use YYYY-MM-DD")
		return
	}
	if fromDate != "" && !isDateOnly(fromDate) {
		web.WriteError(w, http.StatusBadRequest, "fromDate must use YYYY-MM-DD")
		return
	}
	if toDate != "" && !isDateOnly(toDate) {
		web.WriteError(w, http.StatusBadRequest, "toDate must use YYYY-MM-DD")
		return
	}
	if shiftID != "" && date == "" {
		web.WriteError(w, http.StatusBadRequest, "date is required when shiftId is provided")
		return
	}
	result, err := h.repo.ListRoundStatuses(r.Context(), current.UserID, current.Role, placeID, userID, shiftID, date, fromDate, toDate)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol round statuses")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
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
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"runNo", "status", "startedAt", "completedAt", "createdAt", "updatedAt", "userId", "attendanceId", "totalActiveSpots"},
		DefaultSortBy: "startedAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	attendanceID := strings.TrimSpace(r.URL.Query().Get("attendanceId"))
	shiftID := strings.TrimSpace(r.URL.Query().Get("shiftId"))
	runNoRaw := strings.TrimSpace(r.URL.Query().Get("runNo"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	fromDate := strings.TrimSpace(r.URL.Query().Get("fromDate"))
	toDate := strings.TrimSpace(r.URL.Query().Get("toDate"))
	var runNo *int
	if attendanceID != "" && !web.IsUUID(attendanceID) {
		web.WriteError(w, http.StatusBadRequest, "attendanceId must be valid UUID")
		return
	}
	if shiftID != "" && !web.IsUUID(shiftID) {
		web.WriteError(w, http.StatusBadRequest, "shiftId must be valid UUID")
		return
	}
	if runNoRaw != "" {
		value, err := strconv.Atoi(runNoRaw)
		if err != nil || value < 1 {
			web.WriteError(w, http.StatusBadRequest, "runNo must be integer >= 1")
			return
		}
		runNo = &value
	}
	if fromDate != "" && !isDateOnly(fromDate) {
		web.WriteError(w, http.StatusBadRequest, "fromDate must use YYYY-MM-DD")
		return
	}
	if toDate != "" && !isDateOnly(toDate) {
		web.WriteError(w, http.StatusBadRequest, "toDate must use YYYY-MM-DD")
		return
	}
	if fromDate != "" && toDate != "" && fromDate > toDate {
		web.WriteError(w, http.StatusBadRequest, "fromDate cannot be greater than toDate")
		return
	}
	result, err := h.repo.ListRuns(r.Context(), current.UserID, current.Role, placeID, userID, attendanceID, shiftID, runNo, status, fromDate, toDate, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol runs")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("runId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid runId")
		return
	}
	item, err := h.repo.GetRun(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrPatrolRunNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol run not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol run")
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlaceID          string  `json:"placeId"`
		UserID           string  `json:"userId"`
		AttendanceID     *string `json:"attendanceId"`
		RunNo            *int    `json:"runNo"`
		TotalActiveSpots *int    `json:"totalActiveSpots"`
		Status           *string `json:"status"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || !web.IsUUID(strings.TrimSpace(body.UserID)) {
		web.WriteError(w, http.StatusBadRequest, "placeId and userId are required")
		return
	}
	attendanceID := trimStringPtr(body.AttendanceID)
	if attendanceID != nil && !web.IsUUID(*attendanceID) {
		web.WriteError(w, http.StatusBadRequest, "attendanceId must be valid UUID")
		return
	}
	if body.RunNo != nil && *body.RunNo < 1 {
		web.WriteError(w, http.StatusBadRequest, "runNo must be >= 1")
		return
	}
	if body.TotalActiveSpots != nil && *body.TotalActiveSpots < 0 {
		web.WriteError(w, http.StatusBadRequest, "totalActiveSpots must be >= 0")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, strings.TrimSpace(body.PlaceID)) {
		return
	}
	item, err := h.repo.CreateRun(r.Context(), strings.TrimSpace(body.PlaceID), strings.TrimSpace(body.UserID), attendanceID, body.RunNo, body.TotalActiveSpots, trimStringPtr(body.Status))
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol run already exists")
		case errors.Is(err, ErrPatrolRoundMasterRequired):
			web.WriteError(w, http.StatusBadRequest, "runNo is required because master ronde already exists for this place")
		case errors.Is(err, ErrPatrolRoundMasterNotFound):
			web.WriteError(w, http.StatusBadRequest, "runNo not found in master ronde for this place")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/user/attendance not found")
		case strings.Contains(strings.ToLower(err.Error()), "status"):
			web.WriteError(w, http.StatusBadRequest, "Invalid patrol run status")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create patrol run")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, item)
}

func (h *Handler) PatchRun(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("runId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid runId")
		return
	}
	currentRun, err := h.repo.GetRun(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrPatrolRunNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol run not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol run")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, currentRun.PlaceID) {
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var runNo, totalActiveSpots *int
	var status *string
	if raw, exists := body["runNo"]; exists {
		value, ok := raw.(float64)
		if !ok || int(value) < 1 {
			web.WriteError(w, http.StatusBadRequest, "runNo must be number >= 1")
			return
		}
		v := int(value)
		runNo = &v
	}
	if raw, exists := body["totalActiveSpots"]; exists {
		value, ok := raw.(float64)
		if !ok || int(value) < 0 {
			web.WriteError(w, http.StatusBadRequest, "totalActiveSpots must be number >= 0")
			return
		}
		v := int(value)
		totalActiveSpots = &v
	}
	if raw, exists := body["status"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "status must be string")
			return
		}
		v := strings.TrimSpace(value)
		status = &v
	}
	item, err := h.repo.UpdateRun(r.Context(), id, runNo, totalActiveSpots, status)
	if err != nil {
		switch {
		case errors.Is(err, ErrPatrolRunNotFound):
			web.WriteError(w, http.StatusNotFound, "Patrol run not found")
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol run already exists")
		case errors.Is(err, ErrPatrolRoundMasterNotFound):
			web.WriteError(w, http.StatusBadRequest, "runNo not found in master ronde for this place")
		case strings.Contains(strings.ToLower(err.Error()), "status"):
			web.WriteError(w, http.StatusBadRequest, "Invalid patrol run status")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update patrol run")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteRun(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("runId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid runId")
		return
	}
	currentRun, err := h.repo.GetRun(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrPatrolRunNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol run not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol run")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, currentRun.PlaceID) {
		return
	}
	out, err := h.repo.DeleteRun(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPatrolRunNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol run not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete patrol run")
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
	fromDate := strings.TrimSpace(r.URL.Query().Get("fromDate"))
	toDate := strings.TrimSpace(r.URL.Query().Get("toDate"))
	if !web.IsUUID(placeID) {
		web.WriteError(w, http.StatusBadRequest, "placeId is required")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{AllowedSortBy: []string{"scannedAt", "submitAt", "placeId", "userId", "spotId", "patrolRunId"}, DefaultSortBy: "scannedAt"})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	attendanceID := strings.TrimSpace(r.URL.Query().Get("attendanceId"))
	if attendanceID != "" && !web.IsUUID(attendanceID) {
		web.WriteError(w, http.StatusBadRequest, "attendanceId must be valid UUID")
		return
	}
	if fromDate != "" && !isDateOnly(fromDate) {
		web.WriteError(w, http.StatusBadRequest, "fromDate must use YYYY-MM-DD")
		return
	}
	if toDate != "" && !isDateOnly(toDate) {
		web.WriteError(w, http.StatusBadRequest, "toDate must use YYYY-MM-DD")
		return
	}
	if fromDate != "" && toDate != "" && fromDate > toDate {
		web.WriteError(w, http.StatusBadRequest, "fromDate cannot be greater than toDate")
		return
	}
	result, err := h.repo.ListScans(r.Context(), current.UserID, current.Role, placeID, strings.TrimSpace(r.URL.Query().Get("patrolRunId")), strings.TrimSpace(r.URL.Query().Get("userId")), attendanceID, fromDate, toDate, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol scans")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetProgress(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	attendanceID := strings.TrimSpace(r.URL.Query().Get("attendanceId"))
	if !web.IsUUID(attendanceID) {
		web.WriteError(w, http.StatusBadRequest, "attendanceId is required")
		return
	}
	result, err := h.repo.GetProgress(r.Context(), current.UserID, current.Role, attendanceID)
	if err != nil {
		if errors.Is(err, ErrProgressNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol progress not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol progress")
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
		PlaceID      string  `json:"placeId"`
		UserID       string  `json:"userId"`
		SpotID       string  `json:"spotId"`
		PatrolRunID  string  `json:"patrolRunId"`
		RunNo        *int    `json:"runNo"`
		AttendanceID *string `json:"attendanceId"`
		ScannedAt    *string `json:"scannedAt"`
		SubmitAt     *string `json:"submitAt"`
		PhotoURL     *string `json:"photoUrl"`
		Note         *string `json:"note"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	if !web.IsUUID(strings.TrimSpace(body.PlaceID)) || !web.IsUUID(strings.TrimSpace(body.UserID)) || !web.IsUUID(strings.TrimSpace(body.SpotID)) {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, and spotId are required")
		return
	}
	if strings.TrimSpace(body.PatrolRunID) != "" && !web.IsUUID(strings.TrimSpace(body.PatrolRunID)) {
		web.WriteError(w, http.StatusBadRequest, "patrolRunId must be valid UUID")
		return
	}
	if body.RunNo != nil && *body.RunNo < 1 {
		web.WriteError(w, http.StatusBadRequest, "runNo must be >= 1")
		return
	}
	attendanceID := trimStringPtr(body.AttendanceID)
	if attendanceID != nil && !web.IsUUID(*attendanceID) {
		web.WriteError(w, http.StatusBadRequest, "attendanceId must be valid UUID")
		return
	}
	result, err := h.repo.CreateScan(r.Context(), strings.TrimSpace(body.PlaceID), strings.TrimSpace(body.UserID), strings.TrimSpace(body.SpotID), strings.TrimSpace(body.PatrolRunID), body.RunNo, attendanceID, trimStringPtr(body.ScannedAt), trimStringPtr(body.SubmitAt), trimStringPtr(body.PhotoURL), trimStringPtr(body.Note))
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol scan already exists")
		case errors.Is(err, ErrPatrolRoundMasterNotFound):
			web.WriteError(w, http.StatusBadRequest, "runNo not found in master ronde for this place")
		case errors.Is(err, ErrPatrolRunNotFound):
			web.WriteError(w, http.StatusBadRequest, "Patrol run not found for selected place/user")
		case errors.Is(err, ErrPatrolRunClosed):
			web.WriteError(w, http.StatusConflict, "Patrol run already completed")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related place/user/spot not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create patrol scan")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":                 result.ID,
		"patrolRunId":        result.PatrolRunID,
		"patrolRunNo":        result.PatrolRunNo,
		"isNewPatrolRun":     result.IsNewPatrolRun,
		"patrolRunCompleted": result.PatrolRunCompleted,
	})
}

func (h *Handler) PatchScan(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("scanId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid scanId")
		return
	}
	currentScan, err := h.repo.GetScan(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrPatrolScanNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol scan not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol scan")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, currentScan.PlaceID) {
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var patrolRunID, spotID, scannedAt, submitAt, photoURL, note *string
	if raw, exists := body["patrolRunId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "patrolRunId must be valid UUID")
			return
		}
		v := strings.TrimSpace(value)
		patrolRunID = &v
	}
	if raw, exists := body["spotId"]; exists {
		value, ok := raw.(string)
		if !ok || !web.IsUUID(strings.TrimSpace(value)) {
			web.WriteError(w, http.StatusBadRequest, "spotId must be valid UUID")
			return
		}
		v := strings.TrimSpace(value)
		spotID = &v
	}
	if raw, exists := body["scannedAt"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "scannedAt must be string")
			return
		}
		v := strings.TrimSpace(value)
		scannedAt = &v
	}
	if raw, exists := body["submitAt"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			web.WriteError(w, http.StatusBadRequest, "submitAt must be string")
			return
		}
		v := strings.TrimSpace(value)
		submitAt = &v
	}
	if raw, exists := body["photoUrl"]; exists {
		switch value := raw.(type) {
		case string:
			v := value
			photoURL = &v
		case nil:
			v := ""
			photoURL = &v
		default:
			web.WriteError(w, http.StatusBadRequest, "photoUrl must be string or null")
			return
		}
	}
	if raw, exists := body["note"]; exists {
		switch value := raw.(type) {
		case string:
			v := value
			note = &v
		case nil:
			v := ""
			note = &v
		default:
			web.WriteError(w, http.StatusBadRequest, "note must be string or null")
			return
		}
	}
	item, err := h.repo.UpdateScan(r.Context(), id, patrolRunID, spotID, scannedAt, submitAt, photoURL, note)
	if err != nil {
		switch {
		case errors.Is(err, ErrPatrolScanNotFound):
			web.WriteError(w, http.StatusNotFound, "Patrol scan not found")
		case errors.Is(err, ErrPatrolRunNotFound):
			web.WriteError(w, http.StatusBadRequest, "Target patrol run not found")
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Patrol scan already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related patrol run or spot not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update patrol scan")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteScan(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("scanId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid scanId")
		return
	}
	currentScan, err := h.repo.GetScan(r.Context(), current.UserID, current.Role, id)
	if err != nil {
		if errors.Is(err, ErrPatrolScanNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol scan not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol scan")
		return
	}
	if !h.requirePlaceAdminAccess(w, r, currentScan.PlaceID) {
		return
	}
	out, err := h.repo.DeleteScan(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPatrolScanNotFound) {
			web.WriteError(w, http.StatusNotFound, "Patrol scan not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete patrol scan")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
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
