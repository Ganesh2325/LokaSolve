package authority

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
	r.Get("/admin/authorities", h.list)
	r.Post("/admin/authorities", h.add)
	r.Post("/admin/authorities/{authorityID}/channels", h.channel)
	r.Post("/admin/authorities/{authorityID}/verification", h.verify)
	r.Get("/admin/referrals", h.referrals)
	r.Get("/issues/{issueID}/authorities", h.match)
	r.Get("/issues/{issueID}/referrals", h.issueReferrals)
	r.Post("/issues/{issueID}/referrals", h.prepare)
	r.Post("/referrals/{referralID}/advance", h.advance)
	r.Post("/referrals/{referralID}/escalate", h.escalate)
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.ListOffices(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) add(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	var req struct {
		OfficialName       string `json:"official_name"`
		OrgType            string `json:"org_type"`
		LocalityText       string `json:"locality_text"`
		Categories         string `json:"categories"`
		VerificationSource string `json:"verification_source"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	item, err := h.Service.AddOffice(r.Context(), principal, req.OfficialName, req.OrgType, req.LocalityText, req.Categories, req.VerificationSource, httpx.RequestID(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) channel(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "authorityID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That office id is not valid."))
		return
	}
	var req struct {
		Channel     string `json:"channel"`
		Destination string `json:"destination"`
		Purpose     string `json:"purpose"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	item, err := h.Service.AddChannel(r.Context(), principal, id, req.Channel, req.Destination, req.Purpose)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) verify(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "authorityID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That office id is not valid."))
		return
	}
	var req struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.Verify(r.Context(), principal, id, req.VerificationStatus); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) referrals(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	items, err := h.Service.ListAll(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) match(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "issueID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That report id is not valid."))
		return
	}
	items, err := h.Service.Match(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) issueReferrals(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "issueID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That report id is not valid."))
		return
	}
	items, err := h.Service.ListForIssue(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h Handler) prepare(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	issueID, err := uuid.Parse(chi.URLParam(r, "issueID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That report id is not valid."))
		return
	}
	var req struct {
		AuthorityID string `json:"authority_id"`
		Consent     bool   `json:"consent"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	authorityID, err := uuid.Parse(strings.TrimSpace(req.AuthorityID))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "Choose an office."))
		return
	}
	item, err := h.Service.Prepare(r.Context(), principal, issueID, authorityID, req.Consent)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h Handler) advance(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "referralID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That referral id is not valid."))
		return
	}
	var req struct {
		Status            string `json:"status"`
		Note              string `json:"note"`
		ExternalReference string `json:"external_reference"`
	}
	if err := httpx.Decode(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.Service.Advance(r.Context(), principal, id, req.Status, req.Note, req.ExternalReference); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) escalate(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.Current(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "referralID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That referral id is not valid."))
		return
	}
	if err := h.Service.Escalate(r.Context(), principal, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
