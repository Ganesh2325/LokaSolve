package identity

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/views"
)

func LoadUser(ctx context.Context, tx pgx.Tx, id uuid.UUID) (views.User, error) {
	var user views.User
	var notifications, privacy []byte
	err := tx.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.phone_e164, u.status, u.is_demo,
		       u.email_verified_at IS NOT NULL, u.phone_verified_at IS NOT NULL,
		       p.display_name, p.preferred_language, p.notification_preferences, p.privacy_preferences
		FROM users u
		JOIN user_profiles p ON p.user_id = u.id
		WHERE u.id = $1
	`, id).Scan(
		&user.ID, &user.Email, &user.Phone, &user.Status, &user.IsDemo,
		&user.EmailVerified, &user.PhoneVerified,
		&user.DisplayName, &user.PreferredLanguage, &notifications, &privacy,
	)
	if err != nil {
		return views.User{}, err
	}
	if err := json.Unmarshal(notifications, &user.NotificationPreferences); err != nil {
		return views.User{}, err
	}
	if err := json.Unmarshal(privacy, &user.PrivacyPreferences); err != nil {
		return views.User{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT r.code
		FROM user_role_assignments ura
		JOIN roles r ON r.id = ura.role_id
		WHERE ura.user_id = $1 AND ura.organization_id IS NULL
		ORDER BY r.code
	`, id)
	if err != nil {
		return views.User{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return views.User{}, err
		}
		user.Roles = append(user.Roles, code)
	}
	if err := rows.Err(); err != nil {
		return views.User{}, err
	}
	if user.Roles == nil {
		user.Roles = []string{}
	}
	return user, nil
}

func AssignRole(ctx context.Context, tx pgx.Tx, userID uuid.UUID, roleCode string, organizationID *uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO user_role_assignments (user_id, role_id, organization_id)
		SELECT $1, id, $3 FROM roles WHERE code = $2
		ON CONFLICT DO NOTHING
	`, userID, roleCode, organizationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM user_role_assignments ura
				JOIN roles r ON r.id = ura.role_id
				WHERE ura.user_id = $1 AND r.code = $2 AND ura.organization_id IS NOT DISTINCT FROM $3
			)
		`, userID, roleCode, organizationID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("unknown role")
		}
	}
	return nil
}
