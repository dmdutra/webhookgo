// Package signature checks the signatures webhook providers put on their
// requests, so captured requests can be marked as authentic or not.
//
// A Check is started from the request headers before the body is read, and
// the body is written to it as it streams in: HMAC schemes then cover the
// whole body even when only the first MAX_BODY_SIZE bytes are stored.
package signature

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Schemes.
const (
	GitHub   = "github"   // X-Hub-Signature-256: sha256=hex(HMAC-SHA256(body))
	Stripe   = "stripe"   // Stripe-Signature: t=<ts>,v1=hex(HMAC-SHA256("<ts>.<body>"))
	Standard = "standard" // Standard Webhooks / Svix: webhook-signature: v1,base64(HMAC-SHA256("<id>.<ts>.<body>"))
	Shopify  = "shopify"  // X-Shopify-Hmac-Sha256: base64(HMAC-SHA256(body))
	Slack    = "slack"    // X-Slack-Signature: v0=hex(HMAC-SHA256("v0:<ts>:<body>"))
	Malga    = "malga"    // Malga webhooks v1.1: X-Plug-Signature: hex(Ed25519("<X-Plug-Date>\n<body>"))
	HMAC     = "hmac"     // any header carrying an HMAC of the body
)

const (
	DefaultTolerance = 300
	MaxTolerance     = 7 * 24 * 3600
	maxSecret        = 8192
	maxPrefix        = 64
)

// Config tells how a URL verifies the requests it receives.
type Config struct {
	Scheme    string `json:"scheme"`
	Secret    string `json:"secret,omitempty"`    // HMAC secret, or the public key for malga
	Header    string `json:"header,omitempty"`    // hmac: header carrying the signature
	Algorithm string `json:"algorithm,omitempty"` // hmac: sha1, sha256 or sha512
	Encoding  string `json:"encoding,omitempty"`  // hmac: hex or base64
	Prefix    string `json:"prefix,omitempty"`    // hmac: text before the signature, e.g. "sha256="
	Tolerance int    `json:"tolerance"`           // seconds a signed timestamp may be off (0 = not checked)
	Reject    bool   `json:"reject"`              // answer 401 when a request does not verify
}

// Timestamped reports whether the scheme signs a timestamp, so Tolerance
// applies.
func Timestamped(scheme string) bool {
	return scheme == Stripe || scheme == Standard || scheme == Slack || scheme == Malga
}

// Validate checks c and fills in the defaults of the hmac options, which are
// cleared for the other schemes.
func (c *Config) Validate() error {
	switch c.Scheme {
	case GitHub, Stripe, Standard, Shopify, Slack, Malga:
		c.Header, c.Algorithm, c.Encoding, c.Prefix = "", "", "", ""
	case HMAC:
		if !validHeaderName(c.Header) {
			return errors.New("signature header must be a valid header name")
		}
		if c.Algorithm == "" {
			c.Algorithm = "sha256"
		}
		if hashFor(c.Algorithm) == nil {
			return errors.New("signature algorithm must be sha1, sha256 or sha512")
		}
		if c.Encoding == "" {
			c.Encoding = "hex"
		}
		if c.Encoding != "hex" && c.Encoding != "base64" {
			return errors.New("signature encoding must be hex or base64")
		}
		if len(c.Prefix) > maxPrefix || strings.ContainsAny(c.Prefix, "\r\n\x00") {
			return fmt.Errorf("signature prefix must be at most %d characters on one line", maxPrefix)
		}
	default:
		return fmt.Errorf("unknown signature scheme %q", c.Scheme)
	}
	if !Timestamped(c.Scheme) {
		c.Tolerance = 0
	} else if c.Tolerance < 0 || c.Tolerance > MaxTolerance {
		return fmt.Errorf("signature tolerance must be between 0 and %d seconds", MaxTolerance)
	}

	switch {
	case c.Secret == "" && c.Scheme == Malga:
		return errors.New("signature public key is required")
	case c.Secret == "":
		return errors.New("signature secret is required")
	case len(c.Secret) > maxSecret:
		return fmt.Errorf("signature secret must be at most %d bytes", maxSecret)
	case c.Scheme == Standard:
		if _, err := standardKey(c.Secret); err != nil {
			return err
		}
	case c.Scheme == Malga:
		if _, err := ParsePublicKey(c.Secret); err != nil {
			return err
		}
	}
	return nil
}

// Status is the outcome of a check.
type Status string

const (
	Valid        Status = "valid"
	Invalid      Status = "invalid"      // the signature does not match, or is malformed
	Missing      Status = "missing"      // the request carries no signature
	Expired      Status = "expired"      // the signature matches but its timestamp is outside the tolerance
	Unverifiable Status = "unverifiable" // the body is too large to check
)

// Result is stored with each captured request.
type Result struct {
	Scheme string `json:"scheme"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Check verifies one request. Write the body to it, then call Finish.
type Check struct {
	cfg    *Config
	now    time.Time
	result *Result // decided from the headers alone

	mac  hash.Hash // HMAC schemes: fed the signed prefix, then the body
	sigs [][]byte  // any of these may match

	key    ed25519.PublicKey // malga: Ed25519 needs the whole message
	sig    []byte
	prefix string

	ts int64 // signed timestamp in Unix seconds, 0 when the scheme has none
}

// Start reads the signature headers of a request.
func (c *Config) Start(h http.Header, now time.Time) *Check {
	k := &Check{cfg: c, now: now}
	switch c.Scheme {
	case GitHub:
		v := h.Get("X-Hub-Signature-256")
		if v == "" {
			return k.done(Missing, "no X-Hub-Signature-256 header")
		}
		hexSig, ok := strings.CutPrefix(v, "sha256=")
		sig, err := hex.DecodeString(hexSig)
		if !ok || err != nil {
			return k.done(Invalid, "X-Hub-Signature-256 is not sha256=<hex>")
		}
		k.hmac(sha256.New, []byte(c.Secret), "", sig)

	case Stripe:
		v := h.Get("Stripe-Signature")
		if v == "" {
			return k.done(Missing, "no Stripe-Signature header")
		}
		var t string
		var sigs [][]byte
		for part := range strings.SplitSeq(v, ",") {
			name, value, _ := strings.Cut(strings.TrimSpace(part), "=")
			switch name {
			case "t":
				t = value
			case "v1":
				if sig, err := hex.DecodeString(value); err == nil {
					sigs = append(sigs, sig)
				}
			}
		}
		if !k.timestamp(t) || len(sigs) == 0 {
			return k.done(Invalid, "Stripe-Signature has no t=<timestamp> or v1=<hex> value")
		}
		k.hmac(sha256.New, []byte(c.Secret), t+".", sigs...)

	case Standard:
		id, t, v := h.Get("Webhook-Id"), h.Get("Webhook-Timestamp"), h.Get("Webhook-Signature")
		if v == "" {
			id, t, v = h.Get("Svix-Id"), h.Get("Svix-Timestamp"), h.Get("Svix-Signature")
		}
		if id == "" || t == "" || v == "" {
			return k.done(Missing, "no webhook-id, webhook-timestamp and webhook-signature headers")
		}
		var sigs [][]byte
		for part := range strings.FieldsSeq(v) {
			if b64, ok := strings.CutPrefix(part, "v1,"); ok {
				if sig, err := decodeBase64(b64); err == nil {
					sigs = append(sigs, sig)
				}
			}
		}
		if len(sigs) == 0 {
			return k.done(Invalid, "webhook-signature has no v1,<base64> signature")
		}
		if !k.timestamp(t) {
			return k.done(Invalid, "webhook-timestamp is not a Unix time")
		}
		key, _ := standardKey(c.Secret)
		k.hmac(sha256.New, key, id+"."+t+".", sigs...)

	case Shopify:
		v := h.Get("X-Shopify-Hmac-Sha256")
		if v == "" {
			return k.done(Missing, "no X-Shopify-Hmac-Sha256 header")
		}
		sig, err := decodeBase64(v)
		if err != nil {
			return k.done(Invalid, "X-Shopify-Hmac-Sha256 is not base64")
		}
		k.hmac(sha256.New, []byte(c.Secret), "", sig)

	case Slack:
		v, t := h.Get("X-Slack-Signature"), h.Get("X-Slack-Request-Timestamp")
		if v == "" || t == "" {
			return k.done(Missing, "no X-Slack-Signature and X-Slack-Request-Timestamp headers")
		}
		hexSig, ok := strings.CutPrefix(v, "v0=")
		sig, err := hex.DecodeString(hexSig)
		if !ok || err != nil {
			return k.done(Invalid, "X-Slack-Signature is not v0=<hex>")
		}
		if !k.timestamp(t) {
			return k.done(Invalid, "X-Slack-Request-Timestamp is not a Unix time")
		}
		k.hmac(sha256.New, []byte(c.Secret), "v0:"+t+":", sig)

	case Malga:
		v, t := h.Get("X-Plug-Signature"), h.Get("X-Plug-Date")
		if v == "" || t == "" {
			return k.done(Missing, "no X-Plug-Signature and X-Plug-Date headers")
		}
		sig, err := hex.DecodeString(v)
		if err != nil || len(sig) != ed25519.SignatureSize {
			return k.done(Invalid, "X-Plug-Signature is not a 128-character hex Ed25519 signature")
		}
		if !k.timestamp(t) {
			return k.done(Invalid, "X-Plug-Date is not a Unix time")
		}
		k.key, _ = ParsePublicKey(c.Secret)
		k.sig, k.prefix = sig, t+"\n"

	case HMAC:
		v := strings.TrimSpace(h.Get(c.Header))
		if v == "" {
			return k.done(Missing, "no "+c.Header+" header")
		}
		raw, ok := strings.CutPrefix(v, c.Prefix)
		if !ok {
			return k.done(Invalid, fmt.Sprintf("%s does not start with %q", c.Header, c.Prefix))
		}
		var sig []byte
		var err error
		if c.Encoding == "base64" {
			sig, err = decodeBase64(raw)
		} else {
			sig, err = hex.DecodeString(raw)
		}
		if err != nil {
			return k.done(Invalid, fmt.Sprintf("%s is not %s", c.Header, c.Encoding))
		}
		k.hmac(hashFor(c.Algorithm), []byte(c.Secret), "", sig)
	}
	return k
}

func (k *Check) done(s Status, detail string) *Check {
	k.result = &Result{Scheme: k.cfg.Scheme, Status: s, Detail: detail}
	return k
}

func (k *Check) hmac(h func() hash.Hash, key []byte, prefix string, sigs ...[]byte) {
	k.mac = hmac.New(h, key)
	k.mac.Write([]byte(prefix))
	k.sigs = sigs
}

// timestamp records a signed Unix time in seconds or milliseconds.
func (k *Check) timestamp(v string) bool {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return false
	}
	if n >= 1e12 {
		n /= 1000
	}
	k.ts = n
	return true
}

// Write feeds the body to the check as it is read.
func (k *Check) Write(p []byte) (int, error) {
	if k.mac != nil && k.result == nil {
		k.mac.Write(p)
	}
	return len(p), nil
}

// Finish returns the outcome. body is the stored body; truncated says it is
// not the whole body, and streamed says every byte went through Write.
func (k *Check) Finish(body []byte, truncated, streamed bool) Result {
	if k.result != nil {
		return *k.result
	}
	fail := func(s Status, detail string) Result { return Result{Scheme: k.cfg.Scheme, Status: s, Detail: detail} }
	switch {
	case k.mac != nil:
		if !streamed {
			return fail(Unverifiable, "the body is too large to verify")
		}
		sum := k.mac.Sum(nil)
		if !slices.ContainsFunc(k.sigs, func(sig []byte) bool { return hmac.Equal(sig, sum) }) {
			return fail(Invalid, "the signature does not match")
		}
	case k.key != nil:
		if truncated {
			return fail(Unverifiable, "the body is larger than MAX_BODY_SIZE, so it cannot be verified")
		}
		if !ed25519.Verify(k.key, []byte(k.prefix+string(body)), k.sig) {
			return fail(Invalid, "the signature does not match")
		}
	default:
		return fail(Unverifiable, "the scheme is not supported")
	}
	if tol := k.cfg.Tolerance; k.ts != 0 && tol > 0 {
		age := k.now.Unix() - k.ts
		switch {
		case age > int64(tol):
			return fail(Expired, fmt.Sprintf("the signed timestamp is %s old, more than the %s tolerance", seconds(age), seconds(int64(tol))))
		case -age > int64(tol):
			return fail(Expired, fmt.Sprintf("the signed timestamp is %s in the future", seconds(-age)))
		}
	}
	return Result{Scheme: k.cfg.Scheme, Status: Valid}
}

func seconds(n int64) string { return (time.Duration(n) * time.Second).String() }

func hashFor(name string) func() hash.Hash {
	switch name {
	case "sha1":
		return sha1.New
	case "sha256":
		return sha256.New
	case "sha512":
		return sha512.New
	}
	return nil
}

// decodeBase64 accepts standard and URL-safe base64, with or without padding.
func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	var err error
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		var b []byte
		if b, err = enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, err
}

// standardKey decodes a Standard Webhooks secret: whsec_ followed by base64.
func standardKey(secret string) ([]byte, error) {
	key, err := decodeBase64(strings.TrimPrefix(secret, "whsec_"))
	if err != nil || len(key) == 0 {
		return nil, errors.New("signature secret must be whsec_ followed by base64")
	}
	return key, nil
}

var pemArmor = regexp.MustCompile(`-----(BEGIN|END) [A-Z ]+-----`)

// ParsePublicKey reads an Ed25519 public key in PEM (line breaks optional,
// since forms often lose them), base64 DER, or as 32 raw bytes in hex or
// base64.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	s = strings.Join(strings.Fields(pemArmor.ReplaceAllString(s, "")), "")
	if b, err := hex.DecodeString(s); err == nil && len(b) == ed25519.PublicKeySize {
		return b, nil
	}
	if b, err := decodeBase64(s); err == nil {
		if len(b) == ed25519.PublicKeySize {
			return b, nil
		}
		if pub, err := x509.ParsePKIXPublicKey(b); err == nil {
			if key, ok := pub.(ed25519.PublicKey); ok {
				return key, nil
			}
			return nil, errors.New("signature public key is not an Ed25519 key")
		}
	}
	return nil, errors.New("signature public key must be an Ed25519 key in PEM, hex or base64")
}

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0 {
			continue
		}
		return false
	}
	return true
}
