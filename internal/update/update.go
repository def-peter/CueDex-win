// Package update finds a newer CueDex release on GitHub and fetches the
// installer it publishes. It is the Windows counterpart of the mac app's
// UpdateController: check, download, verify, then hand the file to the user.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LatestURL is the GitHub endpoint that names the newest published release.
// Drafts and pre-releases are not "latest", which is what the check wants.
const LatestURL = "https://api.github.com/repos/def-peter/CueDex-win/releases/latest"

// PageURL is the download page the panel opens when it cannot install by
// itself.
const PageURL = "https://github.com/def-peter/CueDex-win/releases/latest"

// ErrNoChecksum says the release named an installer but published no digest
// for it. The panel falls back to the release page rather than run a file it
// cannot check.
var ErrNoChecksum = errors.New("update: the release publishes no checksum")

// Release is one published version.
type Release struct {
	// Version is the tag without its leading v, such as "0.3.0".
	Version string
	// Page is the release's page in a browser.
	Page string
	// Installer is the Windows setup attached to the release, empty when the
	// release has none.
	Installer Asset
}

// Asset is one file attached to a release.
type Asset struct {
	Name string
	URL  string
	// Digest is GitHub's "sha256:<hex>" for the file, empty when it published
	// none.
	Digest string
	Size   int64
}

// Fetch reads the newest release. A client of nil uses a 15 second default,
// which keeps a launch from waiting on a network that is not there.
func Fetch(ctx context.Context, client *http.Client, endpoint string) (Release, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if endpoint == "" {
		endpoint = LatestURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "CueDex")

	response, err := client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("update: releases answered %s", response.Status)
	}
	return decode(response.Body)
}

type releaseJSON struct {
	Tag    string `json:"tag_name"`
	Page   string `json:"html_url"`
	Assets []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
		// Digest is "sha256:<hex>", added by GitHub in 2025.
		Digest string `json:"digest"`
	} `json:"assets"`
}

func decode(body io.Reader) (Release, error) {
	var payload releaseJSON
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return Release{}, err
	}
	release := Release{
		Version: Normalize(payload.Tag),
		Page:    payload.Page,
	}
	for _, asset := range payload.Assets {
		if !isSetup(asset.Name) {
			continue
		}
		release.Installer = Asset{
			Name:   asset.Name,
			URL:    asset.URL,
			Digest: asset.Digest,
			Size:   asset.Size,
		}
		break
	}
	return release, nil
}

// isSetup reports whether an asset is the Windows installer, so a release that
// also carries a checksum file or a bare exe still finds its setup.
func isSetup(name string) bool {
	return strings.HasPrefix(name, "cuedex-setup-") && strings.HasSuffix(name, "-amd64.exe")
}

// Normalize strips the tag's v and any pre-release suffix: "v0.3.0" and
// "0.3.0-beta.1" both compare as "0.3.0".
func Normalize(tag string) string {
	version := strings.TrimSpace(tag)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	if i := strings.IndexAny(version, "-+"); i >= 0 {
		version = version[:i]
	}
	return version
}

// IsNewer reports whether candidate names a later version than current. Both
// are dotted numbers; a part that is not a number counts as zero, so a
// malformed tag never claims to be newer than what is running.
func IsNewer(candidate, current string) bool {
	a, b := Normalize(candidate), Normalize(current)
	if a == "" || b == "" {
		return false
	}
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) || i < len(right); i++ {
		x, y := 0, 0
		if i < len(left) {
			x, _ = strconv.Atoi(left[i])
		}
		if i < len(right) {
			y, _ = strconv.Atoi(right[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// Download fetches the installer into dir and checks it against the digest the
// release published. It returns the file's path, ready to run.
func Download(ctx context.Context, client *http.Client, asset Asset, dir string) (string, error) {
	if asset.Digest == "" {
		return "", ErrNoChecksum
	}
	want, ok := strings.CutPrefix(asset.Digest, "sha256:")
	if !ok || len(want) != sha256.Size*2 {
		return "", ErrNoChecksum
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "CueDex")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update: download answered %s", response.Status)
	}

	path := filepath.Join(dir, asset.Name)
	temp := path + ".part"
	file, err := os.Create(temp)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), response.Body); err != nil {
		file.Close()
		os.Remove(temp)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return "", err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, want) {
		os.Remove(temp)
		return "", fmt.Errorf("update: %s does not match its checksum", asset.Name)
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return "", err
	}
	return path, nil
}
