package issues

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/auth"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Handler struct {
	Service *Service
}

func (h Handler) Routes(r chi.Router) {
	r.Post("/issues", h.create)
	r.Get("/issues", h.list)
	r.Get("/issues/{issueID}", h.get)
}

func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	var req struct {
		Description               string            `json:"description"`
		CategoryLabel             *string           `json:"category_label"`
		StatedUrgency             string            `json:"stated_urgency"`
		AddressText               *string           `json:"address_text"`
		LocalityText              *string           `json:"locality_text"`
		Latitude                  *float64          `json:"latitude"`
		Longitude                 *float64          `json:"longitude"`
		IdentityVisibleToResolver bool              `json:"identity_visible_to_resolver"`
		FollowPublic              bool              `json:"follow_public"`
		ProblemTypeID             *string           `json:"problem_type_id"`
		Answers                   map[string]string `json:"answers"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var problemTypeID *uuid.UUID
	if req.ProblemTypeID != nil && strings.TrimSpace(*req.ProblemTypeID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.ProblemTypeID))
		if err != nil {
			httpx.WriteError(w, r, httpx.New(400, "validation_error", "Choose a problem type from the list."))
			return
		}
		problemTypeID = &parsed
	}
	issue, safety, err := h.Service.Create(r.Context(), principal, CreateInput{
		Description: req.Description, CategoryLabel: req.CategoryLabel, StatedUrgency: req.StatedUrgency,
		AddressText: req.AddressText, LocalityText: req.LocalityText, Latitude: req.Latitude, Longitude: req.Longitude,
		IdentityVisibleToResolver: req.IdentityVisibleToResolver, FollowPublic: req.FollowPublic,
		ProblemTypeID: problemTypeID, Answers: req.Answers, RequestID: httpx.RequestID(r.Context()),
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	body := map[string]any{"issue": issue}
	if safety != nil {
		body["safety"] = safety
	}
	httpx.WriteJSON(w, http.StatusCreated, body)
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.List(r.Context(), principal, r.URL.Query().Get("q"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "issueID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That report id is not valid."))
		return
	}
	issue, err := h.Service.Get(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, issue)
}
