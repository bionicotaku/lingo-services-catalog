package gcs_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	gcs "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/gcs"
	"github.com/go-kratos/kratos/v2/log"
)

func TestSignedResumableInitURL(t *testing.T) {
	ctx := context.Background()
	keyPEM, accessID := generateTestKey(t)
	fixed := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	signer, err := gcs.NewResumableSigner(ctx, accessID, log.NewStdLogger(io.Discard),
		gcs.WithServiceAccountKey(accessID, keyPEM),
		gcs.WithClock(func() time.Time { return fixed }),
	)
	if err != nil {
		t.Fatalf("NewResumableSigner: %v", err)
	}

	ttl := 10 * time.Minute
	signedURL, expires, err := signer.SignedResumableInitURL(ctx, "my-bucket", "raw_videos/user/video", "video/mp4", ttl)
	if err != nil {
		t.Fatalf("SignedResumableInitURL: %v", err)
	}
	if !expires.Equal(fixed.Add(ttl)) {
		t.Fatalf("expected expires %v, got %v", fixed.Add(ttl), expires)
	}

	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	if parsed.Host == "" {
		t.Fatal("expected host in signed url")
	}
	if !strings.Contains(parsed.Path, "raw_videos/user/video") {
		t.Fatalf("expected object path in signed url, got %s", parsed.Path)
	}

	query := parsed.Query()
	if query.Get("X-Goog-Expires") == "" {
		t.Fatalf("missing TTL in signed url")
	}
	headers := strings.ToLower(query.Get("X-Goog-SignedHeaders"))
	if !strings.Contains(headers, "x-goog-resumable") {
		t.Fatalf("signed headers missing resumable flag: %s", headers)
	}
	if !strings.Contains(headers, "x-goog-if-generation-match") {
		t.Fatalf("signed headers missing generation match: %s", headers)
	}
}

func generateTestKey(t *testing.T) ([]byte, string) {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}
	pemBytes := pem.EncodeToMemory(block)
	accessID := "test-signer@unit-test.iam.gserviceaccount.com"
	return pemBytes, accessID
}
