package issuebridge

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/workflow"
)

type Handler struct {
	DB    *database.DB
	Token string
}

func (h Handler) Mount(r chi.Router) {
	r.Get("/internal/v1/issue-grants", h.grants)
	r.Post("/internal/v1/issue-projections", h.project)
	r.Post("/internal/v1/reviews", h.openReview)
	r.Post("/internal/v1/reviews/resolve", h.resolve)
}

func (h Handler) authorized(r *http.Request) bool {
	return h.Token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Legacy-Service-Token")), []byte(h.Token)) == 1
}

func (h Handler) grants(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		httpx.WriteError(w, r, httpx.New(http.StatusUnauthorized, "invalid_session", "Sign in again."))
		return
	}
	userID, err := uuid.Parse(r.URL.Query().Get("user_id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That identifier is not valid."))
		return
	}
	issueID, err := uuid.Parse(r.URL.Query().Get("issue_id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.New(400, "validation_error", "That identifier is not valid."))
		return
	}
	var assignee, jurisdiction bool
	err = h.DB.WithFlags(r.Context(), nil, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(), `SELECT assignee, jurisdiction FROM issue_access_grant($1, $2, $3)`, userID, issueID, r.URL.Query().Get("locality")).Scan(&assignee, &jurisdiction)
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"assignee": assignee, "jurisdiction": jurisdiction})
}

func (h Handler) project(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		httpx.WriteError(w, r, httpx.New(http.StatusUnauthorized, "invalid_session", "Sign in again."))
		return
	}
	var fact struct {
		ID                        uuid.UUID  `json:"id"`
		ReporterID                uuid.UUID  `json:"reporter_user_id"`
		Description               string     `json:"description"`
		Category                  *string    `json:"category_label"`
		Locality                  *string    `json:"locality_text"`
		Address                   *string    `json:"address_text"`
		IdentityVisibleToResolver bool       `json:"identity_visible_to_resolver"`
		Status                    string     `json:"status"`
		FollowPublic              bool       `json:"follow_public"`
		Severity                  string     `json:"severity"`
		SeveritySource            string     `json:"severity_source"`
		SeverityReason            string     `json:"severity_reason"`
		Safety                    *string    `json:"safety_guidance"`
		EscalationDue             *time.Time `json:"escalation_due_at"`
		CreatedAt                 time.Time  `json:"created_at"`
		Latitude                  *float64   `json:"latitude"`
		Longitude                 *float64   `json:"longitude"`
		Provenance                string     `json:"classification_provenance"`
		Suggestion                *string    `json:"suggested_label"`
	}
	if err := httpx.Decode(r, &fact); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if fact.CreatedAt.IsZero() {
		fact.CreatedAt = time.Now().UTC()
	}
	err := h.DB.WithFlags(r.Context(), nil, nil, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `
			INSERT INTO issue_projection (
				id, reporter_user_id, description, category_label, locality_text, address_text,
				identity_visible_to_resolver, status, follow_public, severity, severity_source,
				severity_reason, safety_guidance, escalation_due_at, created_at, updated_at,
				latitude, longitude, classification_provenance, suggested_label
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15, now(), $16,$17,$18,$19)
			ON CONFLICT (id) DO UPDATE SET
				description = EXCLUDED.description, category_label = EXCLUDED.category_label,
				locality_text = EXCLUDED.locality_text, address_text = EXCLUDED.address_text,
				status = EXCLUDED.status, follow_public = EXCLUDED.follow_public,
				severity = EXCLUDED.severity, severity_source = EXCLUDED.severity_source,
				severity_reason = EXCLUDED.severity_reason, safety_guidance = EXCLUDED.safety_guidance,
				escalation_due_at = EXCLUDED.escalation_due_at,
				latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude,
				classification_provenance = EXCLUDED.classification_provenance,
				suggested_label = EXCLUDED.suggested_label, updated_at = now()
		`, fact.ID, fact.ReporterID, fact.Description, fact.Category, fact.Locality, fact.Address,
			fact.IdentityVisibleToResolver, fact.Status, fact.FollowPublic, empty(fact.Severity, "P3"), empty(fact.SeveritySource, "unclassified"),
			fact.SeverityReason, fact.Safety, fact.EscalationDue, fact.CreatedAt,
			fact.Latitude, fact.Longitude, empty(fact.Provenance, "UNCLASSIFIED"), fact.Suggestion)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) openReview(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		httpx.WriteError(w, r, httpx.New(http.StatusUnauthorized, "invalid_session", "Sign in again."))
		return
	}
	var review struct {
		IssueID    uuid.UUID `json:"issue_id"`
		Severity   string    `json:"severity"`
		Reason     string    `json:"reason"`
		Department string    `json:"department"`
		Locality   string    `json:"locality"`
		Category   string    `json:"category"`
		Excerpt    string    `json:"excerpt"`
		Deadline   time.Time `json:"deadline"`
	}
	if err := httpx.Decode(r, &review); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err := h.DB.WithFlags(r.Context(), nil, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		var category, locality *string
		if review.Category != "" {
			category = &review.Category
		}
		if review.Locality != "" && review.Locality != "Not provided" {
			locality = &review.Locality
		}
		extra, err := workflow.RoutingReasons(r.Context(), tx, category, locality)
		if err != nil {
			return err
		}
		reason := strings.TrimSpace(review.Reason + " " + strings.Join(extra, " "))
		area := review.Locality
		if area == "" {
			area = "Not provided"
		}
		if review.Deadline.IsZero() {
			review.Deadline = time.Now().UTC().Add(72 * time.Hour)
		}
		_, err = tx.Exec(r.Context(), `
			INSERT INTO review_items (
				id, issue_id, reason, severity, geographic_area, suggested_department, report_excerpt, deadline_at
			)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8
			WHERE NOT EXISTS (
				SELECT 1 FROM review_items WHERE issue_id = $2 AND status = 'open'
			)
		`, uuid.New(), review.IssueID, reason, empty(review.Severity, "P3"), area, review.Department, review.Excerpt, review.Deadline)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) resolve(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		httpx.WriteError(w, r, httpx.New(http.StatusUnauthorized, "invalid_session", "Sign in again."))
		return
	}
	var body struct {
		IssueID uuid.UUID `json:"issue_id"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err := h.DB.WithFlags(r.Context(), nil, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `UPDATE review_items SET status = 'resolved' WHERE issue_id = $1 AND status = 'open'`, body.IssueID)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func empty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
