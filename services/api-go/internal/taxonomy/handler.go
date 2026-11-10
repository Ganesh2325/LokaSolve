package taxonomy

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"lokasolve/services/api-go/internal/auth"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Handler struct {
	Service *Service
}

func (h Handler) Routes(r chi.Router) {
	r.Get("/taxonomy", h.list)
	r.Post("/admin/problem-types", h.add)
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	categories, err := h.Service.List(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": categories})
}

func (h Handler) add(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	var req struct {
		CategoryName string `json:"category_name"`
		Name         string `json:"name"`
		Prompt       string `json:"prompt"`
		FieldLabel   string `json:"field_label"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	created, err := h.Service.AddProblemType(r.Context(), principal, req.CategoryName, req.Name, req.Prompt, req.FieldLabel, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}
