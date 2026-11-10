package users

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
	"lokasolve/services/api-go/internal/platform/validate"
	"lokasolve/services/api-go/internal/views"
)

type Service struct {
	DB *database.DB
}

type ProfileUpdate struct {
	DisplayName             *string
	PreferredLanguage       *string
	NotificationPreferences *map[string]bool
	PrivacyPreferences      *map[string]bool
	RequestID               string
}

type LocationInput struct {
	Label        *string
	AddressText  *string
	LocalityText *string
	Latitude     *float64
	Longitude    *float64
	Source       string
	RequestID    string
}

func (s *Service) Me(ctx context.Context, principal authn.Principal) (views.User, error) {
	var user views.User
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		loaded, err := identity.LoadUser(ctx, tx, principal.UserID)
		if err != nil {
			return err
		}
		user = loaded
		return nil
	})
	return user, err
}

func (s *Service) Update(ctx context.Context, principal authn.Principal, in ProfileUpdate) (views.User, error) {
	if in.DisplayName == nil && in.PreferredLanguage == nil && in.NotificationPreferences == nil && in.PrivacyPreferences == nil {
		return views.User{}, httpx.New(400, "validation_error", "Provide a field to update.")
	}
	fields := map[string]string{}
	var name string
	var language string
	if in.DisplayName != nil {
		var errText string
		name, errText = validate.DisplayName(*in.DisplayName)
		fields["display_name"] = errText
	}
	if in.PreferredLanguage != nil {
		var errText string
		language, errText = validate.Language(*in.PreferredLanguage)
		fields["preferred_language"] = errText
	}
	if in.NotificationPreferences != nil {
		fields["notification_preferences"] = preferenceError(*in.NotificationPreferences, []string{"in_app", "email", "sms", "whatsapp"})
	}
	if in.PrivacyPreferences != nil {
		fields["privacy_preferences"] = preferenceError(*in.PrivacyPreferences, []string{"public_identity"})
	}
	if invalid(fields) {
		return views.User{}, httpx.Validation(fields)
	}
	var user views.User
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		current, err := identity.LoadUser(ctx, tx, principal.UserID)
		if err != nil {
			return err
		}
		changed := []string{}
		if in.DisplayName != nil {
			current.DisplayName = name
			changed = append(changed, "display_name")
		}
		if in.PreferredLanguage != nil {
			current.PreferredLanguage = language
			changed = append(changed, "preferred_language")
		}
		if in.NotificationPreferences != nil {
			for key, value := range *in.NotificationPreferences {
				current.NotificationPreferences[key] = value
			}
			changed = append(changed, "notification_preferences")
		}
		if in.PrivacyPreferences != nil {
			for key, value := range *in.PrivacyPreferences {
				current.PrivacyPreferences[key] = value
			}
			changed = append(changed, "privacy_preferences")
		}
		notifications, err := json.Marshal(current.NotificationPreferences)
		if err != nil {
			return err
		}
		privacy, err := json.Marshal(current.PrivacyPreferences)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE user_profiles
			SET display_name = $2, preferred_language = $3, notification_preferences = $4, privacy_preferences = $5
			WHERE user_id = $1
		`, principal.UserID, current.DisplayName, current.PreferredLanguage, notifications, privacy); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "profile.update", ResourceType: "user", ResourceID: &principal.UserID,
			RequestID: in.RequestID, Metadata: map[string]any{"fields": changed},
		}); err != nil {
			return err
		}
		user = current
		return nil
	})
	return user, err
}

func (s *Service) Locations(ctx context.Context, principal authn.Principal) ([]views.Location, error) {
	var places []views.Location
	err := s.DB.WithFlags(ctx, &principal.UserID, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, label, address_text, locality_text, latitude, longitude, source, created_at
			FROM locations WHERE user_id = $1 ORDER BY created_at DESC
		`, principal.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var place views.Location
			var created time.Time
			if err := rows.Scan(&place.ID, &place.Label, &place.AddressText, &place.LocalityText, &place.Latitude, &place.Longitude, &place.Source, &created); err != nil {
				return err
			}
			place.CreatedAt = created.UTC().Format(time.RFC3339)
			places = append(places, place)
		}
		return rows.Err()
	})
	if places == nil {
		places = []views.Location{}
	}
	return places, err
}

func (s *Service) CreateLocation(ctx context.Context, principal authn.Principal, in LocationInput) (views.Location, error) {
	fields := map[string]string{}
	switch in.Source {
	case "user_gps", "user_pin":
		if in.Latitude == nil || in.Longitude == nil {
			fields["latitude"] = "Choose a point on the map or share a device location. Coordinates are not guessed."
		}
	case "user_address":
		if in.AddressText == nil || strings.TrimSpace(*in.AddressText) == "" {
			fields["address_text"] = "Enter an address."
		}
	case "user_locality":
		if in.LocalityText == nil || strings.TrimSpace(*in.LocalityText) == "" {
			fields["locality_text"] = "Enter a locality."
		}
	default:
		fields["source"] = "Choose user_gps, user_pin, user_address, or user_locality."
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		fields["longitude"] = "Provide both latitude and longitude, or neither."
	}
	if in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90) {
		fields["latitude"] = "Latitude must be between -90 and 90."
	}
	if in.Longitude != nil && (*in.Longitude < -180 || *in.Longitude > 180) {
		fields["longitude"] = "Longitude must be between -180 and 180."
	}
	if invalid(fields) {
		return views.Location{}, httpx.Validation(fields)
	}
	var place views.Location
	err := s.DB.WithFlags(ctx, &principal.UserID, nil, func(tx pgx.Tx) error {
		var created time.Time
		err := tx.QueryRow(ctx, `
			INSERT INTO locations (user_id, label, address_text, locality_text, latitude, longitude, geom, source)
			VALUES (
				$1, $2, $3, $4, $5, $6,
				CASE WHEN $5::float8 IS NULL THEN NULL
				     ELSE ST_SetSRID(ST_MakePoint($6::float8, $5::float8), 4326)::geography END,
				$7
			)
			RETURNING id::text, label, address_text, locality_text, latitude, longitude, source, created_at
		`, principal.UserID, trimPtr(in.Label), trimPtr(in.AddressText), trimPtr(in.LocalityText), in.Latitude, in.Longitude, in.Source).Scan(
			&place.ID, &place.Label, &place.AddressText, &place.LocalityText, &place.Latitude, &place.Longitude, &place.Source, &created,
		)
		if err != nil {
			return err
		}
		place.CreatedAt = created.UTC().Format(time.RFC3339)
		locationID, err := uuid.Parse(place.ID)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "location.create", ResourceType: "location", ResourceID: &locationID,
			RequestID: in.RequestID, Metadata: map[string]any{"source": in.Source, "has_coordinates": in.Latitude != nil},
		})
	})
	return place, err
}

func (s *Service) DeleteLocation(ctx context.Context, principal authn.Principal, locationID uuid.UUID, requestID string) error {
	return s.DB.WithFlags(ctx, &principal.UserID, nil, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM locations WHERE id = $1 AND user_id = $2`, locationID, principal.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(404, "not_found", "That saved place was not found.")
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "location.delete", ResourceType: "location", ResourceID: &locationID,
			RequestID: requestID,
		})
	})
}

func (s *Service) RequestDeletion(ctx context.Context, principal authn.Principal, requestID string) (views.PrivacyRequest, error) {
	var request views.PrivacyRequest
	err := s.DB.WithFlags(ctx, &principal.UserID, nil, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id::text, request_type, status, created_at
			FROM privacy_requests
			WHERE user_id = $1 AND request_type = 'deletion' AND status = 'submitted'
			ORDER BY created_at DESC
			LIMIT 1
		`, principal.UserID).Scan(&request.ID, &request.RequestType, &request.Status, &request.CreatedAt)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO privacy_requests (user_id, request_type) VALUES ($1, 'deletion')
			RETURNING id::text, request_type, status, created_at
		`, principal.UserID).Scan(&request.ID, &request.RequestType, &request.Status, &request.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET status = 'pending_deletion' WHERE id = $1 AND status = 'active'
		`, principal.UserID); err != nil {
			return err
		}
		requestIDValue, err := uuid.Parse(request.ID)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, Action: "privacy.deletion_requested", ResourceType: "privacy_request",
			ResourceID: &requestIDValue, RequestID: requestID,
		}); err != nil {
			return err
		}
		return events.Enqueue(ctx, tx, "user", principal.UserID, "privacy.deletion_requested", map[string]any{
			"user_id": principal.UserID.String(), "request_id": requestID,
		})
	})
	return request, err
}

func (s *Service) FeatureFlags(ctx context.Context) (map[string]bool, error) {
	flags := map[string]bool{}
	err := s.DB.WithFlags(ctx, nil, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT key, enabled FROM feature_flags
			WHERE key IN ('org_self_service', 'otp_login', 'ai_intake')
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var enabled bool
			if err := rows.Scan(&key, &enabled); err != nil {
				return err
			}
			flags[key] = enabled
		}
		return rows.Err()
	})
	return flags, err
}

func preferenceError(values map[string]bool, allowed []string) string {
	allowedSet := map[string]struct{}{}
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range values {
		if _, ok := allowedSet[key]; !ok {
			return "Unsupported preference."
		}
	}
	return ""
}

func invalid(fields map[string]string) bool {
	for _, value := range fields {
		if value != "" {
			return true
		}
	}
	return false
}

func trimPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
