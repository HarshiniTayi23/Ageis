package store

// S3 is an ObjectStore backed by any S3-compatible service (AWS S3, MinIO)
// whose bucket was created with Object Lock enabled.
//
// How each guarantee in the ObjectStore contract is met:
//
//	Put never overwrites   -> conditional write: If-None-Match: *  (412 = exists)
//	Nothing deleted early  -> every Put carries a COMPLIANCE retain-until date,
//	                          on top of the bucket's default retention. In
//	                          COMPLIANCE mode nobody, including the account
//	                          root user, can shorten it or delete the version.
//	Delete during retention-> resolves the current version id and deletes
//	                          that exact version; the service refuses.
//
// Caveat (documented in ADR-0006): on a versioned bucket a caller with write
// access can still add a "delete marker" that HIDES the latest version from
// plain GETs. The data itself survives and can be recovered by version id.
// Defending against that (a recovery path that lists versions) is roadmap work.

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // G501: Content-MD5 is an integrity header S3 requires with Object Lock, not a security primitive
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// S3Config configures an S3 store.
type S3Config struct {
	Endpoint  string // e.g. http://localhost:9000 or https://s3.us-east-1.amazonaws.com
	Region    string // default us-east-1
	Bucket    string
	Creds     Credentials
	PathStyle bool          // true for MinIO; false = virtual-hosted (bucket.host)
	Retention time.Duration // COMPLIANCE retention applied to each new object; 0 = bucket default only
	Client    *http.Client
}

// S3 implements ObjectStore.
type S3 struct {
	cfg  S3Config
	base *url.URL
	now  func() time.Time
}

// NewS3 validates cfg and returns a store.
func NewS3(cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" || cfg.Endpoint == "" {
		return nil, errors.New("store: s3 needs endpoint and bucket")
	}
	if cfg.Creds.AccessKey == "" || cfg.Creds.SecretKey == "" {
		return nil, errors.New("store: s3 needs credentials")
	}
	u, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("store: bad s3 endpoint %q", cfg.Endpoint)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 60 * time.Second}
	}
	return &S3{cfg: cfg, base: u, now: time.Now}, nil
}

func (s *S3) objectURL(key string, query url.Values) *url.URL {
	u := *s.base
	if s.cfg.PathStyle {
		u.Path = "/" + s.cfg.Bucket + "/" + key
	} else {
		u.Host = s.cfg.Bucket + "." + u.Host
		u.Path = "/" + key
	}
	u.RawPath = encodePath(u.Path)
	u.RawQuery = query.Encode()
	return &u
}

// do signs and sends a request, retrying transport errors and 5xx responses
// with exponential backoff. The caller must close the response body.
func (s *S3) do(ctx context.Context, method string, u *url.URL, body []byte, hdr http.Header) (*http.Response, error) {
	payload := emptySHA256Hex
	if body != nil {
		payload = hexSHA256(body)
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(100<<attempt) * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if body == nil {
			req.Body, req.ContentLength = http.NoBody, 0
		}
		for k, v := range hdr {
			req.Header[k] = v
		}
		signV4(req, s.cfg.Creds, s.cfg.Region, "s3", payload, s.now())
		resp, err := s.cfg.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = readS3Error(resp)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("store: s3 %s %s: %w", method, u.Path, lastErr)
}

// Put writes a new object, failing with ErrExists if the key exists.
func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	sum := md5.Sum(data) //nolint:gosec // see import comment
	h := http.Header{}
	h.Set("Content-Md5", base64.StdEncoding.EncodeToString(sum[:]))
	h.Set("Content-Type", "application/octet-stream")
	h.Set("If-None-Match", "*")
	if s.cfg.Retention > 0 {
		h.Set("X-Amz-Object-Lock-Mode", "COMPLIANCE")
		h.Set("X-Amz-Object-Lock-Retain-Until-Date", s.now().Add(s.cfg.Retention).UTC().Format(time.RFC3339))
	}
	resp, err := s.do(ctx, http.MethodPut, s.objectURL(key, nil), data, h)
	if err != nil {
		return err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusPreconditionFailed, http.StatusConflict:
		return ErrExists
	}
	return readS3Error(resp)
}

// Get returns an object's current version.
func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, s.objectURL(key, nil), nil, nil)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(resp.Body)
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	return nil, readS3Error(resp)
}

// Exists reports whether key has a current version.
func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.head(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (s *S3) head(ctx context.Context, key string) (http.Header, error) {
	resp, err := s.do(ctx, http.MethodHead, s.objectURL(key, nil), nil, nil)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Header, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("store: s3 HEAD %s: status %d", key, resp.StatusCode)
}

type listResult struct {
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

// List returns all keys with the prefix, following pagination.
func (s *S3) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := s.do(ctx, http.MethodGet, s.objectURL("", q), nil, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			err := readS3Error(resp)
			drain(resp)
			return nil, err
		}
		var lr listResult
		err = xml.NewDecoder(resp.Body).Decode(&lr)
		drain(resp)
		if err != nil {
			return nil, fmt.Errorf("store: s3 list: %w", err)
		}
		for _, c := range lr.Contents {
			keys = append(keys, c.Key)
		}
		if !lr.IsTruncated || lr.NextContinuationToken == "" {
			return keys, nil
		}
		token = lr.NextContinuationToken
	}
}

// Delete permanently deletes the current version of key. Under Object Lock
// retention the service refuses, and Delete returns ErrImmutable.
func (s *S3) Delete(ctx context.Context, key string) error {
	h, err := s.head(ctx, key)
	if err != nil {
		return err
	}
	q := url.Values{}
	if v := h.Get("X-Amz-Version-Id"); v != "" && v != "null" {
		q.Set("versionId", v)
	}
	resp, err := s.do(ctx, http.MethodDelete, s.objectURL(key, q), nil, nil)
	if err != nil {
		return err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusForbidden, http.StatusBadRequest:
		// AWS: 403 AccessDenied. MinIO: 400 "Object is WORM protected".
		return fmt.Errorf("%w: %w", ErrImmutable, readS3Error(resp))
	}
	return readS3Error(resp)
}

type s3Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func readS3Error(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var e s3Error
	if xml.Unmarshal(b, &e) == nil && e.Code != "" {
		return fmt.Errorf("store: s3 status %d: %s: %s", resp.StatusCode, e.Code, e.Message)
	}
	return fmt.Errorf("store: s3 status %d", resp.StatusCode)
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}
