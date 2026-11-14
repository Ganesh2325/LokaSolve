package issueclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Fact struct {
	ID             uuid.UUID  `json:"id"`
	ReporterID     uuid.UUID  `json:"reporter_user_id"`
	Description    string     `json:"description"`
	Category       *string    `json:"category_label"`
	Locality       *string    `json:"locality_text"`
	Address        *string    `json:"address_text"`
	Status         string     `json:"status"`
	FollowPublic   bool       `json:"follow_public"`
	Severity       string     `json:"severity"`
	SeveritySource string     `json:"severity_source"`
	SeverityReason string     `json:"severity_reason"`
	Safety         *string    `json:"safety_guidance"`
	EscalationDue  *time.Time `json:"escalation_due_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string) *Client {
	return &Client{base: base, token: token, http: &http.Client{Timeout: 4 * time.Second}}
}

func (c *Client) Facts(ctx context.Context, id uuid.UUID) (Fact, error) {
	var fact Fact
	err := c.call(ctx, http.MethodGet, c.base+"/internal/v1/issues/"+id.String(), nil, &fact)
	return fact, err
}

func (c *Client) Events(ctx context.Context, id uuid.UUID) ([]Event, error) {
	var body struct {
		Data []Event `json:"data"`
	}
	err := c.call(ctx, http.MethodGet, c.base+"/internal/v1/issues/"+id.String()+"/events", nil, &body)
	return body.Data, err
}

func (c *Client) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	return c.call(ctx, http.MethodPost, c.base+"/internal/v1/issues/"+id.String()+"/status", map[string]string{"status": status}, nil)
}

func (c *Client) OverrideSeverity(ctx context.Context, id, actor uuid.UUID, level, reason string) error {
	return c.call(ctx, http.MethodPost, c.base+"/internal/v1/issues/"+id.String()+"/severity", map[string]string{
		"severity": level, "reason": reason, "actor_user_id": actor.String(),
	}, nil)
}

func (c *Client) call(ctx context.Context, method, endpoint string, payload any, dest any) error {
	var buf *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(raw)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, buf)
	if err != nil {
		return err
	}
	req.Header.Set("X-Issue-Service-Token", c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return httpx.New(http.StatusServiceUnavailable, "issue_unavailable", "Try again shortly.")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return httpx.New(http.StatusNotFound, "not_found", "That report is not available.")
	}
	if res.StatusCode >= 300 {
		return httpx.New(http.StatusServiceUnavailable, "issue_unavailable", "Try again shortly.")
	}
	if dest == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(dest)
}
