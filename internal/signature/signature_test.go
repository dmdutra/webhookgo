package signature

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Vectors come from the providers' documentation where they publish one
// (GitHub, Standard Webhooks, Slack, Malga's samples repository); the
// others were computed with openssl.
const (
	slackBody = "token=xyzz0WbapA4vBCDEFasx0q6G&team_id=T1DC2JH3J&team_domain=testteamnow&channel_id=G8PSS9T3V&channel_name=foobar&user_id=U2CERLKJA&user_name=roadrunner&command=%2Fwebhook-collect&text=&response_url=https%3A%2F%2Fhooks.slack.com%2Fcommands%2FT1DC2JH3J%2F397700885554%2F96rGlfmibIGlgcZRskXaIFfN&trigger_id=398738663015.47445629121.803a0bc887a14d10d2c447fce8b6703c"
	malgaKey  = "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAS8O+o7Cqd3mtpriJvOVK/cdo8se5VU5vZkCMUttd0WA=\n-----END PUBLIC KEY-----\n"
	malgaSig  = "5b20c43cfd55f0c1884196786d58392e1828232e05d66e9153032ae9d374bff785d0efc4ed850ea5856b71c950bd2a1914f376381386355fce2c51163ed26e01"
	malgaBody = `{"event":"ping","payload":{"object":{}}}`
)

type vector struct {
	name    string
	cfg     Config
	headers map[string]string
	body    string
	now     int64 // Unix seconds the check runs at
}

func vectors() []vector {
	return []vector{
		{"github", Config{Scheme: GitHub, Secret: "It's a Secret to Everybody"},
			map[string]string{"X-Hub-Signature-256": "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"},
			"Hello, World!", 0},
		{"stripe", Config{Scheme: Stripe, Secret: "whsec_test_secret", Tolerance: 300},
			map[string]string{"Stripe-Signature": "t=1792238400,v1=00ff,v1=572d6a6edaee3154a002c24559b8c58ed1207b7e7d6c714ff00d14ded2a1452f,v0=abc"},
			`{"id":"evt_1"}`, 1792238400 + 60},
		{"standard", Config{Scheme: Standard, Secret: "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", Tolerance: 300},
			map[string]string{
				"Webhook-Id": "msg_p5jXN8AQM9LWM0D4loKWxJek", "Webhook-Timestamp": "1614265330",
				"Webhook-Signature": "v1a,ignored v1,AAAA v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=",
			},
			`{"test": 2432232314}`, 1614265330},
		{"svix headers", Config{Scheme: Standard, Secret: "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", Tolerance: 300},
			map[string]string{
				"Svix-Id": "msg_p5jXN8AQM9LWM0D4loKWxJek", "Svix-Timestamp": "1614265330",
				"Svix-Signature": "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=",
			},
			`{"test": 2432232314}`, 1614265330},
		{"shopify", Config{Scheme: Shopify, Secret: "shpss_secret"},
			map[string]string{"X-Shopify-Hmac-Sha256": "xocA3LkeJ/RvNrkxep1EUtFHuj7rIoIqZYaLU26LyPQ="},
			`{"id":1}`, 0},
		{"slack", Config{Scheme: Slack, Secret: "8f742231b10e8888abcd99yyyzzz85a5", Tolerance: 300},
			map[string]string{
				"X-Slack-Signature":         "v0=a2114d57b48eac39b9ad189dd8316235a7b4a8d21a10bd27519666489c69b503",
				"X-Slack-Request-Timestamp": "1531420618",
			},
			slackBody, 1531420618 + 299},
		{"malga", Config{Scheme: Malga, Secret: malgaKey, Tolerance: 300},
			map[string]string{"X-Plug-Date": "1661795163719", "X-Plug-Signature": malgaSig},
			malgaBody, 1661795163},
		{"malga key on one line", Config{Scheme: Malga, Secret: strings.ReplaceAll(malgaKey, "\n", " ")},
			map[string]string{"X-Plug-Date": "1661795163719", "X-Plug-Signature": malgaSig},
			malgaBody, 0},
		{"hmac sha1 base64", Config{Scheme: HMAC, Secret: "s3cr3t", Header: "X-Signature", Algorithm: "sha1", Encoding: "base64"},
			map[string]string{"X-Signature": "AB02dh1Jszc+vHSyB78WTVQVZCg="},
			"payload", 0},
		{"hmac sha512 hex with prefix", Config{Scheme: HMAC, Secret: "s3cr3t", Header: "X-Sig", Algorithm: "sha512", Prefix: "sha512="},
			map[string]string{"X-Sig": "sha512=B52ADA23C1E510612D3D755B310BED210FF06BD15571BE41D298A0D176C97190B6C552CFA694BC57A5AC95E570E2632CDC6E84E6ED9193BDBDE59016E1DAAE6B"},
			"payload", 0},
	}
}

func run(cfg Config, headers map[string]string, body string, now int64) Result {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	k := cfg.Start(h, time.Unix(now, 0))
	// Write in two pieces, as a streamed body would arrive.
	_, _ = k.Write([]byte(body[:len(body)/2]))
	_, _ = k.Write([]byte(body[len(body)/2:]))
	return k.Finish([]byte(body), false, true)
}

func TestValidSignatures(t *testing.T) {
	for _, v := range vectors() {
		if r := run(v.cfg, v.headers, v.body, v.now); r.Status != Valid || r.Scheme != v.cfg.Scheme {
			t.Errorf("%s: %+v", v.name, r)
		}
	}
}

func TestTamperedBodiesAndWrongSecrets(t *testing.T) {
	for _, v := range vectors() {
		if r := run(v.cfg, v.headers, v.body+" ", v.now); r.Status != Invalid {
			t.Errorf("%s, tampered body: %+v", v.name, r)
		}
		wrong := v.cfg
		if wrong.Scheme == Malga {
			wrong.Secret = "MCowBQYDK2VwAyEAIda0HljovhG1yKez/Du7MUoKup/cbXqPgwyGOATOiJQ=" // another key
		} else if wrong.Scheme == Standard {
			wrong.Secret = "whsec_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		} else {
			wrong.Secret += "x"
		}
		if r := run(wrong, v.headers, v.body, v.now); r.Status != Invalid {
			t.Errorf("%s, wrong secret: %+v", v.name, r)
		}
		if r := run(v.cfg, nil, v.body, v.now); r.Status != Missing || r.Detail == "" {
			t.Errorf("%s, no headers: %+v", v.name, r)
		}
	}
}

func TestTimestampTolerance(t *testing.T) {
	for _, v := range vectors() {
		if !Timestamped(v.cfg.Scheme) || v.cfg.Tolerance == 0 {
			continue
		}
		late := run(v.cfg, v.headers, v.body, v.now+int64(v.cfg.Tolerance)+60)
		early := run(v.cfg, v.headers, v.body, v.now-2*int64(v.cfg.Tolerance)-400)
		if late.Status != Expired || !strings.Contains(late.Detail, "old") || early.Status != Expired || !strings.Contains(early.Detail, "future") {
			t.Errorf("%s: late %+v, early %+v", v.name, late, early)
		}
		off := v.cfg
		off.Tolerance = 0
		if r := run(off, v.headers, v.body, v.now+86400*365); r.Status != Valid {
			t.Errorf("%s with tolerance 0: %+v", v.name, r)
		}
	}
}

func TestMalformedHeaders(t *testing.T) {
	for _, tc := range []struct {
		cfg     Config
		headers map[string]string
	}{
		{Config{Scheme: GitHub, Secret: "s"}, map[string]string{"X-Hub-Signature-256": "md5=00"}},
		{Config{Scheme: GitHub, Secret: "s"}, map[string]string{"X-Hub-Signature-256": "sha256=zz"}},
		{Config{Scheme: Stripe, Secret: "s"}, map[string]string{"Stripe-Signature": "v1=00"}},
		{Config{Scheme: Stripe, Secret: "s"}, map[string]string{"Stripe-Signature": "t=1,v0=00"}},
		{Config{Scheme: Standard, Secret: "whsec_AAAA"}, map[string]string{"Webhook-Id": "a", "Webhook-Timestamp": "1", "Webhook-Signature": "v1a,AAAA"}},
		{Config{Scheme: Standard, Secret: "whsec_AAAA"}, map[string]string{"Webhook-Id": "a", "Webhook-Timestamp": "soon", "Webhook-Signature": "v1,AAAA"}},
		{Config{Scheme: Shopify, Secret: "s"}, map[string]string{"X-Shopify-Hmac-Sha256": "!!"}},
		{Config{Scheme: Slack, Secret: "s"}, map[string]string{"X-Slack-Signature": "v1=00", "X-Slack-Request-Timestamp": "1"}},
		{Config{Scheme: Malga, Secret: malgaKey}, map[string]string{"X-Plug-Signature": "abcd", "X-Plug-Date": "1"}},
		{Config{Scheme: Malga, Secret: malgaKey}, map[string]string{"X-Plug-Signature": malgaSig, "X-Plug-Date": "yesterday"}},
		{Config{Scheme: HMAC, Secret: "s", Header: "X-Sig", Prefix: "sha256="}, map[string]string{"X-Sig": "00ff"}},
		{Config{Scheme: HMAC, Secret: "s", Header: "X-Sig"}, map[string]string{"X-Sig": "not hex"}},
	} {
		if r := run(tc.cfg, tc.headers, "body", 1); r.Status != Invalid || r.Detail == "" {
			t.Errorf("%s %v: %+v", tc.cfg.Scheme, tc.headers, r)
		}
	}
}

func TestLargeBodies(t *testing.T) {
	v := vectors()[0]
	h := http.Header{}
	h.Set("X-Hub-Signature-256", v.headers["X-Hub-Signature-256"])
	k := v.cfg.Start(h, time.Now())
	_, _ = k.Write([]byte(v.body))
	// A body cut at MAX_BODY_SIZE still verifies when every byte was streamed.
	if r := k.Finish([]byte("Hello"), true, true); r.Status != Valid {
		t.Errorf("streamed: %+v", r)
	}
	if r := v.cfg.Start(h, time.Now()).Finish(nil, true, false); r.Status != Unverifiable {
		t.Errorf("not streamed: %+v", r)
	}

	m := vectors()[6]
	h = http.Header{}
	for name, value := range m.headers {
		h.Set(name, value)
	}
	if r := m.cfg.Start(h, time.Unix(m.now, 0)).Finish([]byte(m.body[:10]), true, true); r.Status != Unverifiable {
		t.Errorf("malga truncated: %+v", r)
	}
}

func TestValidate(t *testing.T) {
	c := Config{Scheme: HMAC, Secret: "s", Header: "X-Sig", Tolerance: 99}
	if err := c.Validate(); err != nil || c.Algorithm != "sha256" || c.Encoding != "hex" || c.Tolerance != 0 {
		t.Fatalf("hmac defaults: %+v, %v", c, err)
	}
	c = Config{Scheme: GitHub, Secret: "s", Header: "X-Sig", Algorithm: "sha1", Encoding: "base64", Prefix: "p"}
	if err := c.Validate(); err != nil || c.Header != "" || c.Algorithm != "" || c.Encoding != "" || c.Prefix != "" {
		t.Fatalf("github clears hmac options: %+v, %v", c, err)
	}
	for _, bad := range []Config{
		{Scheme: "md5", Secret: "s"},
		{Scheme: GitHub},
		{Scheme: Malga},
		{Scheme: GitHub, Secret: strings.Repeat("s", maxSecret+1)},
		{Scheme: Stripe, Secret: "s", Tolerance: -1},
		{Scheme: Stripe, Secret: "s", Tolerance: MaxTolerance + 1},
		{Scheme: Standard, Secret: "whsec_!!!"},
		{Scheme: Malga, Secret: "not a key"},
		{Scheme: HMAC, Secret: "s"},
		{Scheme: HMAC, Secret: "s", Header: "bad header"},
		{Scheme: HMAC, Secret: "s", Header: "X-Sig", Algorithm: "md5"},
		{Scheme: HMAC, Secret: "s", Header: "X-Sig", Encoding: "base32"},
		{Scheme: HMAC, Secret: "s", Header: "X-Sig", Prefix: "a\nb"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestParsePublicKey(t *testing.T) {
	want, err := ParsePublicKey(malgaKey)
	if err != nil {
		t.Fatal(err)
	}
	// A P-256 key is a valid public key, but not an Ed25519 one.
	p256 := "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAErPRdZidZGiCd8ER7fiBFbkFjh65N/KYvhTPzkxwiK4BlxcB1xs6vTX9KtaVKJz8961qTC8ptr9Em5U5+yEbfNg=="
	if _, err := ParsePublicKey(p256); err == nil || !strings.Contains(err.Error(), "not an Ed25519") {
		t.Errorf("P-256 key: %v", err)
	}
	for _, s := range []string{
		"MCowBQYDK2VwAyEAS8O+o7Cqd3mtpriJvOVK/cdo8se5VU5vZkCMUttd0WA=",     // DER, base64
		"4bc3bea3b0aa7779ada6b889bce54afdc768f2c7b9554e6f66408c52db5dd160", // raw, hex
		"S8O+o7Cqd3mtpriJvOVK/cdo8se5VU5vZkCMUttd0WA=",                     // raw, base64
	} {
		got, err := ParsePublicKey(s)
		if err != nil || !got.Equal(want) {
			t.Errorf("ParsePublicKey(%q) = %x, %v", s, got, err)
		}
	}
}
