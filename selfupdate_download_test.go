package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReleaseArchiveName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "claudius-maximus_darwin_arm64.tar.gz"},
		{"linux", "amd64", "claudius-maximus_linux_amd64.tar.gz"},
		{"windows", "amd64", "claudius-maximus_windows_amd64.zip"},
	}
	for _, tt := range tests {
		if got := releaseArchiveName(tt.goos, tt.goarch); got != tt.want {
			t.Errorf("releaseArchiveName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestBinaryName(t *testing.T) {
	if got := binaryName("windows"); got != "claudius-maximus.exe" {
		t.Errorf("binaryName(windows) = %q, want claudius-maximus.exe", got)
	}
	if got := binaryName("linux"); got != "claudius-maximus" {
		t.Errorf("binaryName(linux) = %q, want claudius-maximus", got)
	}
}

func TestResolveLatestTag(t *testing.T) {
	t.Run("absolute location", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "http://example.invalid/juitde/claudius-maximus/releases/tag/v0.2.0")
			w.WriteHeader(http.StatusFound)
		}))
		defer srv.Close()

		tag, err := resolveLatestTag(srv.Client(), srv.URL, "juitde/claudius-maximus")
		if err != nil {
			t.Fatalf("resolveLatestTag: %v", err)
		}
		if tag != "v0.2.0" {
			t.Errorf("tag = %q, want v0.2.0", tag)
		}
	})

	t.Run("relative location", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/juitde/claudius-maximus/releases/tag/v0.3.1")
			w.WriteHeader(http.StatusFound)
		}))
		defer srv.Close()

		tag, err := resolveLatestTag(srv.Client(), srv.URL, "juitde/claudius-maximus")
		if err != nil {
			t.Fatalf("resolveLatestTag: %v", err)
		}
		if tag != "v0.3.1" {
			t.Errorf("tag = %q, want v0.3.1", tag)
		}
	})

	t.Run("no redirect", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		if _, err := resolveLatestTag(srv.Client(), srv.URL, "juitde/claudius-maximus"); err == nil {
			t.Fatal("expected an error when the server does not redirect")
		}
	})
}

func TestHTTPGetOK(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("payload"))
		}))
		defer srv.Close()

		data, err := httpGetOK(srv.Client(), srv.URL)
		if err != nil {
			t.Fatalf("httpGetOK: %v", err)
		}
		if string(data) != "payload" {
			t.Errorf("data = %q, want %q", data, "payload")
		}
	})

	t.Run("404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		if _, err := httpGetOK(srv.Client(), srv.URL); err == nil {
			t.Fatal("expected an error for a 404 response")
		}
	})
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("some archive bytes")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	t.Run("match", func(t *testing.T) {
		checksums := []byte(fmt.Sprintf("%s  claudius-maximus_linux_amd64.tar.gz\n", hash))
		if err := verifyChecksum(data, checksums, "claudius-maximus_linux_amd64.tar.gz"); err != nil {
			t.Errorf("verifyChecksum: %v", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		checksums := []byte("0000000000000000000000000000000000000000000000000000000000000000  claudius-maximus_linux_amd64.tar.gz\n")
		if err := verifyChecksum(data, checksums, "claudius-maximus_linux_amd64.tar.gz"); err == nil {
			t.Error("expected a checksum mismatch error")
		}
	})

	t.Run("no entry", func(t *testing.T) {
		checksums := []byte(fmt.Sprintf("%s  claudius-maximus_darwin_arm64.tar.gz\n", hash))
		if err := verifyChecksum(data, checksums, "claudius-maximus_linux_amd64.tar.gz"); err == nil {
			t.Error("expected an error when no entry matches")
		}
	})

	t.Run("does not match a same-prefixed filename", func(t *testing.T) {
		// A regression this project's install.sh actually hit: matching by
		// substring/regex instead of exact filename lets a filename sharing
		// a prefix with the real one (e.g. a future .sig/SBOM entry) match
		// too, and turn a good download into a false checksum mismatch.
		checksums := []byte(fmt.Sprintf(
			"%s  claudius-maximus_linux_amd64.tar.gz.sig\n0000000000000000000000000000000000000000000000000000000000000000  claudius-maximus_linux_amd64.tar.gz\n",
			hash))
		err := verifyChecksum(data, checksums, "claudius-maximus_linux_amd64.tar.gz")
		if err == nil {
			t.Fatal("expected the mismatched real entry to be used, not the same-prefixed decoy")
		}
	})
}

func TestExtractBinary(t *testing.T) {
	t.Run("tar.gz with a decoy entry", func(t *testing.T) {
		archive := buildTarGz(t, map[string][]byte{
			"LICENSE":          []byte("license text"),
			"README.md":        []byte("readme text"),
			"claudius-maximus": []byte("the real binary"),
		})
		data, err := extractBinary(archive, "claudius-maximus_linux_amd64.tar.gz", "claudius-maximus")
		if err != nil {
			t.Fatalf("extractBinary: %v", err)
		}
		if string(data) != "the real binary" {
			t.Errorf("data = %q, want %q", data, "the real binary")
		}
	})

	t.Run("zip with a decoy entry", func(t *testing.T) {
		archive := buildZip(t, map[string][]byte{
			"LICENSE":              []byte("license text"),
			"claudius-maximus.exe": []byte("the real binary"),
		})
		data, err := extractBinary(archive, "claudius-maximus_windows_amd64.zip", "claudius-maximus.exe")
		if err != nil {
			t.Fatalf("extractBinary: %v", err)
		}
		if string(data) != "the real binary" {
			t.Errorf("data = %q, want %q", data, "the real binary")
		}
	})

	t.Run("missing entry", func(t *testing.T) {
		archive := buildTarGz(t, map[string][]byte{"LICENSE": []byte("license text")})
		if _, err := extractBinary(archive, "claudius-maximus_linux_amd64.tar.gz", "claudius-maximus"); err == nil {
			t.Error("expected an error when the binary entry is missing")
		}
	})
}

func buildTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatalf("write tar header for %s: %v", name, err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("write tar content for %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write zip content for %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}
