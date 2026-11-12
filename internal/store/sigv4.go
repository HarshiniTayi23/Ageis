package store

// AWS Signature Version 4, implemented with the standard library only.
//
// Every S3 request must prove the caller holds the secret key without sending
// it. SigV4 does that in four steps:
//
//  1. Canonical request: method, path, sorted query, sorted signed headers
//     and the SHA-256 of the body, in one exact textual layout.
//  2. String to sign: algorithm, timestamp, credential scope
//     (date/region/service/aws4_request) and the hash of step 1.
//  3. Signing key: HMAC chain over the secret, date, region and service, so
//     a leaked signing key only works for one day, one region, one service.
//  4. Signature: HMAC(signing key, string to sign), sent in Authorization.
//
// Tests check this file against AWS's published examples.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	sigAlgorithm   = "AWS4-HMAC-SHA256"
	amzDateFormat  = "20060102T150405Z"
	emptySHA256Hex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// Credentials are an access key pair.
type Credentials struct {
	AccessKey string
	SecretKey string
}

// signV4 adds x-amz-date, x-amz-content-sha256 and Authorization headers to
// req. payloadHash is the lowercase hex SHA-256 of the request body.
func signV4(req *http.Request, creds Credentials, region, service, payloadHash string, now time.Time) {
	amzDate := now.UTC().Format(amzDateFormat)
	date := amzDate[:8]
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonical, signed := canonicalRequest(req, payloadHash)
	scope := date + "/" + region + "/" + service + "/aws4_request"
	toSign := sigAlgorithm + "\n" + amzDate + "\n" + scope + "\n" + hexSHA256([]byte(canonical))

	key := signingKey(creds.SecretKey, date, region, service)
	sig := hex.EncodeToString(hmacSHA256(key, []byte(toSign)))
	req.Header.Set("Authorization", sigAlgorithm+
		" Credential="+creds.AccessKey+"/"+scope+
		", SignedHeaders="+signed+
		", Signature="+sig)
}

// canonicalRequest returns the canonical request text and the
// semicolon-separated list of signed header names.
func canonicalRequest(req *http.Request, payloadHash string) (string, string) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers := map[string]string{"host": host}
	for name, vals := range req.Header {
		n := strings.ToLower(name)
		if shouldSign(n) {
			headers[n] = strings.Join(trimAll(vals), ",")
		}
	}
	names := make([]string, 0, len(headers))
	for n := range headers {
		names = append(names, n)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, n := range names {
		ch.WriteString(n + ":" + headers[n] + "\n")
	}
	signed := strings.Join(names, ";")

	path := req.URL.EscapedPath()
	if req.URL.RawPath == "" {
		path = encodePath(req.URL.Path)
	}
	if path == "" {
		path = "/"
	}
	return strings.Join([]string{
		req.Method,
		path,
		canonicalQuery(req.URL.Query()),
		ch.String(),
		signed,
		payloadHash,
	}, "\n"), signed
}

func shouldSign(name string) bool {
	switch name {
	case "host", "content-md5", "content-type", "date", "range", "if-none-match", "if-match":
		return true
	}
	return strings.HasPrefix(name, "x-amz-")
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// encodePath encodes each path segment but keeps the slashes. S3, unlike
// other AWS services, does not double-encode paths.
func encodePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = uriEncode(s, true)
	}
	return strings.Join(segs, "/")
}

// uriEncode implements AWS's URI encoding: every byte except the RFC 3986
// unreserved characters is percent-encoded with uppercase hex.
func uriEncode(s string, encodeSlash bool) string {
	const hexdig = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexdig[c>>4])
			b.WriteByte(hexdig[c&15])
		}
	}
	return b.String()
}

func signingKey(secret, date, region, service string) []byte {
	k := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	k = hmacSHA256(k, []byte(region))
	k = hmacSHA256(k, []byte(service))
	return hmacSHA256(k, []byte("aws4_request"))
}

func hmacSHA256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

func hexSHA256(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func trimAll(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = strings.Join(strings.Fields(v), " ")
	}
	return out
}
