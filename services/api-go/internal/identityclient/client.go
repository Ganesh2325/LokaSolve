package identityclient

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Membership struct {
	OrganizationID uuid.UUID
	MemberRole     string
	OrgType        string
	Status         string
}

type Client struct {
	base   string
	token  string
	http   *http.Client
	mu     sync.Mutex
	keys   map[string]*rsa.PublicKey
	loaded time.Time
	cache  sync.Map
}

type cached struct {
	at          time.Time
	principal   authn.Principal
	memberships []Membership
}

type claims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
}

func New(base, token string) *Client {
	return &Client{
		base: base, token: token,
		http: &http.Client{Timeout: 3 * time.Second},
		keys: map[string]*rsa.PublicKey{},
	}
}

func (c *Client) Resolve(ctx context.Context, raw string) (authn.Principal, []Membership, error) {
	userID, sessionID, err := c.parse(ctx, raw)
	if err != nil {
		return authn.Principal{}, nil, httpx.New(http.StatusUnauthorized, "invalid_session", "Sign in again.")
	}
	if item, ok := c.cache.Load(sessionID.String()); ok {
		cachedItem := item.(cached)
		if time.Since(cachedItem.at) < 5*time.Second {
			return cachedItem.principal, cachedItem.memberships, nil
		}
	}
	endpoint := c.base + "/internal/v1/principals/" + url.PathEscape(userID.String()) + "?session_id=" + url.QueryEscape(sessionID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return authn.Principal{}, nil, err
	}
	req.Header.Set("X-Identity-Service-Token", c.token)
	res, err := c.http.Do(req)
	if err != nil {
		return authn.Principal{}, nil, httpx.New(http.StatusServiceUnavailable, "identity_unavailable", "Try again shortly.")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		c.cache.Delete(sessionID.String())
		return authn.Principal{}, nil, httpx.New(http.StatusUnauthorized, "invalid_session", "Sign in again.")
	}
	if res.StatusCode != http.StatusOK {
		return authn.Principal{}, nil, httpx.New(http.StatusServiceUnavailable, "identity_unavailable", "Try again shortly.")
	}
	var body struct {
		UserID      string   `json:"user_id"`
		SessionID   string   `json:"session_id"`
		Status      string   `json:"status"`
		IsDemo      bool     `json:"is_demo"`
		Locked      bool     `json:"locked"`
		Roles       []string `json:"roles"`
		Permissions []string `json:"permissions"`
		Memberships []struct {
			OrganizationID string `json:"organization_id"`
			MemberRole     string `json:"member_role"`
			OrgType        string `json:"org_type"`
			Status         string `json:"status"`
		} `json:"memberships"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return authn.Principal{}, nil, err
	}
	principal := authn.Principal{
		UserID: userID, SessionID: sessionID, Status: body.Status, IsDemo: body.IsDemo, Locked: body.Locked,
		GlobalRoles: map[string]struct{}{}, Permissions: map[string]struct{}{}, MemberRole: map[uuid.UUID]string{},
	}
	for _, role := range body.Roles {
		principal.GlobalRoles[role] = struct{}{}
	}
	for _, permission := range body.Permissions {
		principal.Permissions[permission] = struct{}{}
	}
	memberships := make([]Membership, 0, len(body.Memberships))
	for _, item := range body.Memberships {
		orgID, err := uuid.Parse(item.OrganizationID)
		if err != nil {
			continue
		}
		principal.MemberRole[orgID] = item.MemberRole
		memberships = append(memberships, Membership{
			OrganizationID: orgID, MemberRole: item.MemberRole, OrgType: item.OrgType, Status: item.Status,
		})
	}
	c.cache.Store(sessionID.String(), cached{at: time.Now(), principal: principal, memberships: memberships})
	return principal, memberships, nil
}

func (c *Client) parse(ctx context.Context, raw string) (uuid.UUID, uuid.UUID, error) {
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		kid, _ := token.Header["kid"].(string)
		return c.key(ctx, kid)
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithIssuer("lokasolve-identity"), jwt.WithAudience("lokasolve"))
	if err != nil || !parsed.Valid {
		return uuid.Nil, uuid.Nil, err
	}
	access, _ := parsed.Claims.(*claims)
	userID, err := uuid.Parse(access.Subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	sessionID, err := uuid.Parse(access.SessionID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return userID, sessionID, nil
}

func (c *Client) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key := c.keys[kid]; key != nil && time.Since(c.loaded) < 5*time.Minute {
		return key, nil
	}
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	key := c.keys[kid]
	if key == nil {
		return nil, fmt.Errorf("unknown signing key")
	}
	return key, nil
}

func (c *Client) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/.well-known/jwks.json", nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, item := range body.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil {
			return err
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil {
			return err
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 + int(b)
		}
		keys[item.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	c.keys = keys
	c.loaded = time.Now()
	return nil
}
