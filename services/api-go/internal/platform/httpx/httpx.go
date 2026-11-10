package httpx

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func Validation(fields map[string]string) *Error {
	clean := map[string]string{}
	for key, value := range fields {
		if value != "" {
			clean[key] = value
		}
	}
	return &Error{Status: 400, Code: "validation_error", Message: "Check the highlighted fields.", Fields: clean}
}

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type errorBody struct {
	Error struct {
		Code    string       `json:"code"`
		Message string       `json:"message"`
		Details []fieldError `json:"details,omitempty"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *Error
	if !errors.As(err, &appErr) || appErr == nil {
		slog.Error("request failed", "request_id", RequestID(r.Context()), "error", err.Error())
		appErr = New(http.StatusInternalServerError, "internal_error", "Something went wrong. Try again.")
	}
	body := errorBody{RequestID: RequestID(r.Context())}
	body.Error.Code = appErr.Code
	body.Error.Message = appErr.Message
	for field, message := range appErr.Fields {
		body.Error.Details = append(body.Error.Details, fieldError{Field: field, Message: message})
	}
	WriteJSON(w, appErr.Status, body)
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return New(http.StatusBadRequest, "validation_error", "The request body could not be read.")
	}
	return nil
}

type ctxKey int

const requestIDKey ctxKey = 1

func WithRequestID(ctx httpContext, id string) httpContext {
	return contextWith(ctx, id)
}

func RequestID(ctx httpContext) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

const RefreshCookie = "lokasolve_refresh"

func SetRefreshCookie(w http.ResponseWriter, secure bool, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookie,
		Value:    token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func ClearRefreshCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookie,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type cursorPayload struct {
	T time.Time `json:"t"`
	I string    `json:"i"`
}

func EncodeCursor(t time.Time, id uuid.UUID) string {
	raw, _ := json.Marshal(cursorPayload{T: t.UTC(), I: id.String()})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeCursor(value string) (time.Time, uuid.UUID, error) {
	if value == "" {
		return time.Time{}, uuid.Nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, uuid.Nil, New(http.StatusBadRequest, "validation_error", "The page cursor is invalid.")
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return time.Time{}, uuid.Nil, New(http.StatusBadRequest, "validation_error", "The page cursor is invalid.")
	}
	id, err := uuid.Parse(payload.I)
	if err != nil {
		return time.Time{}, uuid.Nil, New(http.StatusBadRequest, "validation_error", "The page cursor is invalid.")
	}
	return payload.T, id, nil
}

func Limit(value int) int {
	if value <= 0 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}

func Bearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return ""
	}
	return token
}
