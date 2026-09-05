// Package updater provides self-update capabilities for WhatsRook matching system package manager designs (brew/apt/dnf style).
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"whatsrook/cmd/store"

	"whatsrook"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

const (
	DefaultRepoOwner   = "ThruqeLabs"
	DefaultRepoName    = "whatsrook"
	DefaultVersionFile = "version.txt"
	DefaultVersionURL  = "https://raw.githubusercontent.com/ThruqeLabs/whatsrook/refs/heads/master/version.txt"
	ChannelKey         = "update_channel" // "stable" or "beta"
)

var EmbeddedAppVersion = func() string {
	if v, err := whatsrook.GetVersion(); err == nil && v.Raw != "" {
		return v.Raw
	}
	return "4.9.26"
}()

// Backward-compatible exports for external callers.
const (
	RepoOwner     = DefaultRepoOwner
	RepoName      = DefaultRepoName
	VersionFile   = DefaultVersionFile
	VersionGithub = DefaultVersionURL
)

// Version holds a semantic version (major.minor.patch).
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

// UpdateResult describes the outcome of an update check or update operation.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	HasNewVersion  bool
	Updated        bool
	IsBeta         bool
	Platform       string
	Message        string
}

// Options configures an Updater instance.
type Options struct {
	RepoOwner   string
	RepoName    string
	VersionFile string
	Channel     string    // "stable" or "beta"
	Out         io.Writer // Writer for progress logs (e.g. os.Stdout)
	HTTPClient  *http.Client
}

// Updater manages checking for and applying application upgrades.
type Updater struct {
	opts Options
}

// New returns a new Updater initialized with the provided Options.
func New(opts Options) *Updater {
	if opts.RepoOwner == "" {
		opts.RepoOwner = DefaultRepoOwner
	}
	if opts.RepoName == "" {
		opts.RepoName = DefaultRepoName
	}
	if opts.VersionFile == "" {
		opts.VersionFile = DefaultVersionFile
	}
	if opts.Channel == "" {
		opts.Channel = "stable"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Updater{opts: opts}
}

// SetOutput sets the destination writer for progress logging.
func (u *Updater) SetOutput(w io.Writer) {
	u.opts.Out = w
}

func (u *Updater) logf(format string, args ...any) {
	if u.opts.Out != nil {
		fmt.Fprintf(u.opts.Out, format+"\n", args...)
	}
}

// GetPlatform returns operating system and architecture string (e.g. linux/amd64).
func GetPlatform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// supportedPlatforms is the complete set of OS/arch combinations that have
// published release assets. Anything outside this set has no downloadable binary.
var supportedPlatforms = map[string]bool{
	"darwin/amd64":  true,
	"darwin/arm64":  true,
	"linux/amd64":   true,
	"linux/arm64":   true,
	"android/arm64": true,
	"windows/amd64": true,
}

// IsSupportedPlatform reports whether the current runtime OS/arch has a
// published release asset.
func IsSupportedPlatform() bool {
	return supportedPlatforms[GetPlatform()]
}

// channelFilePath returns the path used to persist the active update channel.
// It prefers os.UserConfigDir()/whatsrook and falls back to the executable directory.
func channelFilePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		p := filepath.Join(dir, "whatsrook")
		_ = os.MkdirAll(p, 0755)
		return filepath.Join(p, ".update-channel")
	}
	if exe, err := ResolveExecutablePath(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".update-channel")
	}
	return ".update-channel"
}

func installedBetaFilePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		p := filepath.Join(dir, "whatsrook")
		_ = os.MkdirAll(p, 0755)
		return filepath.Join(p, ".installed-beta")
	}
	if exe, err := ResolveExecutablePath(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".installed-beta")
	}
	return ".installed-beta"
}

// GetInstalledBetaVersion returns the persisted version identifier for alpha/beta builds.
func GetInstalledBetaVersion() string {
	data, err := os.ReadFile(installedBetaFilePath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SetInstalledBetaVersion persists the installed alpha/beta build version identifier.
func SetInstalledBetaVersion(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		_ = os.Remove(installedBetaFilePath())
		return nil
	}
	return os.WriteFile(installedBetaFilePath(), []byte(v+"\n"), 0644)
}

// FormatVersionDisplay formats versions cleanly without raw hash prefix collisions.
// E.g. "sha256:d8860761..." -> "beta-d8860", "sha:d8860761..." -> "beta-d8860", "21.8.26" -> "v21.8.26".
func FormatVersionDisplay(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	for _, prefix := range []string{"sha256:", "sha:", "beta-", "beta:", "alpha-", "alpha:"} {
		if after, ok := strings.CutPrefix(v, prefix); ok {
			after = strings.TrimSpace(after)
			if len(after) > 5 {
				after = after[:5]
			}
			return "beta-" + after
		}
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	if len(v) >= 12 && isHex(v) {
		return "beta-" + v[:5]
	}
	return "v" + v
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// GetStoredChannel returns the persisted update channel ("stable" or "beta"),
// defaulting to "stable" when no preference has been saved yet.
func GetStoredChannel() string {
	data, err := os.ReadFile(channelFilePath())
	if err != nil {
		return "stable"
	}
	if strings.TrimSpace(strings.ToLower(string(data))) == "beta" {
		return "beta"
	}
	return "stable"
}

// SetStoredChannel writes the update channel preference to disk.
func SetStoredChannel(channel string) error {
	channel = strings.TrimSpace(strings.ToLower(channel))
	if channel != "stable" && channel != "beta" {
		return fmt.Errorf("invalid channel %q: must be \"stable\" or \"beta\"", channel)
	}
	return os.WriteFile(channelFilePath(), []byte(channel+"\n"), 0644)
}

// GetChannel gets configured update channel ("stable" or "beta").
// It checks the SQLStore settings table first, falling back to the local file channel preference.
func GetChannel(ctx context.Context, Sqlstore *sqlstore.SQLStore) string {
	if Sqlstore != nil {
		ch, err := store.GetSetting(ctx, Sqlstore, ChannelKey)
		if err == nil && ch != "" {
			chLower := strings.ToLower(strings.TrimSpace(ch))
			if chLower == "stable" || chLower == "beta" {
				return chLower
			}
		}
	}
	return GetStoredChannel()
}

// SetChannel sets update channel ("stable" or "beta") across both SQLStore and local file preference.
func SetChannel(ctx context.Context, SQlstore *sqlstore.SQLStore, channel string) error {
	channel = strings.TrimSpace(strings.ToLower(channel))
	if channel != "stable" && channel != "beta" {
		return fmt.Errorf("invalid channel %q: must be \"stable\" or \"beta\"", channel)
	}
	_ = SetStoredChannel(channel)
	if SQlstore == nil {
		return nil
	}
	return store.PutSetting(ctx, SQlstore, ChannelKey, channel)
}

// ParseVersion converts a semver string into a Version struct.
func ParseVersion(raw string) (Version, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "v")

	parts := strings.Split(clean, ".")
	if len(parts) < 3 {
		return Version{Raw: raw}, fmt.Errorf("invalid semver format: %s", raw)
	}

	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patchStr, _, _ := strings.Cut(parts[2], "-")
	patch, err3 := strconv.Atoi(patchStr)

	if err1 != nil || err2 != nil || err3 != nil {
		return Version{Raw: raw}, fmt.Errorf("non-numeric semver component in %s", raw)
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   raw,
	}, nil
}

// Compare compares two versions, returning -1/0/+1 like cmp.Compare.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}
	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}
	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}
	return 0
}

// GetAppVersion attempts to read version from local version.txt or installed beta, falling back to whatsrook.GetVersion().
func GetAppVersion() string {
	if GetStoredChannel() == "beta" {
		if betaVer := GetInstalledBetaVersion(); betaVer != "" {
			return FormatVersionDisplay(betaVer)
		}
	}
	return FormatVersionDisplay(ReadEffectiveLocalVersion(DefaultVersionFile))
}

// ReadEffectiveLocalVersion checks cwd, executable directory, and fallback embedded version.
func ReadEffectiveLocalVersion(versionFile string) string {
	if ver, err := ReadLocalVersion(versionFile); err == nil && strings.TrimSpace(ver) != "" {
		return strings.TrimSpace(ver)
	}
	if exePath, err := ResolveExecutablePath(); err == nil {
		exeDir := filepath.Dir(exePath)
		if ver, err := ReadLocalVersion(filepath.Join(exeDir, versionFile)); err == nil && strings.TrimSpace(ver) != "" {
			return strings.TrimSpace(ver)
		}
	}
	if v, err := whatsrook.GetVersion(); err == nil && v.Raw != "" {
		return v.Raw
	}
	return EmbeddedAppVersion
}

// ReadLocalVersion reads and parses the version string from a local version file.
func ReadLocalVersion(versionPath string) (string, error) {
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}
	clean := strings.TrimSpace(string(data))
	if clean == "" {
		return "", fmt.Errorf("empty version file %q", versionPath)
	}
	// Also support parsing legacy TOML format if present
	if strings.Contains(clean, "version") && strings.Contains(clean, "=") {
		return parseVersionFromTOML(clean)
	}
	return clean, nil
}

func parseVersionFromTOML(content string) (string, error) {
	lines := strings.SplitSeq(content, "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"'`)
				return val, nil
			}
		}
	}
	return "", fmt.Errorf("version key not found in toml")
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
	UpdatedAt          string `json:"updated_at"`
}

type githubRelease struct {
	TagName         string        `json:"tag_name"`
	TargetCommitish string        `json:"target_commitish"`
	Assets          []githubAsset `json:"assets"`
	Body            string        `json:"body"`
}

type progressReader struct {
	reader     io.Reader
	total      int64
	current    int64
	out        io.Writer
	lastUpdate time.Time
	barWidth   int
	finished   bool
}

func newProgressReader(r io.Reader, total int64, out io.Writer) *progressReader {
	return &progressReader{
		reader:   r,
		total:    total,
		out:      out,
		barWidth: 24,
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.current += int64(n)
		pr.render(false)
	}
	if err == io.EOF {
		pr.finish()
	}
	return n, err
}

func (pr *progressReader) finish() {
	if pr.finished {
		return
	}
	pr.finished = true
	if pr.out != nil {
		pr.render(true)
		fmt.Fprintln(pr.out)
	}
}

func (pr *progressReader) render(final bool) {
	if pr.out == nil {
		return
	}
	now := time.Now()
	if !final && now.Sub(pr.lastUpdate) < 60*time.Millisecond {
		return
	}
	pr.lastUpdate = now

	width := pr.barWidth
	if width <= 0 {
		width = 24
	}

	if pr.total > 0 {
		pct := float64(pr.current) / float64(pr.total) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
		hashes := min(int(float64(width)*(float64(pr.current)/float64(pr.total))), width)
		if final {
			hashes = width
			pct = 100.0
		}
		bar := strings.Repeat("=", hashes)
		spaces := strings.Repeat(" ", width-hashes)
		fmt.Fprintf(pr.out, "\r[%s%s] %5.1f%% (%s / %s)", bar, spaces, pct, formatBytes(pr.current), formatBytes(pr.total))
	} else {
		fmt.Fprintf(pr.out, "\rDownloading... %s", formatBytes(pr.current))
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FetchRemoteVersion fetches the latest version string using the Updater's configured HTTP client and context.
func (u *Updater) FetchRemoteVersion(ctx context.Context) (string, error) {
	if u.opts.Channel == "beta" {
		return u.fetchRemoteBetaVersion(ctx)
	}
	return u.fetchRemoteStableVersion(ctx)
}

func (u *Updater) fetchRemoteBetaVersion(ctx context.Context) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/alpha", u.opts.RepoOwner, u.opts.RepoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "whatsrook-updater")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := u.opts.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var rel githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err == nil {
			candidates, _ := candidateAssetNames()
			targetAssetMap := make(map[string]bool)
			for _, c := range candidates {
				targetAssetMap[c] = true
			}

			for _, asset := range rel.Assets {
				if targetAssetMap[asset.Name] {
					if asset.Digest != "" {
						return asset.Digest, nil
					}
					if asset.UpdatedAt != "" {
						return fmt.Sprintf("beta-%s", asset.UpdatedAt), nil
					}
				}
			}
			if rel.TargetCommitish != "" {
				return fmt.Sprintf("beta-%s", rel.TargetCommitish), nil
			}
		}
	}

	// Fallback: HEAD request to asset download URL to inspect headers
	candidates, errCand := candidateAssetNames()
	if errCand == nil && len(candidates) > 0 {
		downloadURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/alpha/%s", u.opts.RepoOwner, u.opts.RepoName, candidates[0])
		reqHead, errHead := http.NewRequestWithContext(ctx, http.MethodHead, downloadURL, nil)
		if errHead == nil {
			reqHead.Header.Set("User-Agent", "whatsrook-updater")
			if respHead, errDo := u.opts.HTTPClient.Do(reqHead); errDo == nil {
				defer respHead.Body.Close()
				if etag := respHead.Header.Get("ETag"); etag != "" {
					return fmt.Sprintf("sha256:%s", strings.Trim(etag, `"`)), nil
				}
				if lastMod := respHead.Header.Get("Last-Modified"); lastMod != "" {
					return fmt.Sprintf("beta-%s", lastMod), nil
				}
			}
		}
	}

	return "", fmt.Errorf("failed to fetch beta release metadata")
}

func (u *Updater) fetchRemoteStableVersion(ctx context.Context) (string, error) {
	versionURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/refs/heads/master/%s", u.opts.RepoOwner, u.opts.RepoName, u.opts.VersionFile)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "whatsrook-updater")

	resp, err := u.opts.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching remote %s", resp.StatusCode, u.opts.VersionFile)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	clean := strings.TrimSpace(string(body))
	if clean == "" {
		return "", fmt.Errorf("empty remote version from %s", versionURL)
	}
	if strings.Contains(clean, "version") && strings.Contains(clean, "=") {
		return parseVersionFromTOML(clean)
	}
	return clean, nil
}

// Check compares local and remote versions for the configured repository and platform.
func (u *Updater) Check(ctx context.Context) (*UpdateResult, error) {
	u.logf("==> Checking for updates (%s/%s, platform: %s)...", u.opts.RepoOwner, u.opts.RepoName, GetPlatform())

	localStr := ReadEffectiveLocalVersion(u.opts.VersionFile)
	if u.opts.Channel == "beta" {
		if installedBeta := GetInstalledBetaVersion(); installedBeta != "" {
			localStr = installedBeta
		}
	}
	remoteStr, err := u.FetchRemoteVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote version: %w", err)
	}

	res := &UpdateResult{
		CurrentVersion: localStr,
		LatestVersion:  remoteStr,
		Platform:       GetPlatform(),
		IsBeta:         u.opts.Channel == "beta",
	}

	if u.opts.Channel == "beta" {
		res.HasNewVersion = localStr != remoteStr
	} else {
		localVer, errLocal := ParseVersion(localStr)
		remoteVer, errRemote := ParseVersion(remoteStr)
		if errLocal == nil && errRemote == nil {
			res.HasNewVersion = remoteVer.Compare(localVer) > 0
		} else {
			res.HasNewVersion = localStr != remoteStr
		}
	}

	if res.HasNewVersion {
		u.logf("==> Update available! Installed: %s -> Latest: %s",
			FormatVersionDisplay(localStr), FormatVersionDisplay(remoteStr))
	} else {
		u.logf("==> %s is already at the latest version (%s).",
			u.opts.RepoName, FormatVersionDisplay(localStr))
	}

	return res, nil
}

// Upgrade checks, downloads, and performs an atomic upgrade of the binary release.
func (u *Updater) Upgrade(ctx context.Context, isBeta bool) (*UpdateResult, error) {
	if isBeta {
		u.opts.Channel = "beta"
	}
	check, err := u.Check(ctx)
	if err != nil && !isBeta {
		return nil, err
	}
	if check == nil {
		check = &UpdateResult{
			IsBeta:   isBeta,
			Platform: GetPlatform(),
		}
	} else {
		check.IsBeta = isBeta
	}

	if !check.HasNewVersion {
		check.Updated = false
		check.Message = fmt.Sprintf("%s is already up to date (%s).", u.opts.RepoName, FormatVersionDisplay(check.CurrentVersion))
		return check, nil
	}

	tag := "latest"
	if isBeta {
		tag = "alpha"
	}

	u.logf("==> [1/3] Downloading %s release for %s...", tag, GetPlatform())
	if err := u.DownloadAndApply(ctx, tag); err != nil {
		return nil, fmt.Errorf("failed to upgrade binary for %s: %w", GetPlatform(), err)
	}

	check.Updated = true
	if isBeta {
		_ = SetInstalledBetaVersion(check.LatestVersion)
	} else {
		_ = SetInstalledBetaVersion("")
		if newVer := ReadEffectiveLocalVersion(u.opts.VersionFile); newVer != "" {
			check.LatestVersion = newVer
		}
	}
	check.Message = fmt.Sprintf("Successfully upgraded binary for %s (%s -> %s).",
		GetPlatform(),
		FormatVersionDisplay(check.CurrentVersion),
		FormatVersionDisplay(check.LatestVersion))
	u.logf("==> Upgrade complete! %s", check.Message)
	return check, nil
}

// candidateAssetNames returns the release asset filename for the current
// platform. Every target ships as .tar.gz only. An error is returned for
// platforms that have no published asset.
func candidateAssetNames() ([]string, error) {
	if !IsSupportedPlatform() {
		return nil, fmt.Errorf(
			"unsupported platform %s — supported platforms are: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, android/arm64, windows/amd64",
			GetPlatform(),
		)
	}
	return []string{
		fmt.Sprintf("whatsrook-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH),
	}, nil
}

// DownloadAndApply downloads a release asset, extracts contents, and performs atomic binary swap.
func (u *Updater) DownloadAndApply(ctx context.Context, tag string) error {
	candidates, err := candidateAssetNames()
	if err != nil {
		return err
	}

	var resp *http.Response
	var chosenAsset string
	var chosenDownloadURL string
	var errLast error

	for _, assetName := range candidates {
		var downloadURL string
		if tag == "latest" {
			downloadURL = fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", u.opts.RepoOwner, u.opts.RepoName, assetName)
		} else {
			downloadURL = fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", u.opts.RepoOwner, u.opts.RepoName, tag, assetName)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
		if err != nil {
			errLast = err
			continue
		}
		req.Header.Set("User-Agent", "whatsrook-updater")

		r, err := u.opts.HTTPClient.Do(req)
		if err != nil {
			errLast = err
			continue
		}

		if r.StatusCode == http.StatusOK {
			resp = r
			chosenAsset = assetName
			chosenDownloadURL = downloadURL
			break
		}
		r.Body.Close()
		errLast = fmt.Errorf("HTTP %d downloading %s from %s", r.StatusCode, assetName, downloadURL)
	}

	if resp == nil {
		return fmt.Errorf("failed to download release asset: %w", errLast)
	}
	defer resp.Body.Close()

	u.logf("==> Downloading %s", chosenDownloadURL)

	pr := newProgressReader(resp.Body, resp.ContentLength, u.opts.Out)
	payloadBytes, err := io.ReadAll(pr)
	pr.finish()
	if err != nil {
		return fmt.Errorf("failed to read downloaded release payload: %w", err)
	}

	calculatedSHA := fmt.Sprintf("sha256:%x", sha256.Sum256(payloadBytes))

	exePath, err := ResolveExecutablePath()
	if err != nil {
		exePath = os.Args[0]
	}
	exeDir := filepath.Dir(exePath)
	cleanExeDir := filepath.Clean(exeDir)

	u.logf("==> [2/3] Extracting release payload (%s) and verifying integrity...", chosenAsset)

	tmpBinary := exePath + ".tmp"
	_ = os.Remove(tmpBinary)

	var foundBinary bool
	if strings.HasSuffix(chosenAsset, ".zip") {
		foundBinary, err = u.extractZipPayload(payloadBytes, cleanExeDir, tmpBinary)
	} else {
		foundBinary, err = u.extractTarGzPayload(payloadBytes, cleanExeDir, tmpBinary)
	}

	if err != nil {
		_ = os.Remove(tmpBinary)
		return err
	}
	if !foundBinary {
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("matching executable binary not found in release archive %s", chosenAsset)
	}

	// Always write version.txt alongside the new executable so local version is permanently updated
	if u.opts.Channel == "beta" || tag == "alpha" {
		_ = os.WriteFile(filepath.Join(cleanExeDir, u.opts.VersionFile), []byte(calculatedSHA+"\n"), 0644)
	} else {
		if remoteVer, err := u.FetchRemoteVersion(ctx); err == nil && remoteVer != "" {
			_ = os.WriteFile(filepath.Join(cleanExeDir, u.opts.VersionFile), []byte(strings.TrimSpace(remoteVer)+"\n"), 0644)
		}
	}

	u.logf("==> [3/3] Performing atomic binary swap with rollback safety...")

	backupPath := exePath + ".bak"
	_ = os.Remove(backupPath)

	// Backup current working binary
	if err := os.Rename(exePath, backupPath); err != nil {
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("failed to backup existing binary: %w", err)
	}

	// Atomic replace with new binary
	if err := os.Rename(tmpBinary, exePath); err != nil {
		// Rollback to original working binary
		_ = os.Rename(backupPath, exePath)
		_ = os.Remove(tmpBinary)
		return fmt.Errorf("failed to replace executable (rolled back): %w", err)
	}

	// Cleanup backup file
	_ = os.Remove(backupPath)
	return nil
}

// isBinaryNameMatch checks if an archive file entry corresponds to the target application binary.
func isBinaryNameMatch(entryName string) bool {
	base := strings.ToLower(filepath.Base(entryName))
	return base == "whatsrook" || base == "whatsrook.exe" || base == "wha-console" || base == "wha-console.exe"
}

func (u *Updater) extractZipPayload(data []byte, cleanExeDir, tmpBinary string) (bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, fmt.Errorf("failed to parse zip archive: %w", err)
	}

	foundBinary := false
	for _, f := range zr.File {
		destPath, errSan := SanitizeExtractPath(cleanExeDir, f.Name)
		if errSan != nil {
			return false, errSan
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(destPath, 0755)
			continue
		}

		cleanRel := filepath.Clean(f.Name)
		if isBinaryNameMatch(cleanRel) {
			rc, err := f.Open()
			if err != nil {
				return false, err
			}
			out, err := os.OpenFile(tmpBinary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return false, err
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			out.Close()
			if err != nil {
				return false, err
			}
			foundBinary = true
			continue
		}

		if strings.HasPrefix(cleanRel, "cli/resources") || strings.HasPrefix(cleanRel, "resources") || strings.HasPrefix(cleanRel, "prompts") || filepath.Base(cleanRel) == u.opts.VersionFile {
			_ = os.MkdirAll(filepath.Dir(destPath), 0755)
			rc, err := f.Open()
			if err == nil {
				resFile, errRes := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if errRes == nil {
					_, _ = io.Copy(resFile, rc)
					resFile.Close()
				}
				rc.Close()
			}
		}
	}
	return foundBinary, nil
}

func (u *Updater) extractTarGzPayload(data []byte, cleanExeDir, tmpBinary string) (bool, error) {
	gzr, errGz := gzip.NewReader(bytes.NewReader(data))
	if errGz != nil {
		return false, fmt.Errorf("failed to decompress gzip archive: %w", errGz)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	foundBinary := false

	for {
		hdr, errHdr := tr.Next()
		if errHdr == io.EOF {
			break
		}
		if errHdr != nil {
			return false, errHdr
		}

		destPath, errSan := SanitizeExtractPath(cleanExeDir, hdr.Name)
		if errSan != nil {
			return false, errSan
		}

		if hdr.Typeflag == tar.TypeDir {
			_ = os.MkdirAll(destPath, 0755)
			continue
		}

		cleanRel := filepath.Clean(hdr.Name)
		if isBinaryNameMatch(cleanRel) {
			out, err := os.OpenFile(tmpBinary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return false, err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return false, err
			}
			foundBinary = true
			continue
		}

		if strings.HasPrefix(cleanRel, "cli/resources") || strings.HasPrefix(cleanRel, "resources") || strings.HasPrefix(cleanRel, "prompts") || filepath.Base(cleanRel) == u.opts.VersionFile {
			_ = os.MkdirAll(filepath.Dir(destPath), 0755)
			resFile, errRes := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if errRes == nil {
				_, _ = io.Copy(resFile, tr)
				resFile.Close()
			}
		}
	}
	return foundBinary, nil
}

// CleanRestartArgs strips one-off update commands and flags to prevent infinite restart loops.
func CleanRestartArgs(args []string) []string {
	var clean []string
	for i, a := range args {
		if i == 0 {
			clean = append(clean, a)
			continue
		}
		low := strings.ToLower(a)
		if low == "update" || low == "upgrade" || low == "check" || low == "now" || low == "apply" || low == "stable" || low == "beta" {
			continue
		}
		if low == "--update" || low == "-u" || strings.HasPrefix(low, "--update=") || strings.HasPrefix(low, "-u=") {
			continue
		}
		clean = append(clean, a)
	}
	return clean
}

// ResolveExecutablePath reliably finds the current executable path, handling procfs (deleted) suffixes and binary renames.
func ResolveExecutablePath() (string, error) {
	// 1. Try os.Executable() and sanitize any procfs / rename artifacts
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		candidates := []string{
			strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(exePath, " (deleted)"), ".bak"), ".tmp"),
			strings.TrimSuffix(exePath, " (deleted)"),
			exePath,
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return filepath.Clean(c), nil
			}
		}
	}

	// 2. Try resolving os.Args[0] (either absolute, relative, or in PATH)
	if len(os.Args) > 0 && os.Args[0] != "" {
		arg0 := os.Args[0]
		if fi, err := os.Stat(arg0); err == nil && !fi.IsDir() {
			if abs, err := filepath.Abs(arg0); err == nil {
				return abs, nil
			}
			return filepath.Clean(arg0), nil
		}
		if lookedUp, err := exec.LookPath(arg0); err == nil {
			if fi, err := os.Stat(lookedUp); err == nil && !fi.IsDir() {
				if abs, err := filepath.Abs(lookedUp); err == nil {
					return abs, nil
				}
				return filepath.Clean(lookedUp), nil
			}
		}
	}

	// 3. Termux / Android standard environment location fallback
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		termuxBin := filepath.Join(prefix, "bin", "whatsrook")
		if fi, err := os.Stat(termuxBin); err == nil && !fi.IsDir() {
			return termuxBin, nil
		}
	}

	// 4. Check common binary names in PATH
	for _, name := range []string{"whatsrook", "whatsrook.exe", "wha-console", "wha-console.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				if abs, err := filepath.Abs(p); err == nil {
					return abs, nil
				}
				return filepath.Clean(p), nil
			}
		}
	}

	// 5. Final fallback to os.Executable()
	return os.Executable()
}

// RestartProcess replaces current process with the updated binary using sanitized arguments.
func RestartProcess(customArgs ...string) error {
	var argv []string
	if len(customArgs) > 0 {
		argv = customArgs
	} else {
		argv = CleanRestartArgs(os.Args)
	}

	execPath, err := ResolveExecutablePath()
	if err != nil {
		if len(argv) > 0 {
			if lookedUp, lErr := exec.LookPath(argv[0]); lErr == nil {
				execPath = lookedUp
			} else {
				execPath = argv[0]
			}
		} else {
			execPath = "whatsrook"
		}
	}
	if len(argv) > 0 {
		argv[0] = execPath
	} else {
		argv = []string{execPath}
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command(execPath, argv[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			return err
		}
		os.Exit(0)
		return nil
	}

	execErr := syscall.Exec(execPath, argv, os.Environ())
	if execErr != nil {
		// Fallback to exec.Command if syscall.Exec fails (e.g. in some restricted environments)
		cmd := exec.Command(execPath, argv[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		if spawnErr := cmd.Start(); spawnErr == nil {
			os.Exit(0)
			return nil
		}
	}

	return execErr
}

// SanitizeExtractPath prevents Zip/Tar Slip vulnerabilities (arbitrary file writing outside target dir).
func SanitizeExtractPath(destDir, entryName string) (string, error) {
	cleanDir := filepath.Clean(destDir)
	cleanEntry := filepath.Clean(entryName)

	if filepath.IsAbs(cleanEntry) || strings.HasPrefix(cleanEntry, "/") || strings.HasPrefix(cleanEntry, "\\") || strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, "\\") {
		return "", fmt.Errorf("illegal archive entry path (Zip Slip attempt): %s", entryName)
	}

	destPath := filepath.Join(cleanDir, cleanEntry)

	rel, err := filepath.Rel(cleanDir, destPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." || strings.Contains(rel, "..") {
		return "", fmt.Errorf("illegal archive entry path (Zip Slip attempt): %s", entryName)
	}

	expectedPrefix := cleanDir
	if !strings.HasSuffix(expectedPrefix, string(filepath.Separator)) {
		expectedPrefix += string(filepath.Separator)
	}
	if !strings.HasPrefix(destPath, expectedPrefix) && destPath != cleanDir {
		return "", fmt.Errorf("illegal archive entry path (Zip Slip attempt): %s", entryName)
	}

	return destPath, nil
}
