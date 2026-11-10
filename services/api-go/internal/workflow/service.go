package workflow

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Service struct {
	DB      *database.DB
	Scanner MalwareScanner
}

type TimelineItem struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type MediaItem struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	ByteSize    int    `json:"byte_size"`
	MalwareScan string `json:"malware_scan"`
}

type NearbyItem struct {
	ID            string    `json:"id"`
	CategoryLabel *string   `json:"category_label"`
	LocalityText  *string   `json:"locality_text"`
	Summary       string    `json:"summary"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type Resolver struct {
	ID                 string  `json:"id"`
	DisplayName        string  `json:"display_name"`
	Kind               string  `json:"kind"`
	LocalityText       *string `json:"locality_text"`
	Skills             string  `json:"skills"`
	VerificationStatus string  `json:"verification_status"`
	Explanation        string  `json:"explanation,omitempty"`
}

type Assignment struct {
	ID              string  `json:"id"`
	IssueID         string  `json:"issue_id"`
	ResolverName    string  `json:"resolver_name"`
	Status          string  `json:"status"`
	Explanation     string  `json:"explanation"`
	QuoteText       *string `json:"quote_text"`
	QuoteApproved   bool    `json:"quote_approved"`
	AppointmentText *string `json:"appointment_text"`
	EvidenceText    *string `json:"evidence_text"`
	CanResolve      bool    `json:"can_resolve"`
	CanConfirm      bool    `json:"can_confirm"`
}

type Notice struct {
	ID        string     `json:"id"`
	IssueID   *string    `json:"issue_id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type ReviewItem struct {
	ID      string `json:"id"`
	IssueID string `json:"issue_id"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
}

type Analytics struct {
	Reports               int `json:"reports"`
	OpenReviews           int `json:"open_reviews"`
	AwaitingAcceptance    int `json:"awaiting_acceptance"`
	AwaitingConfirmation  int `json:"awaiting_confirmation"`
	Resolved              int `json:"resolved"`
	OlderThan48Hours      int `json:"older_than_48_hours"`
	PublicNearby          int `json:"public_nearby"`
	ExternalMessagesReady int `json:"external_messages_ready"`
}

func (s *Service) Timeline(ctx context.Context, principal authn.Principal, issueID uuid.UUID) ([]TimelineItem, error) {
	items := []TimelineItem{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, kind, body, created_at FROM issue_events
			WHERE issue_id = $1 ORDER BY created_at
		`, issueID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item TimelineItem
			if err := rows.Scan(&item.ID, &item.Kind, &item.Body, &item.CreatedAt); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) SaveMedia(ctx context.Context, principal authn.Principal, issueID uuid.UUID, fileName, declared string, body []byte) (MediaItem, error) {
	contentType, storedName, err := InspectUpload(fileName, declared, body)
	if err != nil {
		return MediaItem{}, err
	}
	scanner := s.Scanner
	if scanner == nil {
		scanner = UnconfiguredScanner{}
	}
	scan, err := scanner.Scan(ctx, contentType, body)
	if err != nil {
		return MediaItem{}, httpx.New(400, "validation_error", "The file could not be checked.")
	}
	if scan.Status == ScanRejected {
		return MediaItem{}, httpx.New(400, "validation_error", "That file was rejected.")
	}
	if scan.Status != ScanClean && scan.Status != ScanNotScanned {
		scan.Status = ScanNotScanned
	}
	item := MediaItem{
		ID: uuid.NewString(), FileName: storedName, ContentType: contentType,
		ByteSize: len(body), MalwareScan: scan.Status,
	}
	err = s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var reporter uuid.UUID
		err := tx.QueryRow(ctx, `SELECT reporter_user_id FROM issues WHERE id = $1 AND reporter_user_id = $2`, issueID, principal.UserID).Scan(&reporter)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		id := uuid.MustParse(item.ID)
		if _, err := tx.Exec(ctx, `
			INSERT INTO media_objects (id, issue_id, owner_user_id, file_name, content_type, byte_size, content, malware_scan)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, id, issueID, principal.UserID, storedName, contentType, len(body), body, scan.Status); err != nil {
			return err
		}
		return Record(ctx, tx, principal.UserID, issueID, principal.UserID, "media", "A file was added to this report. It has not been malware-scanned.", "File added")
	})
	return item, err
}

func (s *Service) ListMedia(ctx context.Context, principal authn.Principal, issueID uuid.UUID) ([]MediaItem, error) {
	items := []MediaItem{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, file_name, content_type, byte_size, malware_scan
			FROM media_objects
			WHERE issue_id = $1 AND `+mediaVisibleSQL+`
			ORDER BY created_at
		`, issueID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item MediaItem
			if err := rows.Scan(&item.ID, &item.FileName, &item.ContentType, &item.ByteSize, &item.MalwareScan); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) OpenMedia(ctx context.Context, principal authn.Principal, issueID, mediaID uuid.UUID) (string, string, string, []byte, error) {
	var contentType, fileName, scan string
	var content []byte
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT content_type, file_name, malware_scan, content
			FROM media_objects
			WHERE id = $1 AND issue_id = $2 AND `+mediaVisibleSQL+`
		`, mediaID, issueID).Scan(&contentType, &fileName, &scan, &content)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.New(404, "not_found", "That file is not available.")
		}
		return err
	})
	if scan != ScanClean && scan != ScanRejected {
		scan = ScanNotScanned
	}
	return contentType, fileName, scan, content, err
}

const mediaVisibleSQL = `
(
  owner_user_id = app_user_id()
  OR app_flag('app.platform_admin')
  OR app_is_issue_reporter(issue_id)
  OR app_is_issue_assignee(issue_id)
  OR app_issue_in_jurisdiction(issue_id)
)`

func (s *Service) Nearby(ctx context.Context, principal authn.Principal) ([]NearbyItem, error) {
	items := []NearbyItem{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, category_label, locality_text, status, created_at
			FROM public_issue_cards()
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item NearbyItem
			if err := rows.Scan(&item.ID, &item.CategoryLabel, &item.LocalityText, &item.Status, &item.CreatedAt); err != nil {
				return err
			}
			item.Summary = publicSummary
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) Classify(ctx context.Context, principal authn.Principal, issueID uuid.UUID, label, requestID string) error {
	label = strings.TrimSpace(label)
	if len([]rune(label)) < 2 || len([]rune(label)) > 80 {
		return httpx.Validation(map[string]string{"category_label": "Use 2 to 80 characters."})
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var reporter uuid.UUID
		err := tx.QueryRow(ctx, `SELECT reporter_user_id FROM issues WHERE id = $1`, issueID).Scan(&reporter)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		if reporter != principal.UserID && !principal.Has("admin:roles_grant") {
			return httpx.New(403, "forbidden", "You do not have access to that action.")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE issues SET category_label = $2, status = 'UNDER_REVIEW', updated_at = now()
			WHERE id = $1 AND (reporter_user_id = $3 OR app_flag('app.platform_admin'))
		`, issueID, label, principal.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE review_items SET status = 'resolved' WHERE issue_id = $1 AND status = 'open'`, issueID); err != nil {
			return err
		}
		if err := Record(ctx, tx, principal.UserID, issueID, reporter, "classification", "The label was set to "+label+".", "Label updated"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "reporter", Action: "issue.classify",
			ResourceType: "issue", ResourceID: &issueID, RequestID: requestID,
			Metadata: map[string]any{"category_label": label},
		})
	})
}

func (s *Service) CreateIncident(ctx context.Context, principal authn.Principal, issueID uuid.UUID) (string, error) {
	incidentID := uuid.New()
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var title string
		var locality *string
		var reporter uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(category_label, 'Local report'), locality_text, reporter_user_id FROM issues WHERE id = $1
		`, issueID).Scan(&title, &locality, &reporter)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO incidents (id, title, locality_text, created_by) VALUES ($1, $2, $3, $4)
		`, incidentID, title, locality, principal.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO incident_links (incident_id, issue_id, relation) VALUES ($1, $2, 'member')
		`, incidentID, issueID); err != nil {
			return err
		}
		return Record(ctx, tx, principal.UserID, issueID, reporter, "incident", "An incident record was opened for this report. Nobody has been dispatched.", "Incident opened")
	})
	return incidentID.String(), err
}

func (s *Service) AddResolver(ctx context.Context, principal authn.Principal, name, kind, locality, skills, ownerEmail, requestID string) (Resolver, error) {
	if !principal.Has("admin:roles_grant") && !principal.Has("admin:taxonomy_manage") {
		return Resolver{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	skills = strings.TrimSpace(skills)
	locality = strings.TrimSpace(locality)
	if kind != "provider" && kind != "organization" && kind != "field" {
		kind = "provider"
	}
	fields := map[string]string{}
	if len([]rune(name)) < 2 || len([]rune(name)) > 120 {
		fields["display_name"] = "Use 2 to 120 characters."
	}
	if len([]rune(skills)) < 2 {
		fields["skills"] = "Name the kind of work this responder can take."
	}
	if len(fields) > 0 {
		return Resolver{}, httpx.Validation(fields)
	}
	var localityValue *string
	if locality != "" {
		localityValue = &locality
	}
	created := Resolver{ID: uuid.NewString(), DisplayName: name, Kind: kind, LocalityText: localityValue, Skills: skills, VerificationStatus: "unverified"}
	err := s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		var owner *uuid.UUID
		ownerEmail = strings.TrimSpace(strings.ToLower(ownerEmail))
		if ownerEmail != "" {
			var id uuid.UUID
			err := tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = $1`, ownerEmail).Scan(&id)
			if err == pgx.ErrNoRows {
				return httpx.Validation(map[string]string{"owner_email": "No account uses that email."})
			}
			if err != nil {
				return err
			}
			owner = &id
		}
		id := uuid.MustParse(created.ID)
		if _, err := tx.Exec(ctx, `
			INSERT INTO resolvers (id, display_name, kind, locality_text, skills, owner_user_id)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, id, name, kind, localityValue, skills, owner); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "operator", Action: "resolver.create",
			ResourceType: "resolver", ResourceID: &id, RequestID: requestID,
			Metadata: map[string]any{"name": name, "kind": kind},
		})
	})
	return created, err
}

func (s *Service) Eligible(ctx context.Context, principal authn.Principal, issueID uuid.UUID) ([]Resolver, error) {
	found := []Resolver{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var category, locality, description *string
		err := tx.QueryRow(ctx, `SELECT category_label, locality_text, description FROM issues WHERE id = $1`, issueID).Scan(&category, &locality, &description)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, display_name, kind, locality_text, skills, verification_status
			FROM resolvers WHERE active ORDER BY display_name
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Resolver
			if err := rows.Scan(&item.ID, &item.DisplayName, &item.Kind, &item.LocalityText, &item.Skills, &item.VerificationStatus); err != nil {
				return err
			}
			if explanation, ok := eligible(item, category, locality); ok {
				item.Explanation = explanation
				found = append(found, item)
			}
		}
		return rows.Err()
	})
	return found, err
}

func eligible(item Resolver, category, locality *string) (string, bool) {
	reasons := []string{}
	if category != nil && *category != "" && strings.Contains(strings.ToLower(item.Skills), strings.ToLower(*category)) {
		reasons = append(reasons, "skills include "+*category)
	}
	if locality != nil && item.LocalityText != nil && *locality != "" && *item.LocalityText != "" &&
		(strings.Contains(strings.ToLower(*locality), strings.ToLower(*item.LocalityText)) || strings.Contains(strings.ToLower(*item.LocalityText), strings.ToLower(*locality))) {
		reasons = append(reasons, "covers "+*item.LocalityText)
	}
	if len(reasons) == 0 {
		return "", false
	}
	note := "Eligible because " + strings.Join(reasons, " and ") + ". Verification is " + item.VerificationStatus + ". No offer has been sent by this check."
	return note, true
}

func (s *Service) Offer(ctx context.Context, principal authn.Principal, issueID, resolverID uuid.UUID, requestID string) (Assignment, error) {
	if !principal.Has("admin:roles_grant") && !principal.Has("admin:taxonomy_manage") {
		return Assignment{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	var created Assignment
	err := s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		matches, err := loadEligible(ctx, tx, issueID)
		if err != nil {
			return err
		}
		var chosen *Resolver
		for i := range matches {
			if matches[i].ID == resolverID.String() {
				chosen = &matches[i]
				break
			}
		}
		if chosen == nil {
			return httpx.New(400, "validation_error", "That responder is not eligible for this report.")
		}
		var reporter uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT reporter_user_id FROM issues WHERE id = $1`, issueID).Scan(&reporter); err != nil {
			return err
		}
		id := uuid.New()
		created = Assignment{ID: id.String(), IssueID: issueID.String(), ResolverName: chosen.DisplayName, Status: "offered", Explanation: chosen.Explanation}
		if _, err := tx.Exec(ctx, `
			INSERT INTO assignments (id, issue_id, resolver_id, status, explanation)
			VALUES ($1, $2, $3, 'offered', $4)
		`, id, issueID, resolverID, chosen.Explanation); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE issues SET status = 'AWAITING_RESOLVER_ACCEPTANCE', updated_at = now() WHERE id = $1`, issueID); err != nil {
			return err
		}
		if err := Record(ctx, tx, principal.UserID, issueID, reporter, "offer", "An offer was recorded for "+chosen.DisplayName+". They have not accepted it.", "Offer recorded"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "operator", Action: "assignment.offer",
			ResourceType: "assignment", ResourceID: &id, RequestID: requestID,
		})
	})
	return created, err
}

func loadEligible(ctx context.Context, tx pgx.Tx, issueID uuid.UUID) ([]Resolver, error) {
	var category, locality *string
	var description string
	if err := tx.QueryRow(ctx, `SELECT category_label, locality_text, description FROM issues WHERE id = $1`, issueID).Scan(&category, &locality, &description); err != nil {
		if err == pgx.ErrNoRows {
			return nil, httpx.New(404, "not_found", "That report is not available.")
		}
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text, display_name, kind, locality_text, skills, verification_status FROM resolvers WHERE active`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := []Resolver{}
	for rows.Next() {
		var item Resolver
		if err := rows.Scan(&item.ID, &item.DisplayName, &item.Kind, &item.LocalityText, &item.Skills, &item.VerificationStatus); err != nil {
			return nil, err
		}
		if explanation, ok := eligible(item, category, locality); ok {
			item.Explanation = explanation
			found = append(found, item)
		}
	}
	return found, rows.Err()
}

func (s *Service) ListAssignments(ctx context.Context, principal authn.Principal) ([]Assignment, error) {
	items := []Assignment{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		operator := principal.Has("admin:roles_grant") || principal.Has("admin:taxonomy_manage")
		rows, err := tx.Query(ctx, `
			SELECT a.id::text, a.issue_id::text, r.display_name, a.status, a.explanation,
				a.quote_text, a.quote_approved, a.appointment_text, a.evidence_text,
				COALESCE(r.owner_user_id = $1, false), COALESCE(i.reporter_user_id = $1, false)
			FROM assignments a
			JOIN resolvers r ON r.id = a.resolver_id
			LEFT JOIN issues i ON i.id = a.issue_id
			ORDER BY a.created_at DESC LIMIT 100
		`, principal.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Assignment
			if err := rows.Scan(&item.ID, &item.IssueID, &item.ResolverName, &item.Status, &item.Explanation, &item.QuoteText, &item.QuoteApproved, &item.AppointmentText, &item.EvidenceText, &item.CanResolve, &item.CanConfirm); err != nil {
				return err
			}
			if operator {
				item.CanResolve = true
				item.CanConfirm = true
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) Transition(ctx context.Context, principal authn.Principal, assignmentID uuid.UUID, status, note, requestID string) error {
	status = strings.TrimSpace(status)
	note = strings.TrimSpace(note)
	allowed := map[string]map[string]string{
		"offered":              {"accepted": "resolver", "rejected": "resolver"},
		"accepted":             {"scheduled": "resolver", "in_progress": "resolver"},
		"scheduled":            {"in_progress": "resolver"},
		"in_progress":          {"resolution_submitted": "resolver"},
		"resolution_submitted": {"resolved": "reporter", "disputed": "reporter"},
		"disputed":             {"reopened": "operator"},
		"reopened":             {"in_progress": "resolver"},
	}
	issueStatus := map[string]string{
		"accepted": "ASSIGNED", "rejected": "ROUTING_PENDING", "scheduled": "SCHEDULED",
		"in_progress": "IN_PROGRESS", "resolution_submitted": "AWAITING_CONFIRMATION",
		"resolved": "RESOLVED", "disputed": "DISPUTED", "reopened": "REOPENED",
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var current string
		var reporter, owner uuid.UUID
		var hasOwner bool
		var issueID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT a.status, a.issue_id, i.reporter_user_id, r.owner_user_id IS NOT NULL, COALESCE(r.owner_user_id, '00000000-0000-0000-0000-000000000000')
			FROM assignments a
			JOIN issues i ON i.id = a.issue_id
			JOIN resolvers r ON r.id = a.resolver_id
			WHERE a.id = $1
		`, assignmentID).Scan(&current, &issueID, &reporter, &hasOwner, &owner)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That assignment is not available.")
		}
		if err != nil {
			return err
		}
		nextParty, ok := allowed[current][status]
		if !ok {
			return httpx.New(400, "validation_error", "That status change is not available.")
		}
		operator := principal.Has("admin:roles_grant") || principal.Has("admin:taxonomy_manage")
		switch nextParty {
		case "resolver":
			if !operator && (!hasOwner || owner != principal.UserID) {
				return httpx.New(403, "forbidden", "You do not have access to that action.")
			}
		case "reporter":
			if principal.UserID != reporter && !operator {
				return httpx.New(403, "forbidden", "You do not have access to that action.")
			}
		case "operator":
			if !operator {
				return httpx.New(403, "forbidden", "You do not have access to that action.")
			}
		}
		if status == "resolution_submitted" && len([]rune(note)) < 8 {
			return httpx.Validation(map[string]string{"note": "Describe the evidence in a few words."})
		}
		if status == "scheduled" && note == "" {
			return httpx.Validation(map[string]string{"note": "Write the appointment."})
		}
		if status == "scheduled" {
			_, err = tx.Exec(ctx, `UPDATE assignments SET status = $2, appointment_text = $3, updated_at = now() WHERE id = $1`, assignmentID, status, note)
		} else if status == "resolution_submitted" {
			_, err = tx.Exec(ctx, `UPDATE assignments SET status = $2, evidence_text = $3, updated_at = now() WHERE id = $1`, assignmentID, status, note)
		} else {
			_, err = tx.Exec(ctx, `UPDATE assignments SET status = $2, updated_at = now() WHERE id = $1`, assignmentID, status)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE issues SET status = $2, updated_at = now() WHERE id = $1`, issueID, issueStatus[status]); err != nil {
			return err
		}
		body := "Assignment status is now " + strings.ReplaceAll(status, "_", " ") + "."
		if note != "" {
			body += " " + note
		}
		if err := Record(ctx, tx, principal.UserID, issueID, reporter, "assignment", body, "Assignment updated"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: nextParty, Action: "assignment.transition",
			ResourceType: "assignment", ResourceID: &assignmentID, RequestID: requestID,
			Metadata: map[string]any{"status": status},
		})
	})
}

func (s *Service) ApproveQuote(ctx context.Context, principal authn.Principal, assignmentID uuid.UUID) error {
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var issueID, reporter uuid.UUID
		var quote *string
		err := tx.QueryRow(ctx, `
			SELECT a.issue_id, i.reporter_user_id, a.quote_text
			FROM assignments a JOIN issues i ON i.id = a.issue_id
			WHERE a.id = $1 AND i.reporter_user_id = $2
		`, assignmentID, principal.UserID).Scan(&issueID, &reporter, &quote)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That quote is not available.")
		}
		if err != nil {
			return err
		}
		if quote == nil || *quote == "" {
			return httpx.New(400, "validation_error", "There is no quote to approve.")
		}
		if _, err := tx.Exec(ctx, `UPDATE assignments SET quote_approved = true, updated_at = now() WHERE id = $1`, assignmentID); err != nil {
			return err
		}
		return Record(ctx, tx, principal.UserID, issueID, reporter, "quote", "The person who filed the report approved the quote.", "Quote approved")
	})
}

func (s *Service) SetQuote(ctx context.Context, principal authn.Principal, assignmentID uuid.UUID, quote string) error {
	if !principal.Has("admin:roles_grant") && !principal.Has("admin:taxonomy_manage") {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return httpx.Validation(map[string]string{"quote_text": "Write the quote."})
	}
	return s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		var issueID, reporter uuid.UUID
		err := tx.QueryRow(ctx, `
			UPDATE assignments SET quote_text = $2, quote_approved = false, updated_at = now()
			WHERE id = $1 RETURNING issue_id
		`, assignmentID, quote).Scan(&issueID)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That assignment is not available.")
		}
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT reporter_user_id FROM issues WHERE id = $1`, issueID).Scan(&reporter); err != nil {
			return err
		}
		return Record(ctx, tx, principal.UserID, issueID, reporter, "quote", "A quote was recorded: "+quote+". It is not approved yet.", "Quote recorded")
	})
}

func (s *Service) Notices(ctx context.Context, principal authn.Principal) ([]Notice, error) {
	items := []Notice{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, issue_id::text, title, body, read_at, created_at
			FROM notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50
		`, principal.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Notice
			if err := rows.Scan(&item.ID, &item.IssueID, &item.Title, &item.Body, &item.ReadAt, &item.CreatedAt); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) MarkRead(ctx context.Context, principal authn.Principal, id uuid.UUID) error {
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`, id, principal.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notifications WHERE id = $1 AND user_id = $2)`, id, principal.UserID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.New(404, "not_found", "That notification is not available.")
			}
		}
		return nil
	})
}

func (s *Service) Reviews(ctx context.Context, principal authn.Principal) ([]ReviewItem, error) {
	if !principal.Has("admin:users_read") && !principal.Has("admin:roles_grant") && !principal.Has("admin:audit_read") {
		return nil, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	items := []ReviewItem{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, issue_id::text, reason, status FROM review_items
			WHERE status = 'open' ORDER BY created_at DESC LIMIT 50
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ReviewItem
			if err := rows.Scan(&item.ID, &item.IssueID, &item.Reason, &item.Status); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) ResolveReview(ctx context.Context, principal authn.Principal, id uuid.UUID) error {
	if !principal.Has("admin:users_read") && !principal.Has("admin:roles_grant") {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE review_items SET status = 'resolved' WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(404, "not_found", "That review is not available.")
		}
		return nil
	})
}

func (s *Service) DigitalAction(ctx context.Context, principal authn.Principal, issueID uuid.UUID, template, reference string) error {
	if !principal.Has("admin:roles_grant") && !principal.Has("admin:taxonomy_manage") {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	template = strings.TrimSpace(template)
	reference = strings.TrimSpace(reference)
	if template == "" {
		return httpx.Validation(map[string]string{"template_name": "Name the action."})
	}
	status := "not_submitted"
	body := template + " was recorded. It has not been submitted to an external office."
	if reference != "" {
		status = "recorded"
		body = template + " has reference " + reference + " typed by an operator. That text is not proof of delivery."
	}
	return s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		var reporter uuid.UUID
		err := tx.QueryRow(ctx, `SELECT reporter_user_id FROM issues WHERE id = $1`, issueID).Scan(&reporter)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		var ref *string
		if reference != "" {
			ref = &reference
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO digital_actions (id, issue_id, template_name, external_reference, status)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New(), issueID, template, ref, status); err != nil {
			return err
		}
		return Record(ctx, tx, principal.UserID, issueID, reporter, "digital", body, "Action recorded")
	})
}

func (s *Service) Measure(ctx context.Context, principal authn.Principal) (Analytics, error) {
	if !principal.Has("admin:users_read") && !principal.Has("admin:roles_grant") && !principal.Has("admin:audit_read") {
		return Analytics{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	var stats Analytics
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT reports, open_reviews, awaiting_acceptance, awaiting_confirmation,
				resolved, older_than_48_hours, public_nearby, external_messages_ready
			FROM issue_metrics()
		`).Scan(&stats.Reports, &stats.OpenReviews, &stats.AwaitingAcceptance, &stats.AwaitingConfirmation, &stats.Resolved, &stats.OlderThan48Hours, &stats.PublicNearby, &stats.ExternalMessagesReady)
	})
	return stats, err
}

func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit+1))
}
