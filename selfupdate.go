package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	selfUpdateRepo = "juitde/claudius-maximus" // matches install.sh's REPO
	githubBaseURL  = "https://github.com"
)

func cliSelfUpdate(svc *Service, args []string) int {
	fs := newFlagSet("self-update")
	targetVersion := fs.String("version", "", "version to install (default: the latest release)")
	force := fs.Bool("force", false, "reinstall even if already on the target version")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	// Check first, so a network round trip is never spent only to then fail
	// on a check that costs nothing - same reasoning as cliInstall's own
	// selfPath() call.
	self, err := selfPath()
	if err != nil {
		return fail(err)
	}

	client := &http.Client{Timeout: 60 * time.Second}

	tag := strings.TrimSpace(*targetVersion)
	if tag == "" {
		tag, err = resolveLatestTag(client, githubBaseURL, selfUpdateRepo)
		if err != nil {
			return fail(fmt.Errorf("determine the latest release: %w", err))
		}
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}

	if !*force && strings.TrimPrefix(tag, "v") == strings.TrimPrefix(version, "v") {
		fmt.Printf("Already on %s\n", tag)
		return exitOK
	}

	archive := releaseArchiveName(runtime.GOOS, runtime.GOARCH)
	archiveData, err := httpGetOK(client, releaseAssetURL(githubBaseURL, selfUpdateRepo, tag, archive))
	if err != nil {
		return fail(fmt.Errorf("download %s - does %s exist for %s/%s? %w",
			archive, tag, runtime.GOOS, runtime.GOARCH, err))
	}
	checksumsData, err := httpGetOK(client, releaseAssetURL(githubBaseURL, selfUpdateRepo, tag, "checksums.txt"))
	if err != nil {
		return fail(fmt.Errorf("download checksums.txt: %w", err))
	}

	if err := verifyChecksum(archiveData, checksumsData, archive); err != nil {
		return fail(err)
	}

	binData, err := extractBinary(archiveData, archive, binaryName(runtime.GOOS))
	if err != nil {
		return fail(err)
	}

	perm := os.FileMode(0o755)
	if info, err := os.Stat(self); err == nil {
		perm = info.Mode().Perm()
	}
	if err := installBinary(self, binData, perm); err != nil {
		return fail(fmt.Errorf("install the new binary: %w", err))
	}

	fmt.Printf("Updated %s from %s to %s\n", appName, version, tag)
	fmt.Println("If claudius-maximus mcp is currently running as a registered " +
		"server, restart it (or your Claude Code session) to use the new version.")
	return exitOK
}

// binaryName is the name of the binary entry inside a release archive -
// matches .goreleaser.yaml's binary: "{{ .ProjectName }}" plus the .exe
// suffix GoReleaser adds automatically for windows builds.
func binaryName(goos string) string {
	if goos == "windows" {
		return appName + ".exe"
	}
	return appName
}

// releaseArchiveName returns the release asset name for goos/goarch, e.g.
// "claudius-maximus_darwin_arm64.tar.gz" or
// "claudius-maximus_windows_amd64.zip" - matches .goreleaser.yaml's
// name_template and format_overrides exactly.
func releaseArchiveName(goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s.%s", appName, goos, goarch, ext)
}

func releaseAssetURL(baseURL, repo, tag, asset string) string {
	return fmt.Sprintf("%s/%s/releases/download/%s/%s", baseURL, repo, tag, asset)
}

// resolveLatestTag follows the one redirect https://github.com/<repo>/releases/latest
// issues - the same one install.sh's -w '%{url_effective}' resolves - without
// fetching the release page's HTML body, by stopping at the first response
// instead of letting the client follow it.
func resolveLatestTag(client *http.Client, baseURL, repo string) (string, error) {
	url := fmt.Sprintf("%s/%s/releases/latest", baseURL, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("reach %s: %w", url, err)
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("%s did not redirect (status %s)", url, resp.Status)
	}
	// Resolved against the request URL rather than assumed absolute - a
	// Location header is not contractually guaranteed to be one.
	resolved, err := resp.Request.URL.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("parse redirect location %q: %w", loc, err)
	}

	const marker = "/releases/tag/"
	idx := strings.Index(resolved.Path, marker)
	if idx == -1 || idx+len(marker) == len(resolved.Path) {
		return "", fmt.Errorf("could not find a release tag in redirect target %s", resolved)
	}
	return resolved.Path[idx+len(marker):], nil
}

// httpGetOK is a plain GET that treats any non-200 status as an error - a
// 404 (a bad --version, or an excluded platform like windows/arm64) must
// produce a clear message here, not a checksum failure against a
// downloaded HTML error page.
func httpGetOK(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", url, err)
	}
	return data, nil
}

// verifyChecksum checks archiveData's sha256 against the entry for
// archiveName in checksumsData (GoReleaser's checksums.txt format:
// "<sha256>  <filename>", one per line). Matches by exact filename, not
// substring/regex - a filename sharing a prefix with archiveName (e.g. a
// future .sig/SBOM entry) must never match.
func verifyChecksum(archiveData, checksumsData []byte, archiveName string) error {
	var expected string
	for _, line := range strings.Split(string(checksumsData), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum entry found for %s in checksums.txt", archiveName)
	}

	sum := sha256.Sum256(archiveData)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s (expected %s, got %s) - refusing to install",
			archiveName, expected, actual)
	}
	return nil
}

// extractBinary finds binName by exact entry name inside archiveData -
// release archives also carry LICENSE and README.md (GoReleaser's default
// archive file set, confirmed against .goreleaser.yaml and a real
// downloaded archive), so "first" or "only" entry would be wrong. There is
// no path-traversal surface either way: the destination the caller writes
// this to is always its own choice, never a path taken from the archive.
func extractBinary(archiveData []byte, archiveName, binName string) ([]byte, error) {
	if strings.HasSuffix(archiveName, ".zip") {
		return extractFromZip(archiveData, binName)
	}
	return extractFromTarGz(archiveData, binName)
}

func extractFromTarGz(data []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		if hdr.Name == binName {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binName)
}

func extractFromZip(data []byte, binName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip archive: %w", err)
	}
	for _, f := range zr.File {
		if f.Name == binName {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s in archive: %w", binName, err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binName)
}
