package attendances

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
		AllowedSortBy: []string{"attendanceDate", "createdAt", "updatedAt", "checkInAt", "checkOutAt", "submitAt", "status", "userId", "placeId"},
		DefaultSortBy: "attendanceDate",
	})
	if !ok {
		web.WriteError(w, http.StatusBadRequest, message)
		return
	}
	result, err := h.repo.List(r.Context(), ListParams{
		ActorUserID:    current.UserID,
		ActorRole:      current.Role,
		PlaceID:        strings.TrimSpace(r.URL.Query().Get("placeId")),
		UserID:         strings.TrimSpace(r.URL.Query().Get("userId")),
		AttendanceDate: strings.TrimSpace(r.URL.Query().Get("attendanceDate")),
		Query:          query,
	})
	if err != nil {
		web.WriteError(w, http.StatusInternalServerError, "Failed to load attendances")
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
		PlaceID          string  `json:"placeId"`
		UserID           string  `json:"userId"`
		AssignmentID     *string `json:"assignmentId"`
		ShiftID          *string `json:"shiftId"`
		AttendanceDate   string  `json:"attendanceDate"`
		CheckInAt        *string `json:"checkInAt"`
		CheckOutAt       *string `json:"checkOutAt"`
		SubmitAt         *string `json:"submitAt"`
		PhotoURL         *string `json:"photoUrl"`
		CheckInPhotoURL  *string `json:"checkInPhotoUrl"`
		CheckOutPhotoURL *string `json:"checkOutPhotoUrl"`
		Status           string  `json:"status"`
		Note             *string `json:"note"`
	}
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	body.PlaceID = strings.TrimSpace(body.PlaceID)
	body.UserID = strings.TrimSpace(body.UserID)
	body.AttendanceDate = strings.TrimSpace(body.AttendanceDate)
	status := normalizeStatus(body.Status)
	if !web.IsUUID(body.PlaceID) || !web.IsUUID(body.UserID) || body.AttendanceDate == "" || status == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, attendanceDate are required and status is invalid")
		return
	}
	id, err := h.repo.Create(r.Context(), CreateInput{
		PlaceID:          body.PlaceID,
		UserID:           body.UserID,
		AssignmentID:     trimUUIDPtr(body.AssignmentID),
		ShiftID:          trimUUIDPtr(body.ShiftID),
		AttendanceDate:   body.AttendanceDate,
		CheckInAt:        trimStringPtr(body.CheckInAt),
		CheckOutAt:       trimStringPtr(body.CheckOutAt),
		SubmitAt:         trimStringPtr(body.SubmitAt),
		PhotoURL:         trimStringPtr(body.PhotoURL),
		CheckInPhotoURL:  trimStringPtr(body.CheckInPhotoURL),
		CheckOutPhotoURL: trimStringPtr(body.CheckOutPhotoURL),
		Status:           status,
		Note:             trimStringPtr(body.Note),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadyExists):
			web.WriteError(w, http.StatusConflict, "Attendance already exists")
		case errors.Is(err, ErrForeignKey):
			web.WriteError(w, http.StatusBadRequest, "Related row not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to create attendance")
		}
		return
	}
	web.WriteJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("attendanceId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid attendanceId")
		return
	}
	var body map[string]any
	if err := web.DecodeJSON(r, &body); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid body")
		return
	}
	var input UpdateInput
	if raw, exists := body["checkInAt"]; exists {
		input.CheckInAt = parseNullableString(raw)
		if input.CheckInAt == nil {
			web.WriteError(w, http.StatusBadRequest, "checkInAt must be string or null")
			return
		}
	}
	if raw, exists := body["checkOutAt"]; exists {
		input.CheckOutAt = parseNullableString(raw)
		if input.CheckOutAt == nil {
			web.WriteError(w, http.StatusBadRequest, "checkOutAt must be string or null")
			return
		}
	}
	if raw, exists := body["submitAt"]; exists {
		input.SubmitAt = parseNullableString(raw)
		if input.SubmitAt == nil {
			web.WriteError(w, http.StatusBadRequest, "submitAt must be string or null")
			return
		}
	}
	if raw, exists := body["photoUrl"]; exists {
		input.PhotoURL = parseNullableString(raw)
		if input.PhotoURL == nil {
			web.WriteError(w, http.StatusBadRequest, "photoUrl must be string or null")
			return
		}
	}
	if raw, exists := body["checkInPhotoUrl"]; exists {
		input.CheckInPhotoURL = parseNullableString(raw)
		if input.CheckInPhotoURL == nil {
			web.WriteError(w, http.StatusBadRequest, "checkInPhotoUrl must be string or null")
			return
		}
	}
	if raw, exists := body["checkOutPhotoUrl"]; exists {
		input.CheckOutPhotoURL = parseNullableString(raw)
		if input.CheckOutPhotoURL == nil {
			web.WriteError(w, http.StatusBadRequest, "checkOutPhotoUrl must be string or null")
			return
		}
	}
	if raw, exists := body["status"]; exists {
		value, ok := raw.(string)
		if !ok {
			web.WriteError(w, http.StatusBadRequest, "status must be string")
			return
		}
		value = normalizeStatus(value)
		if value == "" {
			web.WriteError(w, http.StatusBadRequest, "Invalid status")
			return
		}
		input.Status = &value
	}
	if raw, exists := body["note"]; exists {
		input.Note = parseNullableString(raw)
		if input.Note == nil {
			web.WriteError(w, http.StatusBadRequest, "note must be string or null")
			return
		}
	}
	item, err := h.repo.Update(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoFieldsToUpdate):
			web.WriteError(w, http.StatusBadRequest, "No fields to update")
		case errors.Is(err, ErrNotFound):
			web.WriteError(w, http.StatusNotFound, "Attendance not found")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to update attendance")
		}
		return
	}
	web.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AuthFromContext(r.Context()); !ok {
		web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
		return
	}
	id := r.PathValue("attendanceId")
	if !web.IsUUID(id) {
		web.WriteError(w, http.StatusBadRequest, "Invalid attendanceId")
		return
	}
	out, err := h.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.WriteError(w, http.StatusNotFound, "Attendance not found")
			return
		}
		web.WriteError(w, http.StatusInternalServerError, "Failed to delete attendance")
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]string{"id": out})
}

func normalizeStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "PRESENT":
		return "PRESENT"
	case "LATE", "ABSENT", "OFF", "SICK", "LEAVE":
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

func parseNullableString(raw any) **string {
	if raw == nil {
		var out *string
		return &out
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	out := &trimmed
	return &out
}
