package workflow

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Rule struct {
	ID                  string
	Severity            string
	Pattern             string
	Reason              string
	Safety              string
	SuggestedDepartment string
	ReviewRequired      bool
	EscalationHours     int
}

type Classification struct {
	Level         string
	Source        string
	Reason        string
	Safety        string
	Department    string
	Evidence      []string
	EscalationDue time.Time
	ReviewID      string
	ReviewReason  string
}

// MatchRule returns the first active rule whose pattern matches. Patterns are
// configuration, not an official inspection and not a model score.
func MatchRule(rules []Rule, text string) (Rule, bool) {
	for _, rule := range rules {
		pattern, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		if pattern.MatchString(text) {
			return rule, true
		}
	}
	return Rule{}, false
}

func ApplySeverity(ctx context.Context, tx pgx.Tx, actor, issueID uuid.UUID, description, urgency string, locality, category *string, extra []string) (Classification, error) {
	rules, err := loadRules(ctx, tx)
	if err != nil {
		return Classification{}, err
	}
	rule, matched := MatchRule(rules, description)
	result := Classification{
		Level:      "P3",
		Source:     "unclassified",
		Reason:     "No configured severity rule matched. A person should confirm this.",
		Department: "Not identified",
		Evidence:   []string{"rules:none"},
	}
	if matched {
		result.Level = rule.Severity
		result.Source = "rule"
		result.Reason = rule.Reason
		result.Safety = rule.Safety
		result.Department = rule.SuggestedDepartment
		result.Evidence = []string{"rule:" + rule.ID}
		if rule.EscalationHours > 0 {
			result.EscalationDue = time.Now().UTC().Add(time.Duration(rule.EscalationHours) * time.Hour)
		}
	}
	if result.EscalationDue.IsZero() {
		result.EscalationDue = time.Now().UTC().Add(72 * time.Hour)
	}
	reasons := []string{}
	if !matched || rule.ReviewRequired || result.Level == "P0" || result.Level == "P1" {
		reasons = append(reasons, "A person needs to assess this classification.")
	}
	if urgency == "immediate_risk" {
		reasons = append(reasons, "The resident marked this as an immediate risk.")
	}
	if locality == nil || strings.TrimSpace(*locality) == "" {
		reasons = append(reasons, "Jurisdiction is ambiguous because no locality was provided.")
	}
	routing, err := RoutingReasons(ctx, tx, category, locality)
	if err != nil {
		return Classification{}, err
	}
	reasons = append(reasons, routing...)
	reasons = append(reasons, "No authorized submission channel is connected. Nothing was sent.")
	for _, item := range extra {
		if strings.TrimSpace(item) != "" {
			reasons = append(reasons, strings.TrimSpace(item))
		}
	}
	evidence, err := json.Marshal(result.Evidence)
	if err != nil {
		return Classification{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE issues
		SET severity = $2, severity_source = $3, severity_confidence = NULL,
			severity_reason = $4, severity_overridden = false, severity_updated_at = now(),
			escalation_due_at = $5, safety_guidance = NULLIF($6, '')
		WHERE id = $1
	`, issueID, result.Level, result.Source, result.Reason, result.EscalationDue, result.Safety); err != nil {
		return Classification{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO severity_events (id, issue_id, actor_user_id, severity, source, reason, evidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, uuid.New(), issueID, actor, result.Level, result.Source, result.Reason, evidence); err != nil {
		return Classification{}, err
	}
	result.ReviewReason = strings.Join(reasons, " ")
	area := "Not provided"
	if locality != nil && strings.TrimSpace(*locality) != "" {
		area = strings.TrimSpace(*locality)
	}
	excerpt := description
	if runes := []rune(excerpt); len(runes) > 280 {
		excerpt = string(runes[:280])
	}
	reviewID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO review_items (
			id, issue_id, reason, severity, geographic_area, suggested_department,
			report_excerpt, deadline_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, reviewID, issueID, result.ReviewReason, result.Level, area, result.Department, excerpt, result.EscalationDue); err != nil {
		return Classification{}, err
	}
	result.ReviewID = reviewID.String()
	return result, nil
}

func loadRules(ctx context.Context, tx pgx.Tx) ([]Rule, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, severity, pattern, reason, safety_text, suggested_department,
			review_required, escalation_hours
		FROM severity_rules
		WHERE active
		ORDER BY priority, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []Rule{}
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.ID, &rule.Severity, &rule.Pattern, &rule.Reason, &rule.Safety, &rule.SuggestedDepartment, &rule.ReviewRequired, &rule.EscalationHours); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func RoutingReasons(ctx context.Context, tx pgx.Tx, category, locality *string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT official_name, locality_text, categories, verification_status
		FROM authorities
		ORDER BY official_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matches := []string{}
	for rows.Next() {
		var name, categories, verification string
		var officeLocality *string
		if err := rows.Scan(&name, &officeLocality, &categories, &verification); err != nil {
			return nil, err
		}
		if category == nil || strings.TrimSpace(*category) == "" || !CategoryListed(categories, *category) {
			continue
		}
		place := "Jurisdiction has not been matched."
		if locality != nil && officeLocality != nil && SamePlace(*locality, *officeLocality) {
			place = "The place names overlap. That is not proof of jurisdiction."
		}
		note := name + " lists this category. " + place
		if verification != "verified" {
			note += " The office is not verified."
		}
		matches = append(matches, note)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if category == nil || strings.TrimSpace(*category) == "" {
		return []string{"No verified category is available. A suggestion is not an authority routing instruction. The report stays in review."}, nil
	}
	if len(matches) == 0 {
		return []string{"No office on file lists this category. The report stays in review. Nothing was sent."}, nil
	}
	return []string{"Routing recommendation only: " + strings.Join(matches, " ") + " Nothing was sent."}, nil
}
