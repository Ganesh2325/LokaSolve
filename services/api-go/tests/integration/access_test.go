//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestReportAccessAndMediaControls(t *testing.T) {
	reset(t)
	owner := register(t, "owner@example.com", "resident", "", "")
	other := register(t, "other@example.com", "resident", "", "")
	provider := register(t, "provider@example.com", "resident", "", "")
	officer := register(t, "officer@example.com", "resident", "", "")
	support := insertStaff(t, "support@example.com", "support_agent")
	admin := insertStaff(t, "root@example.com", "super_admin")

	private := createIssue(t, owner.AccessToken, "The streetlight on lane 4 has been dark.", "Ward 12", "Lane 4, near the school gate", false)
	public := createIssue(t, owner.AccessToken, "A public drain is blocked on the main road.", "Ward 12", "12 Temple Street", true)
	outside := createIssue(t, owner.AccessToken, "This case is outside the officer locality.", "Other Town", "9 Hill Road", false)
	privateID := private["id"].(string)
	publicID := public["id"].(string)
	outsideID := outside["id"].(string)

	own := authed(t, http.MethodGet, "/api/v1/issues/"+privateID, owner.AccessToken, nil, http.StatusOK)
	if own["address_text"] == nil || own["latitude"] == nil || own["visibility"] != "owner" || own["description"] == "" {
		t.Fatalf("owner view = %#v", own)
	}
	listed := authed(t, http.MethodGet, "/api/v1/issues", owner.AccessToken, nil, http.StatusOK)
	if data, _ := listed["data"].([]any); len(data) < 3 {
		t.Fatalf("owner list = %#v", listed)
	}

	missing := notFoundMessage(t, "/api/v1/issues/"+uuid.NewString(), other.AccessToken)
	hidden := notFoundMessage(t, "/api/v1/issues/"+privateID, other.AccessToken)
	if hidden != missing || hidden != "That report is not available." {
		t.Fatalf("private message %q missing message %q", hidden, missing)
	}
	if err := otherCannotReadIssueColumns(t, "other@example.com"); err != nil {
		t.Fatal(err)
	}

	card := authed(t, http.MethodGet, "/api/v1/issues/"+publicID, other.AccessToken, nil, http.StatusOK)
	for _, key := range []string{"latitude", "longitude", "address_text", "description", "answers", "identity_visible_to_resolver"} {
		if _, ok := card[key]; ok {
			t.Fatalf("public card leaked %s: %#v", key, card)
		}
	}
	if card["visibility"] != "public" || card["locality_text"] != "Ward 12" {
		t.Fatalf("public card = %#v", card)
	}
	nearby := authed(t, http.MethodGet, "/api/v1/nearby", other.AccessToken, nil, http.StatusOK)
	rawNearby := fmt.Sprint(nearby)
	if bytes.Contains([]byte(rawNearby), []byte("Temple Street")) || bytes.Contains([]byte(rawNearby), []byte("12.97")) {
		t.Fatalf("nearby leaked place detail: %s", rawNearby)
	}

	providerID := userID(t, "provider@example.com")
	resolverID := uuid.New()
	assignmentID := uuid.New()
	if _, err := adminDB.Exec(context.Background(), `
		INSERT INTO resolvers (id, display_name, kind, skills, owner_user_id)
		VALUES ($1, 'Lane Crew', 'provider', 'Lighting', $2)
	`, resolverID.String(), providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(context.Background(), `
		INSERT INTO assignments (id, issue_id, resolver_id, status, explanation)
		VALUES ($1, $2, $3, 'accepted', 'Assigned for the lane.')
	`, assignmentID.String(), privateID, resolverID.String()); err != nil {
		t.Fatal(err)
	}
	unrelated := createIssue(t, owner.AccessToken, "A different private report stays closed.", "Ward 12", "Hidden house", false)
	if message := notFoundMessage(t, "/api/v1/issues/"+unrelated["id"].(string), provider.AccessToken); message != missing {
		t.Fatalf("unrelated provider message %q", message)
	}
	assigned := authed(t, http.MethodGet, "/api/v1/issues/"+privateID, provider.AccessToken, nil, http.StatusOK)
	if assigned["visibility"] != "assignment" || assigned["description"] == "" || assigned["address_text"] == nil {
		t.Fatalf("assignment view = %#v", assigned)
	}

	authed(t, http.MethodPost, "/api/v1/issues/"+privateID+"/classification", other.AccessToken, map[string]string{"category_label": "Lighting"}, http.StatusNotFound)
	if _, err := adminDB.Exec(context.Background(), `UPDATE assignments SET status = 'resolution_submitted' WHERE id = $1`, assignmentID.String()); err != nil {
		t.Fatal(err)
	}
	authed(t, http.MethodPost, "/api/v1/assignments/"+assignmentID.String()+"/transition", provider.AccessToken, map[string]string{"status": "resolved", "note": "not the reporter"}, http.StatusForbidden)

	grantJurisdiction(t, "officer@example.com", "Ward 12")
	seen := authed(t, http.MethodGet, "/api/v1/issues/"+privateID, officer.AccessToken, nil, http.StatusOK)
	if seen["visibility"] != "jurisdiction" || seen["locality_text"] != "Ward 12" {
		t.Fatalf("officer view = %#v", seen)
	}
	if message := notFoundMessage(t, "/api/v1/issues/"+outsideID, officer.AccessToken); message != missing {
		t.Fatalf("outside officer message %q", message)
	}
	if message := notFoundMessage(t, "/api/v1/issues/"+privateID, support.AccessToken); message != missing {
		t.Fatalf("support message %q", message)
	}
	adminView := authed(t, http.MethodGet, "/api/v1/issues/"+privateID, admin.AccessToken, nil, http.StatusOK)
	if adminView["visibility"] != "admin" || adminView["address_text"] == nil {
		t.Fatalf("admin view = %#v", adminView)
	}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	saved := uploadMedia(t, owner.AccessToken, privateID, "street.jpg", "image/jpeg", jpeg, http.StatusCreated)
	if saved["malware_scan"] != "not_scanned" || saved["file_name"] != "media.jpg" || saved["content_type"] != "image/jpeg" {
		t.Fatalf("upload = %#v", saved)
	}
	uploadMedia(t, owner.AccessToken, privateID, "note.mp3", "audio/mpeg", append([]byte("ID3"), bytes.Repeat([]byte{0}, 7)...), http.StatusCreated)
	uploadMedia(t, owner.AccessToken, privateID, "clip.mp4", "video/mp4", []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}, http.StatusCreated)
	uploadMedia(t, owner.AccessToken, privateID, "street.jpg", "image/png", jpeg, http.StatusBadRequest)
	uploadMedia(t, owner.AccessToken, privateID, "street.png", "image/jpeg", jpeg, http.StatusBadRequest)
	uploadMedia(t, owner.AccessToken, privateID, "street.jpg", "image/jpeg", []byte{0xFF, 0xD8, 0xFF}, http.StatusBadRequest)
	uploadMedia(t, owner.AccessToken, privateID, "notes.txt", "text/plain", []byte("hello world"), http.StatusBadRequest)
	uploadMedia(t, owner.AccessToken, privateID, "street.jpg", "image/jpeg", bytes.Repeat([]byte{0}, 4_000_001), http.StatusBadRequest)

	mediaID := saved["id"].(string)
	downloadMedia(t, provider.AccessToken, privateID, mediaID, http.StatusOK)
	downloadMedia(t, other.AccessToken, privateID, mediaID, http.StatusNotFound)
	downloadMedia(t, "", privateID, mediaID, http.StatusUnauthorized)
	publicSaved := uploadMedia(t, owner.AccessToken, publicID, "street.jpg", "image/jpeg", jpeg, http.StatusCreated)
	publicMedia := authed(t, http.MethodGet, "/api/v1/issues/"+publicID+"/media", other.AccessToken, nil, http.StatusOK)
	if data, _ := publicMedia["data"].([]any); len(data) != 0 {
		t.Fatalf("public media = %#v", publicMedia)
	}
	downloadMedia(t, other.AccessToken, publicID, publicSaved["id"].(string), http.StatusNotFound)
	if _, err := adminDB.Exec(context.Background(), `UPDATE assignments SET status = 'rejected' WHERE id = $1`, assignmentID.String()); err != nil {
		t.Fatal(err)
	}
	downloadMedia(t, provider.AccessToken, privateID, mediaID, http.StatusNotFound)
	if message := notFoundMessage(t, "/api/v1/issues/"+privateID, provider.AccessToken); message != missing {
		t.Fatalf("revoked provider message %q", message)
	}
	downloadMedia(t, owner.AccessToken, privateID, mediaID, http.StatusOK)
}

func TestRegisterLoginAndReportStillWork(t *testing.T) {
	reset(t)
	session := register(t, "flow@example.com", "resident", "", "")
	again := login(t, "flow@example.com")
	if again.AccessToken == "" {
		t.Fatal("login did not return a token")
	}
	created := createIssue(t, session.AccessToken, "The tap on this street is leaking.", "Ward 3", "Near the pump", false)
	got := authed(t, http.MethodGet, "/api/v1/issues/"+created["id"].(string), session.AccessToken, nil, http.StatusOK)
	if got["status"] == "" || got["description"] != "The tap on this street is leaking." {
		t.Fatalf("tracked report = %#v", got)
	}
}

func createIssue(t *testing.T, token, description, locality, address string, public bool) map[string]any {
	t.Helper()
	body := authed(t, http.MethodPost, "/api/v1/issues", token, map[string]any{
		"description": description, "locality_text": locality, "address_text": address,
		"latitude": 12.97, "longitude": 77.59, "follow_public": public, "stated_urgency": "normal",
	}, http.StatusCreated)
	issue, _ := body["issue"].(map[string]any)
	id, _ := issue["id"].(string)
	if id == "" {
		t.Fatalf("create = %#v", body)
	}
	return issue
}

func notFoundMessage(t *testing.T, path, token string) string {
	t.Helper()
	body := authed(t, http.MethodGet, path, token, nil, http.StatusNotFound)
	errObj, _ := body["error"].(map[string]any)
	message, _ := errObj["message"].(string)
	return message
}

func otherCannotReadIssueColumns(t *testing.T, email string) error {
	t.Helper()
	id := userID(t, email)
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return appDB.WithFlags(context.Background(), &parsed, nil, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(context.Background(), `SELECT count(*) FROM issues`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("another resident can see %d issue rows", count)
		}
		return nil
	})
}

func userID(t *testing.T, email string) string {
	t.Helper()
	var id string
	if err := adminDB.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func grantJurisdiction(t *testing.T, email, locality string) {
	t.Helper()
	user := userID(t, email)
	var org string
	if err := adminDB.QueryRow(context.Background(), `
		INSERT INTO organizations (name, org_type) VALUES ('Ward Office', 'municipality') RETURNING id::text
	`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(context.Background(), `
		INSERT INTO organization_members (organization_id, user_id, member_role) VALUES ($1, $2, 'member')
	`, org, user); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(context.Background(), `
		INSERT INTO organization_jurisdictions (organization_id, locality_text) VALUES ($1, $2)
	`, org, locality); err != nil {
		t.Fatal(err)
	}
}

func uploadMedia(t *testing.T, token, issueID, filename, contentType string, body []byte, want int) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, api.URL+"/api/v1/issues/"+issueID+"/media", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("upload %s: got %d want %d body %s", filename, res.StatusCode, want, raw)
	}
	if want != http.StatusCreated {
		return nil
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func downloadMedia(t *testing.T, token, issueID, mediaID string, want int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, api.URL+"/api/v1/issues/"+issueID+"/media/"+mediaID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("download got %d want %d body %s", res.StatusCode, want, raw)
	}
	if want == http.StatusOK {
		if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("X-Malware-Scan") != "not_scanned" {
			t.Fatalf("headers = %#v", res.Header)
		}
		if res.Header.Get("Content-Type") != "image/jpeg" || !bytes.Equal(raw, []byte{0xFF, 0xD8, 0xFF, 0xD9}) {
			t.Fatalf("body/type = %s %x", res.Header.Get("Content-Type"), raw)
		}
		if disposition := res.Header.Get("Content-Disposition"); disposition != `inline; filename="media.jpg"` {
			t.Fatalf("disposition = %s", disposition)
		}
	}
}
