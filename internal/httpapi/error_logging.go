package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"satpam-go/internal/apierrorlogs"
	"satpam-go/internal/auth"
	"satpam-go/internal/web"
)

const (
	maxLoggedBodyBytes     = 64 * 1024
	maxLoggedResponseBytes = 16 * 1024
	maxLoggedTextLength    = 4000
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len() < maxLoggedResponseBytes {
		remaining := maxLoggedResponseBytes - w.body.Len()
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.body.Write(p[:remaining])
	}
	return w.ResponseWriter.Write(p)
}

func loggingMiddleware(tokenService *auth.TokenService, errorLogRepo *apierrorlogs.Repository, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldCaptureAPIErrorLog(r) {
			next.ServeHTTP(w, r)
			return
		}

		requestBodyText := captureRequestBody(r)
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		if errorLogRepo == nil || recorder.status < 400 {
			return
		}

		userID, userRole := parseAuthFromHeader(tokenService, r.Header.Get("Authorization"))
		queryMap := requestQueryMap(r)
		placeID := detectPlaceID(r, queryMap, requestBodyText)
		message := detectResponseMessage(recorder.body.Bytes())
		responseBodyText := trimLoggedText(string(recorder.body.Bytes()))
		clientIP := clientIPFromRequest(r)
		userAgent := strings.TrimSpace(r.UserAgent())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := errorLogRepo.Insert(ctx, apierrorlogs.CreateParams{
			OccurredAt:   time.Now(),
			Method:       r.Method,
			Path:         r.URL.Path,
			StatusCode:   recorder.status,
			Message:      stringPtr(message),
			PlaceID:      placeID,
			UserID:       stringPtr(userID),
			UserRole:     stringPtr(userRole),
			ClientIP:     stringPtr(clientIP),
			UserAgent:    stringPtr(userAgent),
			RequestQuery: queryMap,
			RequestBody:  stringPtr(requestBodyText),
			ResponseBody: stringPtr(responseBodyText),
		})
		if err != nil {
			log.Printf("api error log insert: %v", err)
		}
	})
}

func shouldCaptureAPIErrorLog(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/uploads/") {
		return false
	}
	return true
}

func captureRequestBody(r *http.Request) string {
	if r == nil || r.Body == nil {
		return ""
	}
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return "<<multipart body omitted>>"
	}

	limited := io.LimitReader(r.Body, maxLoggedBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return ""
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))

	truncated := len(raw) > maxLoggedBodyBytes
	if truncated {
		raw = raw[:maxLoggedBodyBytes]
	}

	text := sanitizeBodyText(raw, contentType)
	if truncated {
		text += " <<truncated>>"
	}
	return trimLoggedText(text)
}

func sanitizeBodyText(raw []byte, contentType string) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	if strings.Contains(contentType, "application/json") || strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		var payload any
		if err := json.Unmarshal(raw, &payload); err == nil {
			sanitizeJSONValue("", payload)
			out, err := json.Marshal(payload)
			if err == nil {
				return string(out)
			}
		}
	}
	return text
}

func sanitizeJSONValue(key string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			lowerKey := strings.ToLower(strings.TrimSpace(childKey))
			switch lowerKey {
			case "password", "passwordhash", "token", "accesstoken", "authorization":
				typed[childKey] = "***redacted***"
				continue
			case "photourl", "photo", "file", "image":
				if rawText, ok := childValue.(string); ok && rawText != "" {
					typed[childKey] = "<<omitted bulky field>>"
					continue
				}
			}
			sanitizeJSONValue(lowerKey, childValue)
		}
	case []any:
		for i := range typed {
			sanitizeJSONValue(key, typed[i])
		}
	case string:
		if strings.EqualFold(key, "photourl") || strings.EqualFold(key, "photo") || strings.EqualFold(key, "image") || strings.EqualFold(key, "file") {
			if typed != "" {
				switch parent := value.(type) {
				case string:
					_ = parent
				}
			}
		}
	}
}

func parseAuthFromHeader(tokenService *auth.TokenService, header string) (string, string) {
	if tokenService == nil || strings.TrimSpace(header) == "" {
		return "", ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", ""
	}
	claims, err := tokenService.Verify(parts[1])
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(claims.UserID), strings.TrimSpace(claims.Role)
}

func requestQueryMap(r *http.Request) map[string]any {
	out := map[string]any{}
	for key, values := range r.URL.Query() {
		if len(values) == 1 {
			out[key] = values[0]
			continue
		}
		copied := make([]string, len(values))
		copy(copied, values)
		out[key] = copied
	}
	return out
}

func detectPlaceID(r *http.Request, queryMap map[string]any, requestBodyText string) *string {
	if raw, ok := queryMap["placeId"].(string); ok && web.IsUUID(strings.TrimSpace(raw)) {
		trimmed := strings.TrimSpace(raw)
		return &trimmed
	}
	if requestBodyText == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(requestBodyText), &payload); err != nil {
		return nil
	}
	raw, ok := payload["placeId"].(string)
	if !ok || !web.IsUUID(strings.TrimSpace(raw)) {
		return nil
	}
	trimmed := strings.TrimSpace(raw)
	return &trimmed
}

func detectResponseMessage(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if message, ok := payload["message"].(string); ok {
			return trimLoggedText(message)
		}
		if message, ok := payload["error"].(string); ok {
			return trimLoggedText(message)
		}
	}
	return trimLoggedText(text)
}

func clientIPFromRequest(r *http.Request) string {
	forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if forwarded != "" {
		return forwarded
	}
	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func trimLoggedText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > maxLoggedTextLength {
		return trimmed[:maxLoggedTextLength] + " <<truncated>>"
	}
	return trimmed
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
