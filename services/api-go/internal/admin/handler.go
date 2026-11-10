package admin

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"lokasolve/services/api-go/internal/auth"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/validate"
)

type Handler struct {
	Service *Service
}

func (h Handler) Routes(r chi.Router) {
	r.Get("/admin/users", h.users)
	r.Post("/admin/users/{userID}/roles", h.grant)
	r.Get("/admin/audit-logs", h.audit)
}

func (h Handler) users(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	page, err := h.Service.Users(r.Context(), principal, r.URL.Query().Get("cursor"), queryLimit(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func (h Handler) grant(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	userID, message := validate.ID(chi.URLParam(r, "userID"))
	if message != "" {
		httpx.WriteError(w, r, httpx.New(http.StatusNotFound, "not_found", "That record was not found."))
		return
	}
	var req struct {
		RoleCode string `json:"role_code"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.GrantRole(r.Context(), principal, userID, req.RoleCode, httpx.RequestID(r.Context())); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) audit(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	page, err := h.Service.Audit(r.Context(), principal, r.URL.Query().Get("cursor"), queryLimit(r))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func queryLimit(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return value
}
