package reports

import (
	"context"
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

func (h *Handler) ListAttendance(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"attendanceDate", "checkInAt", "checkOutAt", "status", "lateMinutes", "userName", "placeName", "createdAt"},
		DefaultSortBy: "attendanceDate",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	filters, ok := h.parseAttendanceFilters(w, r, current)
	if !ok {
		return
	}
	resp, summary, err := h.repo.ListAttendance(r.Context(), filters, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load attendance report")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       resp.Data,
		"pagination": resp.Pagination,
		"sort":       resp.Sort,
		"summary":    summary,
	})
}

func (h *Handler) DownloadAttendance(w http.ResponseWriter, r *http.Request) {
	h.downloadAttendance(w, r)
}

func (h *Handler) ListPatrolScans(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"scannedAt", "patrolRunId", "userName", "placeName", "spotName"},
		DefaultSortBy: "scannedAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	filters, ok := h.parsePatrolFilters(w, r, current)
	if !ok {
		return
	}
	resp, summary, err := h.repo.ListPatrolScans(r.Context(), filters, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load patrol scan report")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       resp.Data,
		"pagination": resp.Pagination,
		"sort":       resp.Sort,
		"summary":    summary,
	})
}

func (h *Handler) DownloadPatrolScans(w http.ResponseWriter, r *http.Request) {
	h.downloadPatrolScans(w, r)
}

func (h *Handler) ListFacilityScans(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"scannedAt", "status", "userName", "placeName", "spotName", "itemName", "createdAt"},
		DefaultSortBy: "scannedAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	filters, ok := h.parseFacilityFilters(w, r, current)
	if !ok {
		return
	}
	resp, summary, err := h.repo.ListFacilityScans(r.Context(), filters, query)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load facility scan report")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       resp.Data,
		"pagination": resp.Pagination,
		"sort":       resp.Sort,
		"summary":    summary,
	})
}

func (h *Handler) DownloadFacilityScans(w http.ResponseWriter, r *http.Request) {
	h.downloadFacilityScans(w, r)
}

func (h *Handler) parseAttendanceFilters(w http.ResponseWriter, r *http.Request, current auth.AuthContext) (AttendanceFilters, bool) {
	filters := AttendanceFilters{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     strings.TrimSpace(r.URL.Query().Get("placeId")),
		UserID:      strings.TrimSpace(r.URL.Query().Get("userId")),
		Status:      strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))),
		FromDate:    strings.TrimSpace(r.URL.Query().Get("fromDate")),
		ToDate:      strings.TrimSpace(r.URL.Query().Get("toDate")),
	}
	if !h.validateCommonFilters(r.Context(), w, current, filters.PlaceID, filters.UserID, filters.FromDate, filters.ToDate) {
		return AttendanceFilters{}, false
	}
	if filters.Status != "" && !isAttendanceStatus(filters.Status) {
		web.WriteError(w, http.StatusBadRequest, "Invalid status")
		return AttendanceFilters{}, false
	}
	return filters, true
}

func (h *Handler) parsePatrolFilters(w http.ResponseWriter, r *http.Request, current auth.AuthContext) (PatrolScanFilters, bool) {
	filters := PatrolScanFilters{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     strings.TrimSpace(r.URL.Query().Get("placeId")),
		UserID:      strings.TrimSpace(r.URL.Query().Get("userId")),
		SpotID:      strings.TrimSpace(r.URL.Query().Get("spotId")),
		PatrolRunID: strings.TrimSpace(r.URL.Query().Get("patrolRunId")),
		FromDate:    strings.TrimSpace(r.URL.Query().Get("fromDate")),
		ToDate:      strings.TrimSpace(r.URL.Query().Get("toDate")),
	}
	if !h.validateCommonFilters(r.Context(), w, current, filters.PlaceID, filters.UserID, filters.FromDate, filters.ToDate) {
		return PatrolScanFilters{}, false
	}
	if filters.SpotID != "" && !web.IsUUID(filters.SpotID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return PatrolScanFilters{}, false
	}
	return filters, true
}

func (h *Handler) parseFacilityFilters(w http.ResponseWriter, r *http.Request, current auth.AuthContext) (FacilityScanFilters, bool) {
	filters := FacilityScanFilters{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     strings.TrimSpace(r.URL.Query().Get("placeId")),
		UserID:      strings.TrimSpace(r.URL.Query().Get("userId")),
		SpotID:      strings.TrimSpace(r.URL.Query().Get("spotId")),
		ItemID:      strings.TrimSpace(r.URL.Query().Get("itemId")),
		Status:      strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))),
		FromDate:    strings.TrimSpace(r.URL.Query().Get("fromDate")),
		ToDate:      strings.TrimSpace(r.URL.Query().Get("toDate")),
	}
	if !h.validateCommonFilters(r.Context(), w, current, filters.PlaceID, filters.UserID, filters.FromDate, filters.ToDate) {
		return FacilityScanFilters{}, false
	}
	if filters.SpotID != "" && !web.IsUUID(filters.SpotID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid spotId")
		return FacilityScanFilters{}, false
	}
	if filters.ItemID != "" && !web.IsUUID(filters.ItemID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid itemId")
		return FacilityScanFilters{}, false
	}
	if filters.Status != "" && !isFacilityStatus(filters.Status) {
		web.WriteError(w, http.StatusBadRequest, "Invalid status")
		return FacilityScanFilters{}, false
	}
	return filters, true
}

func (h *Handler) validateCommonFilters(ctx context.Context, w http.ResponseWriter, current auth.AuthContext, placeID, userID, fromDate, toDate string) bool {
	if placeID != "" {
		if !web.IsUUID(placeID) {
			web.WriteError(w, http.StatusBadRequest, "Invalid placeId")
			return false
		}
		if !auth.IsGlobalAdminRole(current.Role) {
			ok, err := h.authRepo.HasPlaceAccess(ctx, current.UserID, placeID, nil)
			if err != nil {
				web.WriteError(w, http.StatusInternalServerError, "Failed to validate access")
				return false
			}
			if !ok {
				web.WriteError(w, http.StatusForbidden, "Forbidden: no access to this place")
				return false
			}
		}
	}
	if userID != "" && !web.IsUUID(userID) {
		web.WriteError(w, http.StatusBadRequest, "Invalid userId")
		return false
	}
	if fromDate != "" && !isDateOnly(fromDate) {
		web.WriteError(w, http.StatusBadRequest, "fromDate must use YYYY-MM-DD")
		return false
	}
	if toDate != "" && !isDateOnly(toDate) {
		web.WriteError(w, http.StatusBadRequest, "toDate must use YYYY-MM-DD")
		return false
	}
	if fromDate != "" && toDate != "" && fromDate > toDate {
		web.WriteError(w, http.StatusBadRequest, "fromDate cannot be greater than toDate")
		return false
	}
	return true
}

func (h *Handler) downloadAttendance(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	filters, ok := h.parseAttendanceFilters(w, r, current)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}
	rows, summary, err := h.repo.DownloadAttendance(r.Context(), filters, "attendanceDate", listquery.SortDesc)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to generate attendance report")
		return
	}
	headers := []string{"Attendance Date", "Place", "User", "Shift", "Status", "Late Minutes", "Check In", "Check Out", "Check In Photo", "Check Out Photo", "Note", "Created At", "Updated At"}
	body := make([][]string, 0, len(rows))
	for _, row := range rows {
		body = append(body, []string{row.AttendanceDate, row.PlaceName, row.FullName, deref(row.ShiftName), row.Status, derefInt(row.LateMinutes), deref(row.CheckInAt), deref(row.CheckOutAt), deref(row.CheckInPhotoURL), deref(row.CheckOutPhotoURL), deref(row.Note), row.CreatedAt, row.UpdatedAt})
	}
	summaryLines := []string{
		"Total Data: " + stringifyInt(summary.TotalData),
		"Present: " + stringifyInt(summary.PresentCount),
		"Late: " + stringifyInt(summary.LateCount),
		"Absent: " + stringifyInt(summary.AbsentCount),
		"Off: " + stringifyInt(summary.OffCount),
		"Sick: " + stringifyInt(summary.SickCount),
		"Leave: " + stringifyInt(summary.LeaveCount),
	}
	h.writeDownload(w, format, "attendance-report", "Attendance Report", headers, body, summaryLines)
}

func (h *Handler) downloadPatrolScans(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	filters, ok := h.parsePatrolFilters(w, r, current)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}
	rows, summary, err := h.repo.DownloadPatrolScans(r.Context(), filters, "scannedAt", listquery.SortDesc)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to generate patrol scan report")
		return
	}
	headers := []string{"Scanned At", "Place", "User", "Spot Code", "Spot Name", "Patrol Run ID", "Photo URL", "Note"}
	body := make([][]string, 0, len(rows))
	for _, row := range rows {
		body = append(body, []string{row.ScannedAt, row.PlaceName, row.FullName, row.SpotCode, row.SpotName, row.PatrolRunID, deref(row.PhotoURL), deref(row.Note)})
	}
	summaryLines := []string{
		"Total Data: " + stringifyInt(summary.TotalData),
		"Unique Patrol Runs: " + stringifyInt(summary.UniquePatrolRuns),
		"Unique Spots: " + stringifyInt(summary.UniqueSpots),
		"Unique Users: " + stringifyInt(summary.UniqueUsers),
	}
	h.writeDownload(w, format, "patrol-scan-report", "Patrol Scan Report", headers, body, summaryLines)
}

func (h *Handler) downloadFacilityScans(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	filters, ok := h.parseFacilityFilters(w, r, current)
	if !ok {
		return
	}
	rows, summary, err := h.repo.DownloadFacilityScans(r.Context(), filters, "scannedAt", listquery.SortDesc)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to generate facility scan report")
		return
	}
	headers := []string{"Scanned At", "Place", "User", "Spot Code", "Spot Name", "Item Name", "Status", "Note", "Created At", "Updated At"}
	body := make([][]string, 0, len(rows))
	for _, row := range rows {
		body = append(body, []string{row.ScannedAt, row.PlaceName, row.FullName, row.SpotCode, row.SpotName, deref(row.ItemName), row.Status, deref(row.Note), row.CreatedAt, row.UpdatedAt})
	}
	summaryLines := []string{
		"Total Data: " + stringifyInt(summary.TotalData),
		"OK: " + stringifyInt(summary.OKCount),
		"NOT_OK: " + stringifyInt(summary.NotOKCount),
		"PARTIAL: " + stringifyInt(summary.PartialCount),
		"Unique Spots: " + stringifyInt(summary.UniqueSpots),
		"Unique Items: " + stringifyInt(summary.UniqueItems),
		"Unique Users: " + stringifyInt(summary.UniqueUsers),
	}
	h.writeDownload(w, "csv", "facility-scan-report", "Facility Scan Report", headers, body, summaryLines)
}

func (h *Handler) writeDownload(w http.ResponseWriter, format, baseName, title string, headers []string, body [][]string, summary []string) {
	if format == "pdf" {
		content, err := renderSimplePDF(title, headers, body, summary)
		if err != nil {
			web.WriteError(w, http.StatusInternalServerError, "Failed to generate PDF report")
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename(baseName, "pdf")+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}
	content, err := renderCSV(headers, body)
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to generate CSV report")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename(baseName, "csv")+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefInt(value *int) string {
	if value == nil {
		return ""
	}
	return stringifyInt(*value)
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

func isAttendanceStatus(v string) bool {
	switch v {
	case "PRESENT", "LATE", "ABSENT", "OFF", "SICK", "LEAVE":
		return true
	default:
		return false
	}
}

func isFacilityStatus(v string) bool {
	switch v {
	case "OK", "NOT_OK", "PARTIAL":
		return true
	default:
		return false
	}
}
