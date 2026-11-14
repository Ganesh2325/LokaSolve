package authority

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Step struct {
	Key    string     `json:"key"`
	Title  string     `json:"title"`
	State  string     `json:"state"`
	Detail string     `json:"detail"`
	At     *time.Time `json:"at,omitempty"`
}

type Progress struct {
	Where                  string     `json:"where"`
	Responsible            string     `json:"responsible"`
	PacketPrepared         bool       `json:"packet_prepared"`
	Submitted              bool       `json:"submitted"`
	Delivered              bool       `json:"delivered"`
	OfficiallyAcknowledged bool       `json:"officially_acknowledged"`
	OfficialReference      *string    `json:"official_reference"`
	OperatorReference      *string    `json:"operator_reference"`
	ReferenceExplanation   string     `json:"reference_explanation"`
	NextActor              string     `json:"next_actor"`
	EscalationDue          *time.Time `json:"escalation_due_at"`
	IfNobodyResponds       string     `json:"if_nobody_responds"`
	Steps                  []Step     `json:"steps"`
}

func (s *Service) Progress(ctx context.Context, principal authn.Principal, issueID uuid.UUID) (Progress, error) {
	var progress Progress
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var created time.Time
		var severity, source, reason string
		var guidance *string
		var due *time.Time
		var locality, category *string
		err := tx.QueryRow(ctx, `
			SELECT created_at, severity, severity_source, severity_reason, safety_guidance,
				escalation_due_at, locality_text, category_label
			FROM issues WHERE id = $1
		`, issueID).Scan(&created, &severity, &source, &reason, &guidance, &due, &locality, &category)
		if err == pgx.ErrNoRows && s.Issue != nil {
			fact, factErr := s.Issue.Facts(ctx, issueID)
			if factErr != nil {
				return factErr
			}
			var allowed bool
			area := ""
			if fact.Locality != nil {
				area = *fact.Locality
			}
			if err := tx.QueryRow(ctx, `
				SELECT $2 = app_user_id()
					OR COALESCE(app_flag('app.platform_admin'), false)
					OR app_is_issue_assignee($1)
					OR EXISTS (
						SELECT 1 FROM issue_access_grant(app_user_id(), $1, $3) g WHERE g.jurisdiction
					)
			`, issueID, fact.ReporterID, area).Scan(&allowed); err != nil {
				return err
			}
			if !allowed {
				return httpx.New(404, "not_found", "That report is not available.")
			}
			created, severity, source, reason = fact.CreatedAt, fact.Severity, fact.SeveritySource, fact.SeverityReason
			guidance, due, locality, category = fact.Safety, fact.EscalationDue, fact.Locality, fact.Category
			err = nil
		}
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		var reviewStatus, reviewReason string
		var reviewDeadline *time.Time
		reviewErr := tx.QueryRow(ctx, `
			SELECT status, reason, deadline_at FROM review_items
			WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1
		`, issueID).Scan(&reviewStatus, &reviewReason, &reviewDeadline)
		if reviewErr != nil && reviewErr != pgx.ErrNoRows {
			return reviewErr
		}
		referral, err := latestReferral(ctx, tx, issueID)
		if err != nil {
			return err
		}
		var providerStatus, providerName string
		providerErr := tx.QueryRow(ctx, `
			SELECT a.status, r.display_name
			FROM assignments a JOIN resolvers r ON r.id = a.resolver_id
			WHERE a.issue_id = $1 ORDER BY a.created_at DESC LIMIT 1
		`, issueID).Scan(&providerStatus, &providerName)
		if providerErr != nil && providerErr != pgx.ErrNoRows {
			return providerErr
		}
		progress = buildProgress(created, severity, source, reason, guidance, due, reviewErr == nil, reviewStatus, reviewReason, reviewDeadline, referral, providerErr == nil, providerStatus, providerName)
		return nil
	})
	return progress, err
}

type referralFact struct {
	Office         string
	Verification   string
	Status         string
	PacketAt       *time.Time
	AuthorizedAt   *time.Time
	SubmittedAt    *time.Time
	DeliveredAt    *time.Time
	OfficialAckAt  *time.Time
	AckKind        *string
	Reference      *string
	ReferenceKind  *string
	Department     *string
	AssignmentKind *string
	NextActor      *string
	EscalationDue  *time.Time
	HandoffAt      *time.Time
	CreatedAt      time.Time
}

func latestReferral(ctx context.Context, tx pgx.Tx, issueID uuid.UUID) (*referralFact, error) {
	var item referralFact
	err := tx.QueryRow(ctx, `
		SELECT a.official_name, a.verification_status, r.status, r.packet_prepared_at, r.authorized_at,
			r.submission_attempted_at, r.delivered_at, r.official_acknowledged_at, r.acknowledgement_kind,
			r.external_reference, r.reference_kind, r.assigned_department, r.assignment_kind, r.next_actor,
			r.escalation_due_at, r.handoff_at, r.created_at
		FROM referrals r JOIN authorities a ON a.id = r.authority_id
		WHERE r.issue_id = $1
		ORDER BY r.created_at DESC
		LIMIT 1
	`, issueID).Scan(
		&item.Office, &item.Verification, &item.Status, &item.PacketAt, &item.AuthorizedAt,
		&item.SubmittedAt, &item.DeliveredAt, &item.OfficialAckAt, &item.AckKind,
		&item.Reference, &item.ReferenceKind, &item.Department, &item.AssignmentKind, &item.NextActor,
		&item.EscalationDue, &item.HandoffAt, &item.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func buildProgress(created time.Time, severity, source, reason string, guidance *string, due *time.Time, hasReview bool, reviewStatus, reviewReason string, reviewDeadline *time.Time, referral *referralFact, hasProvider bool, providerStatus, providerName string) Progress {
	progress := Progress{
		Where:                "The report is saved. No office or provider has been selected, and nothing has been submitted.",
		Responsible:          "No department or provider is responsible yet.",
		NextActor:            "A reviewer",
		ReferenceExplanation: "No official reference number is on file.",
		IfNobodyResponds:     "If the review deadline passes, the report stays open. LokaSolve does not close it, does not invent a complaint number, and has not contacted an office.",
		Steps: []Step{
			{Key: "received", Title: "Report received", State: "recorded", Detail: "The report is saved.", At: &created},
			{Key: "assessed", Title: "Problem assessed", State: "recorded", Detail: "A keyword rule assessed the description. This is not an official inspection.", At: &created},
			{Key: "severity", Title: "Severity assigned", State: "recorded", Detail: severity + " from " + source + ". " + reason, At: &created},
		},
	}
	if guidance != nil && *guidance != "" {
		progress.Steps[1].Detail = *guidance
	}
	deadline := due
	if hasReview {
		state := "waiting"
		if reviewStatus == "resolved" || reviewStatus == "rejected" || reviewStatus == "no_destination" {
			state = "recorded"
		}
		detail := reviewReason
		if reviewStatus == "no_destination" {
			detail = "A reviewer recorded that no verified destination exists. The report was not discarded. " + reviewReason
		}
		progress.Steps = append(progress.Steps, Step{Key: "human_review", Title: "Human review required", State: state, Detail: detail, At: reviewDeadline})
		if reviewDeadline != nil {
			deadline = reviewDeadline
		}
		progress.NextActor = "A reviewer"
	}
	if hasProvider {
		progress.Where = "A provider assignment is on file for " + providerName + ". Provider acceptance is not a completed repair, and this is not a government submission."
		progress.Responsible = providerName + " is the provider named on the assignment."
		progress.Steps = append(progress.Steps, Step{
			Key: "provider", Title: "Provider assignment", State: "recorded",
			Detail: "Assignment status is " + providerStatus + ". This path is separate from an official complaint.",
		})
	}
	if referral == nil {
		progress.Steps = append(progress.Steps,
			Step{Key: "authority", Title: "Authority identification", State: "waiting", Detail: "No responsible office has been identified."},
			Step{Key: "packet", Title: "Complaint packet prepared", State: "unknown", Detail: "No complaint packet has been prepared."},
			Step{Key: "submitted", Title: "Submission attempted", State: "unknown", Detail: "No submission has been attempted."},
			Step{Key: "delivered", Title: "Delivery", State: "unknown", Detail: "No delivery receipt is on file."},
			Step{Key: "acknowledged", Title: "Official acknowledgement", State: "unknown", Detail: "No official acknowledgement is on file."},
			Step{Key: "reference", Title: "Official reference number", State: "unknown", Detail: "No official reference number is on file."},
		)
		progress.EscalationDue = deadline
		return progress
	}
	progress.Responsible = referral.Office + " is the directory match. Verification is " + referral.Verification + ". That is not confirmation from the office."
	progress.Where = "A directory match names " + referral.Office + ". A prepared packet is not a submitted complaint."
	progress.PacketPrepared = referral.PacketAt != nil
	progress.Submitted = referral.SubmittedAt != nil
	progress.Delivered = referral.DeliveredAt != nil
	progress.OfficiallyAcknowledged = referral.OfficialAckAt != nil
	if referral.Reference != nil && referral.ReferenceKind != nil && *referral.ReferenceKind == "operator_entered" {
		progress.OperatorReference = referral.Reference
		progress.ReferenceExplanation = "An operator typed this number. It is not a portal receipt."
	} else {
		progress.ReferenceExplanation = "No official reference number is on file."
	}
	if referral.NextActor != nil && *referral.NextActor != "" && *referral.NextActor != "none" {
		progress.NextActor = actorLabel(*referral.NextActor)
	}
	if referral.EscalationDue != nil {
		deadline = referral.EscalationDue
	}
	progress.EscalationDue = deadline
	progress.Steps = append(progress.Steps,
		Step{Key: "authority", Title: "Responsible authority identified", State: "recorded", Detail: progress.Responsible, At: &referral.CreatedAt},
		packetStep(referral),
		authorizationStep(referral),
		evidenceStep("submitted", "Submission attempted", referral.SubmittedAt, "A submission attempt is recorded.", "No submission has been attempted."),
		evidenceStep("delivered", "Delivered", referral.DeliveredAt, "A delivery receipt is recorded.", "No delivery receipt is on file."),
		acknowledgementStep(referral),
		referenceStep(progress.OperatorReference),
		departmentStep(referral),
	)
	if referral.HandoffAt != nil {
		progress.Steps = append(progress.Steps, Step{
			Key: "handoff", Title: "Manual handoff recorded", State: "recorded",
			Detail: "A person recorded a handoff. This is not a delivery receipt.", At: referral.HandoffAt,
		})
	}
	switch referral.Status {
	case "action_started", "resolution_reported", "confirmed", "disputed", "closed":
		progress.Steps = append(progress.Steps, Step{Key: "field", Title: "Field action", State: "recorded", Detail: "An operator recorded a field-action note. LokaSolve has no report from the office."})
	}
	switch referral.Status {
	case "resolution_reported":
		progress.Steps = append(progress.Steps, Step{Key: "verification", Title: "User verification pending", State: "waiting", Detail: "Resolution evidence was recorded. You have not verified it."})
		progress.NextActor = "You"
	case "confirmed", "closed":
		progress.Steps = append(progress.Steps, Step{Key: "verification", Title: "Resolved", State: "recorded", Detail: "You confirmed the resolution."})
	case "disputed":
		progress.Steps = append(progress.Steps, Step{Key: "verification", Title: "Disputed", State: "recorded", Detail: "You disputed the resolution. It stays open for review."})
	}
	return progress
}

func packetStep(item *referralFact) Step {
	if item.PacketAt == nil {
		return Step{Key: "packet", Title: "Complaint packet prepared", State: "unknown", Detail: "No complaint packet has been prepared."}
	}
	return Step{Key: "packet", Title: "Complaint packet prepared", State: "recorded", Detail: "The packet is prepared. It has not been submitted.", At: item.PacketAt}
}

func authorizationStep(item *referralFact) Step {
	if item.PacketAt == nil {
		return Step{Key: "authorization", Title: "Submission awaiting authorization", State: "unknown", Detail: "There is no packet to authorize."}
	}
	if item.AuthorizedAt == nil {
		return Step{Key: "authorization", Title: "Submission awaiting authorization", State: "waiting", Detail: "An operator has not approved the packet. Nothing was submitted."}
	}
	return Step{Key: "authorization", Title: "Submission authorized for a later attempt", State: "recorded", Detail: "An operator approved the packet. Approval is not a submission.", At: item.AuthorizedAt}
}

func evidenceStep(key, title string, at *time.Time, recorded, missing string) Step {
	if at == nil {
		return Step{Key: key, Title: title, State: "unknown", Detail: missing}
	}
	return Step{Key: key, Title: title, State: "recorded", Detail: recorded, At: at}
}

func acknowledgementStep(item *referralFact) Step {
	if item.OfficialAckAt != nil {
		return Step{Key: "acknowledged", Title: "Official acknowledgement", State: "recorded", Detail: "An official acknowledgement is recorded.", At: item.OfficialAckAt}
	}
	if item.AckKind != nil && *item.AckKind == "operator_note" {
		return Step{Key: "acknowledged", Title: "Official acknowledgement", State: "unknown", Detail: "An operator wrote a note. That note is not an official acknowledgement."}
	}
	return Step{Key: "acknowledged", Title: "Official acknowledgement", State: "unknown", Detail: "No official acknowledgement is on file."}
}

func referenceStep(operatorReference *string) Step {
	if operatorReference == nil {
		return Step{Key: "reference", Title: "Official reference number", State: "unknown", Detail: "No official reference number is on file."}
	}
	return Step{Key: "reference", Title: "Official reference number", State: "unknown", Detail: "An operator entered " + *operatorReference + ". This is not a portal receipt."}
}

func departmentStep(item *referralFact) Step {
	if item.Department == nil || *item.Department == "" {
		return Step{Key: "department", Title: "Department assignment", State: "unknown", Detail: "No department assignment is confirmed."}
	}
	return Step{Key: "department", Title: "Department assignment", State: "unknown", Detail: "An operator recorded " + *item.Department + ". The office has not confirmed it."}
}

func actorLabel(actor string) string {
	switch actor {
	case "resident":
		return "You"
	case "operator":
		return "An operator"
	case "reviewer":
		return "A reviewer"
	default:
		return "Not recorded"
	}
}
