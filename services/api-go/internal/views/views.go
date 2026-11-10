package views

import "time"

type User struct {
	ID                      string          `json:"id"`
	Email                   *string         `json:"email"`
	Phone                   *string         `json:"phone_e164"`
	DisplayName             string          `json:"display_name"`
	PreferredLanguage       string          `json:"preferred_language"`
	Status                  string          `json:"status"`
	IsDemo                  bool            `json:"is_demo"`
	Roles                   []string        `json:"roles"`
	NotificationPreferences map[string]bool `json:"notification_preferences"`
	PrivacyPreferences      map[string]bool `json:"privacy_preferences"`
	EmailVerified           bool            `json:"email_verified"`
	PhoneVerified           bool            `json:"phone_verified"`
}

type Session struct {
	User         User   `json:"user"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type Location struct {
	ID           string   `json:"id"`
	Label        *string  `json:"label"`
	AddressText  *string  `json:"address_text"`
	LocalityText *string  `json:"locality_text"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	Source       string   `json:"source"`
	CreatedAt    string   `json:"created_at"`
}

type Organization struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"org_type"`
	Status     string `json:"status"`
	IsDemo     bool   `json:"is_demo"`
	MemberRole string `json:"member_role,omitempty"`
}

type Member struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	MemberRole  string `json:"member_role"`
	Status      string `json:"status"`
}

type Department struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Team struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	DepartmentID *string `json:"department_id"`
}

type AdminUser struct {
	ID          string    `json:"id"`
	Email       *string   `json:"email"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	IsDemo      bool      `json:"is_demo"`
	Roles       []string  `json:"roles"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditRecord struct {
	ID           string         `json:"id"`
	ActorUserID  *string        `json:"actor_user_id"`
	ActorRole    string         `json:"actor_role"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *string        `json:"resource_id"`
	TenantID     *string        `json:"tenant_id"`
	RequestID    string         `json:"request_id"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

type PrivacyRequest struct {
	ID          string    `json:"id"`
	RequestType string    `json:"request_type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
}
