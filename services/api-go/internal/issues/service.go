package issues

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/events"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/workflow"
)

type Service struct {
	DB *database.DB
}

type Issue struct {
	ID                        string          `json:"id"`
	Description               string          `json:"description"`
	CategoryLabel             *string         `json:"category_label"`
	StatedUrgency             string          `json:"stated_urgency"`
	AddressText               *string         `json:"address_text"`
	LocalityText              *string         `json:"locality_text"`
	Latitude                  *float64        `json:"latitude"`
	Longitude                 *float64        `json:"longitude"`
	IdentityVisibleToResolver bool            `json:"identity_visible_to_resolver"`
	FollowPublic              bool            `json:"follow_public"`
	Answers                   json.RawMessage `json:"answers"`
	Status                    string          `json:"status"`
	CreatedAt                 time.Time       `json:"created_at"`
	Visibility                string          `json:"visibility"`
	Severity                  *IssueSeverity  `json:"severity,omitempty"`
	SafetyGuidance            *string         `json:"safety_guidance,omitempty"`
	EscalationDue             *time.Time      `json:"escalation_due_at,omitempty"`
}

type IssueSeverity struct {
	Level      string   `json:"level"`
	Source     string   `json:"source"`
	Confidence *float64 `json:"confidence"`
	Reason     string   `json:"reason"`
	Overridden bool     `json:"overridden"`
}

// PublicReport is the only shape returned to someone who can see that a report
// was marked public and has no other grant. It has no address, coordinates,
// narrative, answers, or media.
type PublicReport struct {
	ID            string    `json:"id"`
	CategoryLabel *string   `json:"category_label"`
	LocalityText  *string   `json:"locality_text"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Visibility    string    `json:"visibility"`
	Summary       string    `json:"summary"`
}

type CreateInput struct {
	Description               string
	CategoryLabel             *string
	StatedUrgency             string
	AddressText               *string
	LocalityText              *string
	Latitude                  *float64
	Longitude                 *float64
	IdentityVisibleToResolver bool
	FollowPublic              bool
	ProblemTypeID             *uuid.UUID
	Answers                   map[string]string
	RequestID                 string
}

type SafetyNotice struct {
	ImmediateDanger bool     `json:"immediate_danger"`
	Dispatched      bool     `json:"dispatched"`
	Channels        []string `json:"channels"`
	Message         string   `json:"message"`
}

func (s *Service) Create(ctx context.Context, principal authn.Principal, in CreateInput) (Issue, *SafetyNotice, error) {
	description := strings.TrimSpace(in.Description)
	fields := map[string]string{}
	if len([]rune(description)) < 8 {
		fields["description"] = "Describe the problem in at least a few words."
	}
	if len([]rune(description)) > 4000 {
		fields["description"] = "Use 4000 characters or fewer."
	}
	urgency := in.StatedUrgency
	if urgency == "" {
		urgency = "normal"
	}
	if urgency != "low" && urgency != "normal" && urgency != "high" && urgency != "immediate_risk" {
		fields["stated_urgency"] = "Choose low, normal, high, or immediate risk."
	}
	category, categoryErr := optional(in.CategoryLabel, 80)
	if categoryErr != "" {
		fields["category_label"] = categoryErr
	}
	address, addressErr := optional(in.AddressText, 300)
	if addressErr != "" {
		fields["address_text"] = addressErr
	}
	locality, localityErr := optional(in.LocalityText, 120)
	if localityErr != "" {
		fields["locality_text"] = localityErr
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		fields["latitude"] = "Provide both coordinates, or leave both empty. Coordinates are never guessed."
	}
	if in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90) {
		fields["latitude"] = "Latitude must be between -90 and 90."
	}
	if in.Longitude != nil && (*in.Longitude < -180 || *in.Longitude > 180) {
		fields["longitude"] = "Longitude must be between -180 and 180."
	}
	if len(fields) > 0 {
		return Issue{}, nil, httpx.Validation(fields)
	}

	id := uuid.New()
	var issue Issue
	if in.Answers == nil {
		in.Answers = map[string]string{}
	}
	answers, err := json.Marshal(in.Answers)
	if err != nil {
		return Issue{}, nil, err
	}
	assessment := workflow.Assess(description)
	if category == nil && assessment.Suggestion != "" {
		suggestion := assessment.Suggestion
		category = &suggestion
	}
	status := "NEEDS_INFORMATION"
	if category != nil {
		status = "UNDER_REVIEW"
	}
	var classification workflow.Classification
	err = s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		if in.ProblemTypeID != nil {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM problem_types WHERE id = $1 AND active)`, *in.ProblemTypeID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.New(400, "validation_error", "Choose a problem type from the list.")
			}
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO issues (
				id, reporter_user_id, description, category_label, stated_urgency,
				address_text, locality_text, latitude, longitude,
				identity_visible_to_resolver, follow_public, problem_type_id, answers, status
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14)
			RETURNING id::text, description, category_label, stated_urgency, address_text, locality_text,
				latitude, longitude, identity_visible_to_resolver, follow_public, answers, status, created_at
		`, id, principal.UserID, description, category, urgency, address, locality, in.Latitude, in.Longitude, in.IdentityVisibleToResolver, in.FollowPublic, in.ProblemTypeID, answers, status)
		if err := scan(row, &issue); err != nil {
			return err
		}
		issue.Visibility = "owner"
		intake := "Report saved."
		if assessment.Suggestion != "" {
			intake += " A keyword suggested the label " + assessment.Suggestion + ". That is not an official classification."
		}
		if err := workflow.Record(ctx, tx, principal.UserID, id, principal.UserID, "intake", intake, "Report saved"); err != nil {
			return err
		}
		if assessment.Safety != "" {
			if err := workflow.Record(ctx, tx, principal.UserID, id, principal.UserID, "safety", assessment.Safety, "Safety guidance"); err != nil {
				return err
			}
		}
		extra := []string{}
		if locality != nil {
			var others int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM issues WHERE id <> $1 AND lower(coalesce(locality_text, '')) = lower($2)`, id, *locality).Scan(&others); err != nil {
				return err
			}
			if others > 0 {
				extra = append(extra, "Another report uses the same locality text. Compare them before treating them as one incident.")
			}
		}
		classified, err := workflow.ApplySeverity(ctx, tx, principal.UserID, id, description, urgency, locality, category, extra)
		if err != nil {
			return err
		}
		issue.Severity = &IssueSeverity{
			Level: classified.Level, Source: classified.Source, Reason: classified.Reason, Overridden: false,
		}
		if classified.Safety != "" {
			issue.SafetyGuidance = &classified.Safety
		} else if assessment.Safety != "" {
			issue.SafetyGuidance = &assessment.Safety
		}
		due := classified.EscalationDue
		issue.EscalationDue = &due
		classification = classified
		if assessment.Safety != "" && classification.Safety == "" {
			classification.Safety = assessment.Safety
		}
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "reporter", Action: "issue.create",
			ResourceType: "issue", ResourceID: &id, RequestID: in.RequestID,
			Metadata: map[string]any{"status": status, "stated_urgency": urgency, "coordinates_provided": in.Latitude != nil},
		}); err != nil {
			return err
		}
		return events.Enqueue(ctx, tx, "issue", id, "report.created", map[string]any{
			"issue_id": id.String(), "reporter_user_id": principal.UserID.String(),
			"status": status, "stated_urgency": urgency, "severity": classification.Level,
		})
	})
	if err != nil {
		return Issue{}, nil, err
	}
	if classification.Safety == "" && urgency != "immediate_risk" && classification.Level != "P0" {
		return issue, nil, nil
	}
	message := classification.Safety
	if message == "" {
		message = "LokaSolve has not dispatched anyone. If someone is in immediate danger, contact India's emergency number 112. For a mental-health crisis, Tele-MANAS is 14416. Do not wait for this report."
	}
	if !strings.Contains(message, "has not dispatched") {
		message += " LokaSolve has not dispatched anyone."
	}
	channels := []string{"112"}
	if strings.Contains(message, "14416") {
		channels = append(channels, "14416")
	}
	return issue, &SafetyNotice{
		ImmediateDanger: urgency == "immediate_risk" || classification.Level == "P0",
		Dispatched:      false,
		Channels:        channels,
		Message:         message,
	}, nil
}

func (s *Service) List(ctx context.Context, principal authn.Principal, query string) ([]Issue, error) {
	query = strings.NewReplacer("%", "", "_", "").Replace(strings.TrimSpace(query))
	items := []Issue{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, description, category_label, stated_urgency, address_text, locality_text,
				latitude, longitude, identity_visible_to_resolver, follow_public, answers, status, created_at
			FROM issues
			WHERE reporter_user_id = $1
				AND ($2 = '' OR description ILIKE '%' || $2 || '%' OR coalesce(category_label, '') ILIKE '%' || $2 || '%' OR coalesce(locality_text, '') ILIKE '%' || $2 || '%')
			ORDER BY created_at DESC
			LIMIT 100
		`, principal.UserID, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Issue
			if err := scan(rows, &item); err != nil {
				return err
			}
			item.Visibility = "owner"
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) Get(ctx context.Context, principal authn.Principal, id uuid.UUID) (any, error) {
	var view any
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var issue Issue
		var owner, assignee, admin bool
		row := tx.QueryRow(ctx, `
			SELECT reporter_user_id = $2,
				EXISTS (
					SELECT 1 FROM assignments a
					JOIN resolvers r ON r.id = a.resolver_id
					WHERE a.issue_id = issues.id
						AND r.owner_user_id = $2
						AND a.status <> 'rejected'
				),
				COALESCE(app_flag('app.platform_admin'), false),
				id::text, description, category_label, stated_urgency, address_text, locality_text,
				latitude, longitude, identity_visible_to_resolver, follow_public, answers, status, created_at
			FROM issues
			WHERE id = $1
		`, id, principal.UserID)
		err := row.Scan(
			&owner, &assignee, &admin,
			&issue.ID, &issue.Description, &issue.CategoryLabel, &issue.StatedUrgency,
			&issue.AddressText, &issue.LocalityText, &issue.Latitude, &issue.Longitude,
			&issue.IdentityVisibleToResolver, &issue.FollowPublic, &issue.Answers, &issue.Status, &issue.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			var pub PublicReport
			err = tx.QueryRow(ctx, `
				SELECT id::text, category_label, locality_text, status, created_at
				FROM public_issue_card($1)
			`, id).Scan(&pub.ID, &pub.CategoryLabel, &pub.LocalityText, &pub.Status, &pub.CreatedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.New(404, "not_found", "That report is not available.")
			}
			if err != nil {
				return err
			}
			pub.Visibility = "public"
			pub.Summary = "Exact address, coordinates, and photos are not included."
			view = pub
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case owner:
			issue.Visibility = "owner"
		case admin:
			issue.Visibility = "admin"
		case assignee:
			issue.Visibility = "assignment"
		default:
			issue.Visibility = "jurisdiction"
		}
		var level, source, reason string
		var confidence *float64
		var overridden bool
		var guidance *string
		var due *time.Time
		if err := tx.QueryRow(ctx, `
			SELECT severity, severity_source, severity_confidence, severity_reason,
				severity_overridden, safety_guidance, escalation_due_at
			FROM issues WHERE id = $1
		`, id).Scan(&level, &source, &confidence, &reason, &overridden, &guidance, &due); err != nil {
			return err
		}
		issue.Severity = &IssueSeverity{Level: level, Source: source, Confidence: confidence, Reason: reason, Overridden: overridden}
		issue.SafetyGuidance = guidance
		issue.EscalationDue = due
		view = issue
		return nil
	})
	return view, err
}

func optional(value *string, max int) (*string, string) {
	if value == nil {
		return nil, ""
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, ""
	}
	if len([]rune(trimmed)) > max {
		return nil, "That is too long."
	}
	return &trimmed, ""
}

type scanner interface {
	Scan(dest ...any) error
}

func scan(row scanner, issue *Issue) error {
	return row.Scan(
		&issue.ID, &issue.Description, &issue.CategoryLabel, &issue.StatedUrgency,
		&issue.AddressText, &issue.LocalityText, &issue.Latitude, &issue.Longitude,
		&issue.IdentityVisibleToResolver, &issue.FollowPublic, &issue.Answers, &issue.Status, &issue.CreatedAt,
	)
}
