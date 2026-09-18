package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func testConfig(endpoint string) Config {
	return Config{
		Type:         TypeS3,
		Endpoint:     endpoint,
		Region:       "cn-east-1",
		Bucket:       "orionx",
		AccessKey:    "AK",
		SecretKey:    "SK",
		UsePathStyle: boolPtr(true),
	}
}

// TestPutRequestShape 固定住与七牛 Kodo 兼容性相关的请求形态：
// path-style 寻址、真实 payload 哈希、不带 CRC32 校验和与 aws-chunked。
func TestPutRequestShape(t *testing.T) {
	var (
		gotMethod, gotPath, gotAuth string
		gotHeaders                  http.Header
		gotBody                     []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	backend, err := New(context.Background(), testConfig(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	payload := []byte("hello world")
	if err := backend.Put(context.Background(), "image/u/a1.png", bytes.NewReader(payload), PutOptions{
		ContentType: "image/png",
		Size:        int64(len(payload)),
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Fatalf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/orionx/image/u/a1.png" {
		t.Fatalf("path = %s, want path-style /orionx/image/u/a1.png", gotPath)
	}
	if ct := gotHeaders.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q", ct)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(payload))
	if hash := gotHeaders.Get("X-Amz-Content-Sha256"); hash != wantHash {
		t.Fatalf("x-amz-content-sha256 = %q, want %q", hash, wantHash)
	}
	for key := range gotHeaders {
		if strings.HasPrefix(strings.ToLower(key), "x-amz-checksum-") {
			t.Fatalf("unexpected checksum header %s (Kodo would reject it)", key)
		}
	}
	if enc := gotHeaders.Get("Content-Encoding"); enc != "" {
		t.Fatalf("content-encoding = %q, want empty (no aws-chunked)", enc)
	}
	if !strings.Contains(gotAuth, "cn-east-1/s3/aws4_request") {
		t.Fatalf("authorization = %q, want SigV4 scope for cn-east-1", gotAuth)
	}
	if !bytes.Equal(gotBody, payload) {
		t.Fatalf("body = %q, want %q", gotBody, payload)
	}
}

func TestGetOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "file body")
	}))
	defer srv.Close()

	backend, err := New(context.Background(), testConfig(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	rc, err := backend.Get(context.Background(), "kb_document/u/a1.md")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "file body" {
		t.Fatalf("body = %q", data)
	}
}

func TestGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
	}))
	defer srv.Close()

	backend, err := New(context.Background(), testConfig(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := backend.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteObject(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	backend, err := New(context.Background(), testConfig(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := backend.Delete(context.Background(), "image/u/a1.png"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/orionx/image/u/a1.png" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
}

func TestPing(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	backend, err := New(context.Background(), testConfig(srv.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := backend.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if gotPath != "/orionx" {
		t.Fatalf("path = %s, want /orionx", gotPath)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer failing.Close()
	broken, err := New(context.Background(), testConfig(failing.URL))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := broken.Ping(context.Background()); err == nil {
		t.Fatal("want error when bucket is not accessible")
	}
}

// TestPresignGet 预签名是本地计算，不需要可用的服务端。
func TestPresignGet(t *testing.T) {
	backend, err := New(context.Background(), testConfig("https://s3.cn-east-1.qiniucs.com"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	raw, err := backend.PresignGet(context.Background(), "image/u/a1.png", time.Hour, PresignOptions{})
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host != "s3.cn-east-1.qiniucs.com" || !strings.HasPrefix(u.Path, "/orionx/image/u/a1.png") {
		t.Fatalf("presigned url = %s", raw)
	}
	q := u.Query()
	if q.Get("X-Amz-Signature") == "" || q.Get("X-Amz-Expires") != "3600" {
		t.Fatalf("query = %v", q)
	}
	if q.Get("response-content-disposition") != "" {
		t.Fatal("preview url must not force a download")
	}

	ascii, err := backend.PresignGet(context.Background(), "image/u/a1.png", time.Hour, PresignOptions{DownloadFileName: "logo.png"})
	if err != nil {
		t.Fatalf("presign download: %v", err)
	}
	if got := mustParse(t, ascii).Query().Get("response-content-disposition"); got != `attachment; filename="logo.png"` {
		t.Fatalf("disposition = %q", got)
	}

	utf8, err := backend.PresignGet(context.Background(), "image/u/a1.png", time.Hour, PresignOptions{DownloadFileName: "报表 2026.png"})
	if err != nil {
		t.Fatalf("presign download: %v", err)
	}
	got := mustParse(t, utf8).Query().Get("response-content-disposition")
	if !strings.HasPrefix(got, "attachment; filename*=UTF-8''") || !strings.Contains(got, "%E6%8A%A5") {
		t.Fatalf("disposition = %q", got)
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func TestAttachmentDisposition(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "logo.png", want: `attachment; filename="logo.png"`},
		{in: `evil"name.png`, want: `attachment; filename="evilname.png"`},
		{in: "a\r\nX-Injected: 1.png", want: `attachment; filename="aX-Injected: 1.png"`},
		{in: "", want: "attachment"},
	}
	for _, tc := range cases {
		if got := attachmentDisposition(tc.in); got != tc.want {
			t.Fatalf("attachmentDisposition(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestConfigWithDefaults(t *testing.T) {
	cfg := Config{Endpoint: "https://x.test/", Prefix: "/dev/"}.WithDefaults()
	if cfg.Endpoint != "https://x.test" || cfg.Prefix != "dev" {
		t.Fatalf("normalize: endpoint=%q prefix=%q", cfg.Endpoint, cfg.Prefix)
	}
	if cfg.PresignTTL != defaultPresignTTL || cfg.MaxUploadSize != defaultMaxUploadSize {
		t.Fatalf("defaults: ttl=%v max=%d", cfg.PresignTTL, cfg.MaxUploadSize)
	}
	if !cfg.PathStyle() {
		t.Fatal("path style must default to true")
	}
	virtualHost := Config{UsePathStyle: boolPtr(false)}
	if virtualHost.PathStyle() {
		t.Fatal("explicit use_path_style=false must win")
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "missing type", cfg: Config{Endpoint: "https://x.test"}},
		{name: "unsupported type", cfg: Config{Type: "oss"}},
		{name: "missing fields", cfg: Config{Type: TypeS3, Endpoint: "https://x.test"}},
		{name: "bad endpoint", cfg: Config{Type: TypeS3, Endpoint: "x.test", Region: "cn-east-1", Bucket: "b", AccessKey: "a", SecretKey: "s"}},
		{name: "non http endpoint", cfg: Config{Type: TypeS3, Endpoint: "ftp://x.test", Region: "cn-east-1", Bucket: "b", AccessKey: "a", SecretKey: "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(ctx, tc.cfg); err == nil {
				t.Fatal("want error")
			}
		})
	}
}
