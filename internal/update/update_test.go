package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsNewerComparesDottedNumbers(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"0.3.0", "0.2.1", true},
		{"v0.3.0", "0.2.1", true},
		{"0.2.1", "0.2.1", false},
		{"0.2.0", "0.2.1", false},
		{"0.10.0", "0.9.9", true},
		{"1.0", "0.9.9", true},
		{"0.3.0-beta.1", "0.2.1", true},
		{"", "0.2.1", false},
		{"0.2.1", "", false},
		{"banana", "0.2.1", false},
	}
	for _, tc := range cases {
		if got := IsNewer(tc.candidate, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"v0.3.0":       "0.3.0",
		" 0.3.0 ":      "0.3.0",
		"V0.3.0":       "0.3.0",
		"0.3.0-rc.1":   "0.3.0",
		"0.3.0+build9": "0.3.0",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchPicksTheSetupAndIgnoresOtherAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); !strings.Contains(got, "github+json") {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "CueDex" {
			t.Errorf("User-Agent = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.3.0",
			"html_url": "https://github.com/def-peter/CueDex-win/releases/tag/v0.3.0",
			"assets": []map[string]any{
				{"name": "CueDex.exe", "size": 11, "browser_download_url": "https://example/CueDex.exe"},
				{"name": "SHA256SUMS.txt", "size": 171, "browser_download_url": "https://example/SHA256SUMS.txt"},
				{
					"name":                 "cuedex-setup-0.3.0-amd64.exe",
					"size":                 4460120,
					"browser_download_url": "https://example/setup.exe",
					"digest":               "sha256:abc",
				},
			},
		})
	}))
	defer server.Close()

	release, err := Fetch(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.3.0" || !strings.Contains(release.Page, "v0.3.0") {
		t.Fatalf("release %+v", release)
	}
	if release.Installer.Name != "cuedex-setup-0.3.0-amd64.exe" ||
		release.Installer.URL != "https://example/setup.exe" ||
		release.Installer.Digest != "sha256:abc" {
		t.Fatalf("installer %+v", release.Installer)
	}
}

func TestFetchReportsABadAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	if _, err := Fetch(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("a 403 was accepted")
	}
}

func TestDownloadChecksTheDigestAndWritesTheFile(t *testing.T) {
	body := []byte("installer bytes")
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer server.Close()

	dir := t.TempDir()
	asset := Asset{
		Name:   "cuedex-setup-0.3.0-amd64.exe",
		URL:    server.URL + "/setup.exe",
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
	}
	path, err := Download(context.Background(), server.Client(), asset, dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, asset.Name) {
		t.Fatalf("path %q", path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(body) {
		t.Fatalf("file %q, %v", got, err)
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatalf("the temporary file survived: %v", err)
	}
}

func TestDownloadRefusesAMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tampered"))
	}))
	defer server.Close()

	dir := t.TempDir()
	asset := Asset{
		Name:   "cuedex-setup-0.3.0-amd64.exe",
		URL:    server.URL + "/setup.exe",
		Digest: "sha256:" + strings.Repeat("0", 64),
	}
	if _, err := Download(context.Background(), server.Client(), asset, dir); err == nil {
		t.Fatal("a mismatched download was accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a rejected download left %d files", len(entries))
	}
}

func TestDownloadNeedsAChecksum(t *testing.T) {
	if _, err := Download(context.Background(), nil, Asset{Name: "x.exe"}, t.TempDir()); err != ErrNoChecksum {
		t.Fatalf("err %v", err)
	}
}
