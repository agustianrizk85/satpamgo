package leaverequests

import (
	"errors"
	"net/http"
	"strings"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
	"satpam-go/internal/web"
)

type Handler struct{ repo *Repository }

func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	query, message, ok := listquery.Parse(r, listquery.Options{
		AllowedSortBy: []string{"createdAt", "updatedAt", "startDate", "status", "userId", "placeId"},
		DefaultSortBy: "createdAt",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	placeID := strings.TrimSpace(r.URL.Query().Get("placeId"))
	if placeID == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId is required")
		return
	}
	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID: current.UserID,
		ActorRole:   current.Role,
		PlaceID:     placeID,
		UserID:      strings.TrimSpace(r.URL.Query().Get("userId")),
		Status:      normalizeRequestStatus(r.URL.Query().Get("status")),
		Query:       query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load leave requests")
		return
	}
	web.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	var body struct {
		PlaceID      string  `json:"placeId"`
		UserID       string  `json:"userId"`
		AssignmentID *string `json:"assignmentId"`
		LeaveType    string  `json:"leaveType"`
		StartDate    string  `json:"startDate"`
		EndDate      *string `json:"endDate"`
		Reason       *string `json:"reason"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.UserID = strings.TrimSpace(body.UserID)
	body.StartDate = strings.TrimSpace(body.StartDate)
	leaveType := normalizeLeaveType(body.LeaveType)
	if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.UserID) || body.StartDate == "" || leaveType == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, leaveType, and startDate are required")
		return
	}
	id, err := h.repo.Create(r.Context(), CreateInput{
		PlaceID:      body.PlaceID,
		UserID:       body.UserID,
		AssignmentID: trimUUIDPtr(body.AssignmentID),
		LeaveType:    leaveType,
		StartDate:    body.StartDate,
		EndDate:      trimStringPtr(body.EndDate),
		Reason:       trimStringPtr(body.Reason),
	})
	if err != nil {
		if errors.Is(err, ErrForeignKey) {
			web.WriteError(w, http.StatusBadRequest, "Related row not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to create leave request")
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
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
		ID      string `json:"id"`
		PlaceID string `json:"placeId"`
		Status  string `json:"status"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.ID = strings.TrimSpace(body.ID)
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	status := normalizeRequestStatus(body.Status)
	if !web.IsUUID(body.ID) || !web.IsUUID(body.PlaceID) || status == "" {
		web.WriteError(w, http.StatusBadRequest, "id, placeId, and status are required")
		return
	}
	id, err := h.repo.UpdateStatus(r.Context(), UpdateStatusInput{ID: body.ID, PlaceID: body.PlaceID, Status: status})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Leave request not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to update leave request")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}

func normalizeLeaveType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SICK", "LEAVE":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeRequestStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "":
		return ""
	case "PENDING", "APPROVED", "REJECTED", "CANCELLED":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
}

func trimUUIDPtr(value *string) *string {
	value = trimStringPtr(value)
	if value == nil || !web.IsUUID(*value) {
		return nil
	}
	return value
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
