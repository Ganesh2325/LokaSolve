//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"lokasolve/services/api-go/internal/events"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/config"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/ratelimit"
	"lokasolve/services/api-go/internal/server"
)

var (
	appDB    *database.DB
	adminDB  *pgx.Conn
	api      *httptest.Server
	otpSink  *authn.CapturingSender
	rdb      *redis.Client
	testConf config.Config
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	maintenance := env("MIGRATE_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
	conn, err := pgx.Connect(ctx, maintenance)
	if err != nil {
		panic(err)
	}
	_, _ = conn.Exec(ctx, `CREATE DATABASE lokasolve_test`)
	conn.Close(ctx)

	adminURL := strings.Replace(maintenance, "/postgres?", "/lokasolve_test?", 1)
	if !strings.Contains(adminURL, "lokasolve_test") {
		adminURL = "postgres://postgres:postgres@localhost:5432/lokasolve_test?sslmode=disable"
	}
	adminDB, err = pgx.Connect(ctx, adminURL)
	if err != nil {
		panic(err)
	}
	if err := database.Migrate(ctx, adminDB); err != nil {
		panic(err)
	}
	appURL := env("TEST_DATABASE_URL", "postgres://lokasolve_app:lokasolve_app@localhost:5432/lokasolve_test?sslmode=disable")
	appDB, err = database.Connect(ctx, appURL)
	if err != nil {
		panic(err)
	}
	rdb = redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	otpSink = authn.NewCapturingSender()
	testConf = config.Config{
		Env: "test", AuthTokenSecret: strings.Repeat("s", 32), OTPPepper: strings.Repeat("p", 32),
		AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour, LockoutThreshold: 5, LockoutDuration: time.Minute,
		CORSOrigins: []string{"http://localhost:3000"},
	}
	built := server.New(testConf, appDB, rdb, otpSink, ratelimit.NewMemory())
	api = httptest.NewServer(built.Handler)
	code := m.Run()
	api.Close()
	appDB.Close()
	adminDB.Close(ctx)
	os.Exit(code)
}

func TestResidentCanRegisterAndCannotAdminister(t *testing.T) {
	reset(t)
	session := register(t, "ada@example.com", "resident", "", "")
	me := authed(t, http.MethodGet, "/api/v1/me", session.AccessToken, nil, http.StatusOK)
	if me["display_name"] != "Ada Example" || me["is_demo"] != false {
		t.Fatalf("profile = %#v", me)
	}
	authed(t, http.MethodGet, "/api/v1/admin/users", session.AccessToken, nil, http.StatusForbidden)
	var audits int
	if err := adminDB.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action = 'auth.register'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count %d err %v", audits, err)
	}
}

func TestLoginLockout(t *testing.T) {
	reset(t)
	register(t, "lock@example.com", "resident", "", "")
	for i := 0; i < 5; i++ {
		status := post(t, "/api/v1/auth/login", map[string]string{"email": "lock@example.com", "password": "wrong-password"}, "")
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d status %d", i, status)
		}
	}
	status := post(t, "/api/v1/auth/login", map[string]string{"email": "lock@example.com", "password": "correct-horse"}, "")
	if status != http.StatusLocked {
		t.Fatalf("expected lock, got %d", status)
	}
}

func TestIdempotentRegisterCreatesOneUser(t *testing.T) {
	reset(t)
	body := map[string]string{"email": "idem@example.com", "password": "correct-horse", "display_name": "Idem"}
	raw := mustJSON(body)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, api.URL+"/api/v1/auth/register", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "register-idem-001")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status %d", res.StatusCode)
		}
	}
	var count int
	if err := adminDB.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = 'idem@example.com'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("users %d err %v", count, err)
	}
}

func TestRefreshReuseRevokesTheFamily(t *testing.T) {
	reset(t)
	first := register(t, "refresh@example.com", "resident", "", "")
	second := refresh(t, first.RefreshToken)
	// A second presentation inside the overlap joins the rotation that just happened.
	joined := refresh(t, first.RefreshToken)
	if joined.RefreshToken != second.RefreshToken {
		t.Fatal("overlapping refresh minted a second session")
	}
	if post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": second.RefreshToken}, "") != http.StatusOK {
		t.Fatal("successor refresh failed")
	}
	// The original token is now a replay: its successor was rotated again.
	reused := post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": first.RefreshToken}, "")
	if reused != http.StatusUnauthorized {
		t.Fatalf("reused status %d", reused)
	}
	again := post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": second.RefreshToken}, "")
	if again != http.StatusUnauthorized {
		t.Fatalf("rotated token survived reuse, status %d", again)
	}
}

func TestOrganizationIsolation(t *testing.T) {
	reset(t)
	owner := register(t, "owner@example.com", "resident", "", "")
	other := register(t, "other@example.com", "resident", "", "")
	created := authed(t, http.MethodPost, "/api/v1/organizations", owner.AccessToken, map[string]string{
		"name": "Ward Office", "org_type": "municipality",
	}, http.StatusCreated)
	orgID, _ := created["id"].(string)
	authed(t, http.MethodGet, "/api/v1/organizations/"+orgID, other.AccessToken, nil, http.StatusNotFound)
	authed(t, http.MethodPost, "/api/v1/organizations/"+orgID+"/members", other.AccessToken, map[string]string{
		"email": "owner@example.com", "member_role": "member",
	}, http.StatusNotFound)
	authed(t, http.MethodPost, "/api/v1/organizations/"+orgID+"/members", owner.AccessToken, map[string]string{
		"email": "other@example.com", "member_role": "field_worker",
	}, http.StatusNoContent)
	members := authed(t, http.MethodGet, "/api/v1/organizations/"+orgID+"/members", owner.AccessToken, nil, http.StatusOK)
	data, _ := members["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("members %#v", members)
	}
}

func TestSupportCanReadUsersButCannotGrantRoles(t *testing.T) {
	reset(t)
	adminSession := insertStaff(t, "root@example.com", "super_admin")
	resident := register(t, "person@example.com", "resident", "", "")
	_ = resident
	var residentID string
	if err := adminDB.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = 'person@example.com'`).Scan(&residentID); err != nil {
		t.Fatal(err)
	}
	authed(t, http.MethodPost, "/api/v1/admin/users/"+residentID+"/roles", adminSession.AccessToken, map[string]string{"role_code": "support_agent"}, http.StatusNoContent)
	support := login(t, "person@example.com")
	authed(t, http.MethodGet, "/api/v1/admin/users", support.AccessToken, nil, http.StatusOK)
	authed(t, http.MethodPost, "/api/v1/admin/users/"+residentID+"/roles", support.AccessToken, map[string]string{"role_code": "super_admin"}, http.StatusForbidden)
	page := authed(t, http.MethodGet, "/api/v1/admin/audit-logs", adminSession.AccessToken, nil, http.StatusOK)
	raw, _ := json.Marshal(page)
	if !bytes.Contains(raw, []byte("admin.role_grant")) {
		t.Fatalf("audit missing grant: %s", raw)
	}
}

func TestLocationIsNotInvented(t *testing.T) {
	reset(t)
	session := register(t, "place@example.com", "resident", "", "")
	created := authed(t, http.MethodPost, "/api/v1/me/locations", session.AccessToken, map[string]string{
		"source": "user_address", "address_text": "12 Market Road, Pune", "label": "Home",
	}, http.StatusCreated)
	if created["latitude"] != nil || created["longitude"] != nil {
		t.Fatalf("coordinates were invented: %#v", created)
	}
	var geom *string
	if err := adminDB.QueryRow(context.Background(), `SELECT ST_AsText(geom::geometry) FROM locations`).Scan(&geom); err != nil {
		t.Fatal(err)
	}
	if geom != nil {
		t.Fatalf("geom = %v", *geom)
	}
	authed(t, http.MethodPost, "/api/v1/me/locations", session.AccessToken, map[string]any{
		"source": "user_pin", "latitude": 18.5,
	}, http.StatusBadRequest)
}

func TestSavedPlaceIsPrivateUnderRLS(t *testing.T) {
	reset(t)
	owner := register(t, "private@example.com", "resident", "", "")
	authed(t, http.MethodPost, "/api/v1/me/locations", owner.AccessToken, map[string]any{
		"source": "user_pin", "latitude": 18.52, "longitude": 73.85, "label": "Gate",
	}, http.StatusCreated)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, env("TEST_DATABASE_URL", "postgres://lokasolve_app:lokasolve_app@localhost:5432/lokasolve_test?sslmode=disable"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM locations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unscoped app query saw %d locations", count)
	}
}

func TestOTPLoginAndPhoneVerification(t *testing.T) {
	reset(t)
	session := register(t, "phone@example.com", "resident", "", "")
	authed(t, http.MethodPost, "/api/v1/me/phone/otp/request", session.AccessToken, map[string]string{
		"phone": "+919800000001", "purpose": "verify_phone",
	}, http.StatusAccepted)
	code := otpSink.Code("+919800000001")
	if len(code) != 6 {
		t.Fatalf("code %q", code)
	}
	authed(t, http.MethodPost, "/api/v1/me/phone/otp/verify", session.AccessToken, map[string]string{
		"phone": "+919800000001", "purpose": "verify_phone", "code": code,
	}, http.StatusOK)
	status := post(t, "/api/v1/auth/otp/request", map[string]string{"phone": "+919800000001", "purpose": "login"}, "")
	if status != http.StatusAccepted {
		t.Fatalf("otp request %d", status)
	}
	loginCode := otpSink.Code("+919800000001")
	loginStatus := post(t, "/api/v1/auth/otp/verify", map[string]string{
		"phone": "+919800000001", "purpose": "login", "code": loginCode,
	}, "")
	if loginStatus != http.StatusOK {
		t.Fatalf("otp login %d", loginStatus)
	}
}

func TestOutboxSurvivesAWorkerCrashBeforeAck(t *testing.T) {
	reset(t)
	register(t, "event@example.com", "resident", "", "")
	ctx := context.Background()
	var unpublished int
	if err := adminDB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND event_type = 'user.registered'`).Scan(&unpublished); err != nil || unpublished != 1 {
		t.Fatalf("unpublished %d err %v", unpublished, err)
	}
	stream := events.RedisStream{Client: rdb, Name: events.StreamName}
	if _, err := events.PublishBatch(ctx, appDB, stream, 10); err != nil {
		t.Fatal(err)
	}
	if err := adminDB.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND event_type = 'user.registered'`).Scan(&unpublished); err != nil || unpublished != 0 {
		t.Fatalf("still unpublished %d err %v", unpublished, err)
	}
	if err := stream.EnsureGroup(ctx, "test-workers"); err != nil {
		t.Fatal(err)
	}
	var eventID string
	if err := adminDB.QueryRow(ctx, `SELECT id::text FROM outbox_events WHERE event_type = 'user.registered' LIMIT 1`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := stream.Append(ctx, events.Event{ID: eventID, Type: "user.registered", Payload: []byte(`{"replay":true}`)}); err != nil {
		t.Fatal(err)
	}
	pending, err := stream.ReadNew(ctx, "test-workers", "crashed", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("expected an unacknowledged event")
	}
	claimed, err := stream.Claim(ctx, "test-workers", "restarted", 10)
	if err != nil {
		t.Fatal(err)
	}
	handled := 0
	for _, event := range claimed {
		first, err := stream.FirstTime(ctx, event.ID)
		if err != nil {
			t.Fatal(err)
		}
		if first {
			handled++
		}
		if err := stream.Ack(ctx, "test-workers", event.RedisID); err != nil {
			t.Fatal(err)
		}
	}
	if handled != 1 {
		t.Fatalf("handled %d events", handled)
	}
}

func TestWhatsAppPreferenceDefaultsClosed(t *testing.T) {
	reset(t)
	session := register(t, "prefs@example.com", "resident", "", "")
	me := authed(t, http.MethodGet, "/api/v1/me", session.AccessToken, nil, http.StatusOK)
	prefs, _ := me["notification_preferences"].(map[string]any)
	if prefs["whatsapp"] != false {
		t.Fatalf("whatsapp default %#v", prefs)
	}
	updated := authed(t, http.MethodPatch, "/api/v1/me", session.AccessToken, map[string]any{
		"notification_preferences": map[string]bool{"whatsapp": true},
	}, http.StatusOK)
	prefs, _ = updated["notification_preferences"].(map[string]any)
	if prefs["whatsapp"] != true {
		t.Fatalf("opt-in not stored %#v", prefs)
	}
	updated = authed(t, http.MethodPatch, "/api/v1/me", session.AccessToken, map[string]any{
		"notification_preferences": map[string]bool{"whatsapp": false},
	}, http.StatusOK)
	prefs, _ = updated["notification_preferences"].(map[string]any)
	if prefs["whatsapp"] != false {
		t.Fatalf("opt-out not stored %#v", prefs)
	}
}

type sessionBody struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func register(t *testing.T, email, kind, orgName, orgType string) sessionBody {
	t.Helper()
	body := map[string]string{
		"email": email, "password": "correct-horse", "display_name": "Ada Example", "account_kind": kind,
		"organization_name": orgName, "organization_type": orgType,
	}
	res := request(t, http.MethodPost, "/api/v1/auth/register", "", body, http.StatusCreated)
	var session sessionBody
	decode(t, res, &session)
	return session
}

func login(t *testing.T, email string) sessionBody {
	t.Helper()
	res := request(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": "correct-horse"}, http.StatusOK)
	var session sessionBody
	decode(t, res, &session)
	return session
}

func refresh(t *testing.T, token string) sessionBody {
	t.Helper()
	res := request(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": token}, http.StatusOK)
	var session sessionBody
	decode(t, res, &session)
	return session
}

func insertStaff(t *testing.T, email, role string) sessionBody {
	t.Helper()
	hash, err := authn.HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var userID string
	if err := adminDB.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_demo) VALUES ($1, $2, true) RETURNING id::text
	`, email, hash).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `INSERT INTO user_profiles (user_id, display_name) VALUES ($1, 'Demo Staff')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `
		INSERT INTO user_role_assignments (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = 'resident'
	`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `
		INSERT INTO user_role_assignments (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = $2
	`, userID, role); err != nil {
		t.Fatal(err)
	}
	return login(t, email)
}

func authed(t *testing.T, method, path, token string, body any, want int) map[string]any {
	t.Helper()
	res := request(t, method, path, token, body, want)
	if res.StatusCode == http.StatusNoContent {
		res.Body.Close()
		return nil
	}
	defer res.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return payload
}

func post(t *testing.T, path string, body any, token string) int {
	t.Helper()
	res := request(t, http.MethodPost, path, token, body, 0)
	res.Body.Close()
	return res.StatusCode
}

func request(t *testing.T, method, path, token string, body any, want int) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(mustJSON(body))
	}
	req, err := http.NewRequest(method, api.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if want != 0 && res.StatusCode != want {
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("%s %s: got %d want %d body %s", method, path, res.StatusCode, want, raw)
	}
	return res
}

func decode(t *testing.T, res *http.Response, dst any) {
	t.Helper()
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		t.Fatal(err)
	}
}

func reset(t *testing.T) {
	t.Helper()
	_, err := adminDB.Exec(context.Background(), `
		TRUNCATE audit_logs, outbox_events, idempotency_keys, otp_challenges, sessions,
		         locations, privacy_requests, teams, departments, organization_members,
		         user_role_assignments, user_contacts, user_profiles, organizations, users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
