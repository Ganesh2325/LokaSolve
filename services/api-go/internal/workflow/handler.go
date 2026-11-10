package workflow

import (
	"fmt"
	"io"
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
	r.Get("/nearby", h.nearby)
	r.Get("/notifications", h.notices)
	r.Post("/notifications/{notificationID}/read", h.read)
	r.Get("/assignments", h.assignments)
	r.Post("/assignments/{assignmentID}/transition", h.transition)
	r.Post("/assignments/{assignmentID}/quote", h.quote)
	r.Post("/assignments/{assignmentID}/approve-quote", h.approveQuote)
	r.Get("/issues/{issueID}/timeline", h.timeline)
	r.Get("/issues/{issueID}/media", h.listMedia)
	r.Post("/issues/{issueID}/media", h.saveMedia)
	r.Get("/issues/{issueID}/media/{mediaID}", h.openMedia)
	r.Post("/issues/{issueID}/classification", h.classify)
	r.Post("/issues/{issueID}/incident", h.incident)
	r.Get("/issues/{issueID}/eligible", h.eligible)
	r.Post("/issues/{issueID}/assignments", h.offer)
	r.Post("/issues/{issueID}/actions", h.action)
	r.Get("/admin/reviews", h.reviews)
	r.Post("/admin/reviews/{reviewID}/resolve", h.resolveReview)
	r.Post("/admin/resolvers", h.addResolver)
	r.Get("/admin/analytics", h.analytics)
}

func (h Handler) nearby(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.Nearby(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) notices(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.Notices(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) read(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "notificationID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That notification id is not valid."))
		return
	}
	if err := h.Service.MarkRead(r.Context(), principal, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) assignments(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.ListAssignments(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) transition(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "assignmentID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That assignment id is not valid."))
		return
	}
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.Transition(r.Context(), principal, id, req.Status, req.Note, httpx.RequestID(r.Context())); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) quote(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "assignmentID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That assignment id is not valid."))
		return
	}
	var req struct {
		QuoteText string `json:"quote_text"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.SetQuote(r.Context(), principal, id, req.QuoteText); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) approveQuote(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "assignmentID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That assignment id is not valid."))
		return
	}
	if err := h.Service.ApproveQuote(r.Context(), principal, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) timeline(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.Service.Timeline(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) listMedia(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.Service.ListMedia(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) saveMedia(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "The upload could not be read."))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "Choose a photo, audio, or video file."))
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 4_000_001))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	item, err := h.Service.SaveMedia(r.Context(), principal, id, header.Filename, header.Header.Get("Content-Type"), body)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) openMedia(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	issueID, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	mediaID, err := parseID(r, "mediaID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	contentType, fileName, scan, body, err := h.Service.OpenMedia(r.Context(), principal, issueID, mediaID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Malware-Scan", scan)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h Handler) classify(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var req struct {
		CategoryLabel string `json:"category_label"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.Classify(r.Context(), principal, id, req.CategoryLabel, httpx.RequestID(r.Context())); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) incident(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	incidentID, err := h.Service.CreateIncident(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"id": incidentID})
}

func (h Handler) eligible(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.Service.Eligible(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) offer(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	issueID, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var req struct {
		ResolverID string `json:"resolver_id"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	resolverID, err := uuid.Parse(strings.TrimSpace(req.ResolverID))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "Choose a responder."))
		return
	}
	item, err := h.Service.Offer(r.Context(), principal, issueID, resolverID, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) action(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := parseID(r, "issueID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var req struct {
		TemplateName      string `json:"template_name"`
		ExternalReference string `json:"external_reference"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.DigitalAction(r.Context(), principal, id, req.TemplateName, req.ExternalReference); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) reviews(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.Reviews(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) resolveReview(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "reviewID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That review id is not valid."))
		return
	}
	if err := h.Service.ResolveReview(r.Context(), principal, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) addResolver(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	var req struct {
		DisplayName string `json:"display_name"`
		Kind        string `json:"kind"`
		Locality    string `json:"locality_text"`
		Skills      string `json:"skills"`
		OwnerEmail  string `json:"owner_email"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	item, err := h.Service.AddResolver(r.Context(), principal, req.DisplayName, req.Kind, req.Locality, req.Skills, req.OwnerEmail, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) analytics(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	stats, err := h.Service.Measure(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, stats)
}

func parseID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.UUID{}, httpx.New(400, "validation_error", "That id is not valid.")
	}
	return id, nil
}
