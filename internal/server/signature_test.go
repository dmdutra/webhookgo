package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dbaio/webhookgo/internal/config"
	"github.com/dbaio/webhookgo/internal/store"
)

func githubSig(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (e *env) last(id string) store.Request {
	e.t.Helper()
	var r store.Request
	e.json(e.do("GET", "/token/"+id+"/request/latest", nil), http.StatusOK, &r)
	return r
}

func TestSignatureSettingsHideTheSecret(t *testing.T) {
	e := setup(t)
	resp := e.do("POST", "/token", strings.NewReader(`{"signature":{"scheme":"github","secret":"top-secret"}}`), "Content-Type", "application/json")
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated || strings.Contains(string(b), "top-secret") {
		t.Fatalf("create: %d %s", resp.StatusCode, b)
	}
	var tok struct {
		UUID      string         `json:"uuid"`
		Signature map[string]any `json:"signature"`
	}
	_ = json.Unmarshal(b, &tok)
	if tok.Signature["scheme"] != "github" || tok.Signature["has_secret"] != true || tok.Signature["reject"] != false {
		t.Fatalf("signature view: %v", tok.Signature)
	}
	if _, ok := tok.Signature["secret"]; ok {
		t.Fatal("the secret must not be returned")
	}

	// A partial update keeps the secret; null turns verification off.
	resp = e.do("PATCH", "/token/"+tok.UUID, strings.NewReader(`{"signature":{"reject":true,"secret":""}}`), "Content-Type", "application/json")
	b, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"reject":true`) || !strings.Contains(string(b), `"has_secret":true`) {
		t.Fatalf("patch: %d %s", resp.StatusCode, b)
	}
	e.do("POST", "/"+tok.UUID, strings.NewReader("x"), "X-Hub-Signature-256", githubSig("top-secret", "x"))
	if s := e.last(tok.UUID).Signature; s == nil || s.Status != "valid" {
		t.Fatalf("secret kept across updates: %+v", s)
	}
	var cleared map[string]any
	e.json(e.do("PATCH", "/token/"+tok.UUID, strings.NewReader(`{"signature":null}`), "Content-Type", "application/json"), http.StatusOK, &cleared)
	if _, ok := cleared["signature"]; ok {
		t.Fatalf("signature not removed: %v", cleared)
	}
	e.do("POST", "/"+tok.UUID, strings.NewReader("y"))
	if s := e.last(tok.UUID).Signature; s != nil {
		t.Fatalf("checked after removal: %+v", s)
	}

	for _, bad := range []string{
		`{"signature":{"scheme":"md5","secret":"s"}}`,
		`{"signature":{"scheme":"github"}}`,
		`{"signature":{"scheme":"malga","secret":"not a key"}}`,
		`{"signature":{"scheme":"hmac","secret":"s"}}`,
		`{"signature":{"scheme":"stripe","secret":"s","tolerance":-1}}`,
		`{"signature":"github"}`,
	} {
		e.json(e.do("POST", "/token", strings.NewReader(bad), "Content-Type", "application/json"), http.StatusUnprocessableEntity, nil)
	}
	// Switching provider needs the new provider's secret.
	e.json(e.do("PATCH", "/token/"+tok.UUID, strings.NewReader(`{"signature":{"scheme":"github","secret":"a"}}`), "Content-Type", "application/json"), http.StatusOK, nil)
	e.json(e.do("PATCH", "/token/"+tok.UUID, strings.NewReader(`{"signature":{"scheme":"slack"}}`), "Content-Type", "application/json"), http.StatusUnprocessableEntity, nil)
}

func TestSignatureChecksAndRejection(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.MaxBodySize = 16 })
	id := e.newToken(`{"default_status":202,"default_content":"ok","cors":true,"signature":{"scheme":"github","secret":"s3"}}`)["uuid"].(string)

	// The signature covers the whole body, even past MAX_BODY_SIZE.
	big := strings.Repeat("a", 100)
	resp := e.do("POST", "/"+id, strings.NewReader(big), "X-Hub-Signature-256", githubSig("s3", big))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("valid: %d", resp.StatusCode)
	}
	if r := e.last(id); r.Signature == nil || r.Signature.Status != "valid" || r.Signature.Scheme != "github" || !r.Truncated {
		t.Fatalf("valid: %+v %+v", r.Signature, r)
	}
	// Without reject, unsigned requests get the normal response.
	e.json(e.do("POST", "/"+id, strings.NewReader("x")), http.StatusAccepted, nil)
	var sums struct {
		Data  []store.Summary `json:"data"`
		Total int             `json:"total"`
	}
	e.json(e.do("GET", "/token/"+id+"/requests?summary=1&sorting=newest", nil), http.StatusOK, &sums)
	if sums.Total != 2 || sums.Data[0].Signature != "missing" || sums.Data[1].Signature != "valid" {
		t.Fatalf("summaries: %+v", sums.Data)
	}

	e.json(e.do("PATCH", "/token/"+id, strings.NewReader(`{"signature":{"reject":true}}`), "Content-Type", "application/json"), http.StatusOK, nil)
	resp = e.do("POST", "/"+id, strings.NewReader("x"), "X-Hub-Signature-256", githubSig("wrong", "x"), "Origin", "https://app.example")
	var rej map[string]any
	e.json(resp, http.StatusUnauthorized, &rej)
	if !strings.Contains(rej["error"].(string), "signature invalid") || resp.Header.Get("X-Request-Id") == "" ||
		resp.Header.Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("rejection: %v %v", rej, resp.Header)
	}
	if r := e.last(id); r.ResponseStatus != http.StatusUnauthorized || r.Signature.Status != "invalid" {
		t.Fatalf("rejected request is still stored: %+v", r)
	}
	e.json(e.do("POST", "/"+id, strings.NewReader("x"), "X-Hub-Signature-256", githubSig("s3", "x")), http.StatusAccepted, nil)

	// CORS preflights are not signed, so they are never rejected.
	resp = e.do("OPTIONS", "/"+id, nil, "Origin", "https://app.example", "Access-Control-Request-Method", "POST")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight: %d", resp.StatusCode)
	}
	if r := e.last(id); r.Signature != nil {
		t.Fatalf("preflight checked: %+v", r.Signature)
	}
}

func TestSignatureMalga(t *testing.T) {
	e := setup(t)
	in, _ := json.Marshal(map[string]any{"signature": map[string]any{
		"scheme": "malga", "secret": "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAS8O+o7Cqd3mtpriJvOVK/cdo8se5VU5vZkCMUttd0WA=\n-----END PUBLIC KEY-----\n",
	}})
	var tok map[string]any
	e.json(e.do("POST", "/token", strings.NewReader(string(in)), "Content-Type", "application/json"), http.StatusCreated, &tok)
	id := tok["uuid"].(string)
	if tok["signature"].(map[string]any)["tolerance"] != float64(300) {
		t.Fatalf("default tolerance: %v", tok["signature"])
	}

	// Malga's sample request (2022) is far outside the default tolerance.
	send := func() *store.Request {
		e.do("POST", "/"+id, strings.NewReader(`{"event":"ping","payload":{"object":{}}}`),
			"Content-Type", "application/json", "X-Plug-Date", "1661795163719",
			"X-Plug-Signature", "5b20c43cfd55f0c1884196786d58392e1828232e05d66e9153032ae9d374bff785d0efc4ed850ea5856b71c950bd2a1914f376381386355fce2c51163ed26e01")
		r := e.last(id)
		return &r
	}
	if r := send(); r.Signature.Status != "expired" || r.Signature.Scheme != "malga" {
		t.Fatalf("old sample: %+v", r.Signature)
	}
	e.json(e.do("PATCH", "/token/"+id, strings.NewReader(`{"signature":{"tolerance":0}}`), "Content-Type", "application/json"), http.StatusOK, nil)
	if r := send(); r.Signature.Status != "valid" {
		t.Fatalf("sample without the age check: %+v", r.Signature)
	}
}
