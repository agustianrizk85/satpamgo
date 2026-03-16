package media

import (
	"errors"
	"net/http"
	"strings"

	"satpam-go/internal/web"
)

type Handler struct {
	service        *Service
	uploadMaxBytes int64
}

func NewHandler(service *Service, uploadMaxBytes int64) *Handler {
	return &Handler{service: service, uploadMaxBytes: uploadMaxBytes}
}

func (h *Handler) UploadAttendance(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, "attendance")
}

func (h *Handler) UploadPatrol(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, "patrol")
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request, category string) {
	r.Body = http.MaxBytesReader(w, r.Body, h.uploadMaxBytes)
	if err := r.ParseMultipartForm(h.uploadMaxBytes); err != nil {
		web.WriteError(w, http.StatusBadRequest, "Invalid multipart body or file too large")
		return
	}

	placeID := strings.TrimSpace(r.FormValue("placeId"))
	userID := strings.TrimSpace(r.FormValue("userId"))
	date := strings.TrimSpace(r.FormValue("date"))
	nameHint := strings.TrimSpace(r.FormValue("name"))

	if !web.IsUUID(placeID) || !web.IsUUID(userID) || date == "" {
		web.WriteError(w, http.StatusBadRequest, "placeId, userId, and date are required")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		web.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	result, err := h.service.Save(SaveInput{
		Category: category,
		PlaceID:  placeID,
		UserID:   userID,
		Date:     date,
		NameHint: nameHint,
		File:     file,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidDate):
			web.WriteError(w, http.StatusBadRequest, "date must use YYYY-MM-DD")
		case errors.Is(err, ErrInvalidFileType):
			web.WriteError(w, http.StatusBadRequest, "file must be jpeg or webp")
		default:
			web.WriteError(w, http.StatusInternalServerError, "Failed to store file")
		}
		return
	}

	web.WriteJSON(w, http.StatusCreated, result)
}
