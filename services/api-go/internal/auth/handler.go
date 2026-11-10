package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/views"
)

type Handler struct {
	Service      *Service
	CookieSecure bool
	RefreshTTL   int
}

func (h Handler) Routes(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
	r.Post("/auth/logout", h.logout)
	r.Post("/auth/otp/request", h.requestLoginOTP)
	r.Post("/auth/otp/verify", h.verifyLoginOTP)
}

func (h Handler) register(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, r, httpx.New(http.StatusBadRequest, "validation_error", "The request body could not be read."))
		return
	}
	var req struct {
		Email             string `json:"email"`
		Password          string `json:"password"`
		DisplayName       string `json:"display_name"`
		PreferredLanguage string `json:"preferred_language"`
		AccountKind       string `json:"account_kind"`
		OrganizationName  string `json:"organization_name"`
		OrganizationType  string `json:"organization_type"`
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	sum := sha256.Sum256(body)
	session, err := h.Service.Register(r.Context(), RegisterInput{
		Email: req.Email, Password: req.Password, DisplayName: req.DisplayName,
		PreferredLanguage: req.PreferredLanguage, AccountKind: req.AccountKind,
		OrganizationName: req.OrganizationName, OrganizationType: req.OrganizationType,
	}, hex.EncodeToString(sum[:]), r.Header.Get("Idempotency-Key"), meta(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.writeSession(w, http.StatusCreated, session)
}

func (h Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	session, err := h.Service.Login(r.Context(), req.Email, req.Password, meta(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.writeSession(w, http.StatusOK, session)
}

func (h Handler) refresh(w http.ResponseWriter, r *http.Request) {
	token, err := presentedRefresh(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	session, err := h.Service.Refresh(r.Context(), token, meta(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.writeSession(w, http.StatusOK, session)
}

func (h Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, err := presentedRefresh(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.Logout(r.Context(), token, meta(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.ClearRefreshCookie(w, h.CookieSecure)
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) requestLoginOTP(w http.ResponseWriter, r *http.Request) {
	h.requestOTP(w, r, nil)
}

func (h Handler) RequestPhoneOTP(w http.ResponseWriter, r *http.Request) {
	principal, ok := Current(r.Context())
	if !ok {
		httpx.WriteError(w, r, errInvalidSession)
		return
	}
	h.requestOTP(w, r, &principal.UserID)
}

func (h Handler) requestOTP(w http.ResponseWriter, r *http.Request, userID *uuid.UUID) {
	var req struct {
		Phone   string `json:"phone"`
		Purpose string `json:"purpose"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if userID != nil && req.Purpose == "" {
		req.Purpose = "verify_phone"
	}
	if err := h.Service.RequestOTP(r.Context(), userID, req.Phone, req.Purpose, meta(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "received"})
}

func (h Handler) verifyLoginOTP(w http.ResponseWriter, r *http.Request) {
	h.verifyOTP(w, r, nil)
}

func (h Handler) VerifyPhoneOTP(w http.ResponseWriter, r *http.Request) {
	principal, ok := Current(r.Context())
	if !ok {
		httpx.WriteError(w, r, errInvalidSession)
		return
	}
	h.verifyOTP(w, r, &principal.UserID)
}

func (h Handler) verifyOTP(w http.ResponseWriter, r *http.Request, userID *uuid.UUID) {
	var req struct {
		Phone   string `json:"phone"`
		Purpose string `json:"purpose"`
		Code    string `json:"code"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if userID != nil && req.Purpose == "" {
		req.Purpose = "verify_phone"
	}
	outcome, err := h.Service.VerifyOTP(r.Context(), userID, req.Phone, req.Purpose, req.Code, meta(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if outcome.Session != nil {
		h.writeSession(w, http.StatusOK, *outcome.Session)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcome.User)
}

func (h Handler) writeSession(w http.ResponseWriter, status int, session views.Session) {
	if h.RefreshTTL > 0 {
		httpx.SetRefreshCookie(w, h.CookieSecure, session.RefreshToken, seconds(h.RefreshTTL))
	}
	httpx.WriteJSON(w, status, session)
}

func meta(r *http.Request) Meta {
	return Meta{IP: httpx.ClientIP(r), UserAgent: r.UserAgent(), RequestID: httpx.RequestID(r.Context())}
}

func presentedRefresh(r *http.Request) (string, error) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		_ = httpx.Decode(r, &req)
	}
	cookie, cookieErr := r.Cookie(httpx.RefreshCookie)
	if req.RefreshToken != "" {
		return req.RefreshToken, nil
	}
	if cookieErr == nil && cookie.Value != "" {
		if r.Header.Get("X-LokaSolve-Client") != "web" {
			return "", httpx.New(http.StatusForbidden, "csrf_blocked", "Refresh the page and try again.")
		}
		return cookie.Value, nil
	}
	return "", errInvalidSession
}
