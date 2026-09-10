// Package updater provides self-update capabilities for WhatsRook matching system package manager designs (brew/apt/dnf style).
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"whatsrook"
)

const (
	DefaultRepoOwner = "ThruqeLabs"
	DefaultRepoName  = "whatsrook"
)

var EmbeddedAppVersion = func() string {
	if v, err := whatsrook.GetVersion(); err == nil && v.Raw != "" {
		return v.Raw
	}
	return "26.09.dev"
}()

// Backward-compatible exports for external callers.
const (
	RepoOwner = DefaultRepoOwner
	RepoName  = DefaultRepoName
)

// Version holds a parsed release version following the YY.MM.CRYPTO_PATCH_EXTRA_BUILD_INFO format.
type Version struct {
	Year           int
	Major          int // alias for Year for backward compatibility
	Month          int
	Minor          int // alias for Month for backward compatibility
	Patch          string
	NumericPatch   int
	ExtraBuildInfo string
	Raw            string
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
	VersionFile string    // Deprecated: releases are fetched via GitHub API
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
	if opts.Channel == "" {
		opts.Channel = GetDefaultChannel()
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Updater{opts: opts}
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
	"windows/arm64": true,
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

// autoUpdateFilePath returns the path used to persist the autoupdate on/off preference.
func autoUpdateFilePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		p := filepath.Join(dir, "whatsrook")
		_ = os.MkdirAll(p, 0755)
		return filepath.Join(p, ".autoupdate")
	}
	if exe, err := ResolveExecutablePath(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".autoupdate")
	}
	return ".autoupdate"
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

// IsBetaVersion reports whether a version identifier represents a beta/alpha/commit build.
func IsBetaVersion(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return false
	}
	for _, prefix := range []string{"beta", "alpha", "sha256:", "sha:"} {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	if strings.Contains(v, "-beta") || strings.Contains(v, "-alpha") || strings.Contains(v, ".beta") || strings.Contains(v, ".alpha") {
		return true
	}
	clean := strings.TrimPrefix(v, "v")
	// Commit hashes (e.g. 34ac0... or 40-char git commit SHA)
	if isHex(clean) && len(clean) >= 5 {
		return true
	}
	// If it cannot be parsed as a standard 3-component semantic version (e.g. 4.9.26),
	// treat it as a non-stable build.
	if _, err := ParseVersion(clean); err != nil {
		return true
	}
	return false
}

// CurrentIsBeta reports whether the currently installed or running binary is a beta/alpha build.
func CurrentIsBeta() bool {
	if beta := GetInstalledBetaVersion(); beta != "" {
		return true
	}
	binVer := GetBinaryVersion()
	return IsBetaVersion(binVer)
}

// GetDefaultChannel returns the natural channel for the current binary:
// "beta" if running a beta/alpha build, or "stable" if running a stable release.
func GetDefaultChannel() string {
	if CurrentIsBeta() {
		return "beta"
	}
	return "stable"
}

// GetStoredChannel returns the persisted update channel ("stable" or "beta"),
// defaulting to the natural channel of the currently running binary when no preference has been saved yet.
func GetStoredChannel() string {
	data, err := os.ReadFile(channelFilePath())
	if err == nil {
		val := strings.TrimSpace(strings.ToLower(string(data)))
		if val == "beta" || val == "stable" {
			return val
		}
	}
	return GetDefaultChannel()
}

// SetStoredChannel writes the update channel preference to disk.
func SetStoredChannel(channel string) error {
	channel = strings.TrimSpace(strings.ToLower(channel))
	if channel != "stable" && channel != "beta" {
		return fmt.Errorf("invalid channel %q: must be \"stable\" or \"beta\"", channel)
	}
	return os.WriteFile(channelFilePath(), []byte(channel+"\n"), 0644)
}

// GetStoredAutoUpdate returns whether auto-update is enabled.
// It checks AUTOUPDATE and AUTO_UPDATE environment variables first, then the local config file.
func GetStoredAutoUpdate() bool {
	if env := strings.TrimSpace(strings.ToLower(os.Getenv("AUTOUPDATE"))); env != "" {
		return env == "on" || env == "true" || env == "1" || env == "yes" || env == "enable" || env == "enabled"
	}
	if env := strings.TrimSpace(strings.ToLower(os.Getenv("AUTO_UPDATE"))); env != "" {
		return env == "on" || env == "true" || env == "1" || env == "yes" || env == "enable" || env == "enabled"
	}
	data, err := os.ReadFile(autoUpdateFilePath())
	if err == nil {
		val := strings.TrimSpace(strings.ToLower(string(data)))
		return val == "on" || val == "true" || val == "1" || val == "yes" || val == "enable" || val == "enabled"
	}
	return false
}

// SetStoredAutoUpdate writes the auto-update preference to disk.
func SetStoredAutoUpdate(enabled bool) error {
	val := "off\n"
	if enabled {
		val = "on\n"
	}
	return os.WriteFile(autoUpdateFilePath(), []byte(val), 0644)
}

// ParseVersion converts a version string (e.g. "26.09.7a3b4c1", "26.09.7a3b4c1_prod", "4.9.26") into a Version struct.
func ParseVersion(raw string) (Version, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "v")

	parts := strings.Split(clean, ".")
	if len(parts) < 2 {
		return Version{Raw: raw}, fmt.Errorf("invalid version format: %s", raw)
	}

	year, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return Version{Raw: raw}, fmt.Errorf("non-numeric year/month segment in %s", raw)
	}

	var patchStr, extra string
	var numericPatch int
	if len(parts) >= 3 {
		patchRaw := strings.Join(parts[2:], ".")
		if idx := strings.IndexAny(patchRaw, "_+"); idx != -1 {
			patchStr = patchRaw[:idx]
			extra = patchRaw[idx+1:]
		} else {
			patchStr = patchRaw
		}
		if num, err := strconv.Atoi(patchStr); err == nil {
			numericPatch = num
		}
	}

	return Version{
		Year:           year,
		Major:          year,
		Month:          month,
		Minor:          month,
		Patch:          patchStr,
		NumericPatch:   numericPatch,
		ExtraBuildInfo: extra,
		Raw:            raw,
	}, nil
}

// Compare compares two versions, returning -1/0/+1 like cmp.Compare.
func (v Version) Compare(other Version) int {
	if v.Year != other.Year {
		if v.Year > other.Year {
			return 1
		}
		return -1
	}
	if v.Month != other.Month {
		if v.Month > other.Month {
			return 1
		}
		return -1
	}
	if v.NumericPatch != 0 || other.NumericPatch != 0 {
		if v.NumericPatch != other.NumericPatch {
			if v.NumericPatch > other.NumericPatch {
				return 1
			}
			return -1
		}
	}
	return 0
}

// BinaryVersion is set at compile time via -ldflags "-X whatsrook/cmd/updater.BinaryVersion=...".
var BinaryVersion = ""

// CommitSHA is set at compile time via -ldflags "-X whatsrook/cmd/updater.CommitSHA=...".
var CommitSHA = ""

// GetBinaryVersion returns the running binary's internal version without inspecting any external files.
func GetBinaryVersion() string {
	if BinaryVersion != "" && BinaryVersion != "dev" {
		return strings.TrimSpace(BinaryVersion)
	}
	if CommitSHA != "" && CommitSHA != "none" {
		return strings.TrimSpace(CommitSHA)
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return strings.TrimSpace(info.Main.Version)
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				return strings.TrimSpace(s.Value)
			}
		}
	}
	if v, err := whatsrook.GetVersion(); err == nil && strings.TrimSpace(v.Raw) != "" {
		return strings.TrimSpace(v.Raw)
	}
	return EmbeddedAppVersion
}

// EqualVersions compares two version identifiers (semver or git commit hashes).
func EqualVersions(v1, v2 string) bool {
	v1 = strings.TrimSpace(strings.ToLower(v1))
	v2 = strings.TrimSpace(strings.ToLower(v2))
	if v1 == v2 {
		return true
	}
	v1Clean := strings.TrimPrefix(strings.TrimPrefix(v1, "beta-"), "alpha-")
	v2Clean := strings.TrimPrefix(strings.TrimPrefix(v2, "beta-"), "alpha-")
	if v1Clean == v2Clean {
		return true
	}
	if isHex(v1Clean) && isHex(v2Clean) && len(v1Clean) >= 5 && len(v2Clean) >= 5 {
		return strings.HasPrefix(v1Clean, v2Clean) || strings.HasPrefix(v2Clean, v1Clean)
	}
	return false
}

// GetAppVersion returns the current binary version formatted for display.
func GetAppVersion() string {
	if CurrentIsBeta() || GetStoredChannel() == "beta" {
		if installedBeta := GetInstalledBetaVersion(); installedBeta != "" {
			return FormatVersionDisplay(installedBeta)
		}
	}
	return FormatVersionDisplay(GetBinaryVersion())
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
			// 1. Extract commit SHA from release body (e.g. "**Commit:** <sha>" or "Commit: <sha>")
			for line := range strings.SplitSeq(rel.Body, "\n") {
				line = strings.TrimSpace(line)
				if after, ok := strings.CutPrefix(line, "**Commit:**"); ok {
					sha := strings.TrimSpace(after)
					if isHex(sha) && len(sha) >= 7 {
						return sha, nil
					}
				}
				if after, ok := strings.CutPrefix(line, "Commit:"); ok {
					sha := strings.TrimSpace(after)
					if isHex(sha) && len(sha) >= 7 {
						return sha, nil
					}
				}
			}

			// 2. Check if TargetCommitish is an exact commit SHA
			if isHex(rel.TargetCommitish) && len(rel.TargetCommitish) >= 7 {
				return rel.TargetCommitish, nil
			}

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

	// Fallback 1: Query git/ref/tags/alpha API endpoint for exact commit SHA
	refURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/ref/tags/alpha", u.opts.RepoOwner, u.opts.RepoName)
	if reqRef, errRef := http.NewRequestWithContext(ctx, http.MethodGet, refURL, nil); errRef == nil {
		reqRef.Header.Set("User-Agent", "whatsrook-updater")
		reqRef.Header.Set("Accept", "application/vnd.github+json")
		if respRef, errDo := u.opts.HTTPClient.Do(reqRef); errDo == nil {
			defer respRef.Body.Close()
			if respRef.StatusCode == http.StatusOK {
				var refObj struct {
					Object struct {
						SHA string `json:"sha"`
					} `json:"object"`
				}
				if err := json.NewDecoder(respRef.Body).Decode(&refObj); err == nil && refObj.Object.SHA != "" {
					return refObj.Object.SHA, nil
				}
			}
		}
	}

	// Fallback 2: HEAD request to asset download URL to inspect headers
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
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.opts.RepoOwner, u.opts.RepoName)
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching latest release", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("failed to parse release metadata: %w", err)
	}

	tag := strings.TrimSpace(rel.TagName)
	tag = strings.TrimPrefix(tag, "v")
	if tag == "" {
		return "", fmt.Errorf("empty tag in latest release")
	}
	return tag, nil
}

// Check compares local and remote versions for the configured repository and platform.
func (u *Updater) Check(ctx context.Context) (*UpdateResult, error) {
	u.logf("==> Checking for updates (%s/%s, platform: %s)...", u.opts.RepoOwner, u.opts.RepoName, GetPlatform())

	localStr := GetBinaryVersion()
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
		res.HasNewVersion = !EqualVersions(localStr, remoteStr)
	} else {
		localVer, errLocal := ParseVersion(localStr)
		remoteVer, errRemote := ParseVersion(remoteStr)
		if errLocal == nil && errRemote == nil {
			res.HasNewVersion = remoteVer.Compare(localVer) > 0
		} else {
			res.HasNewVersion = !EqualVersions(localStr, remoteStr)
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
		_ = SetStoredChannel("beta")
	} else {
		_ = SetInstalledBetaVersion("")
		_ = SetStoredChannel("stable")
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
			"unsupported platform %s — supported platforms are: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, android/arm64, windows/amd64, windows/arm64",
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

		if strings.HasPrefix(cleanRel, "cli/resources") || strings.HasPrefix(cleanRel, "resources") || strings.HasPrefix(cleanRel, "prompts") {
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

		if strings.HasPrefix(cleanRel, "cli/resources") || strings.HasPrefix(cleanRel, "resources") || strings.HasPrefix(cleanRel, "prompts") {
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
		if low == "autoupdate" || low == "auto-update" || strings.HasPrefix(low, "autoupdate=") || strings.HasPrefix(low, "auto-update=") {
			continue
		}
		if low == "on" || low == "off" || low == "enable" || low == "disable" || low == "status" {
			// If preceding argument was autoupdate, skip it
			if i > 1 {
				prev := strings.ToLower(args[i-1])
				if prev == "autoupdate" || prev == "auto-update" {
					continue
				}
			}
		}
		if low == "--update" || low == "-u" || strings.HasPrefix(low, "--update=") || strings.HasPrefix(low, "-u=") {
			continue
		}
		clean = append(clean, a)
	}
	return clean
}

// RestartProcess cleanly restarts the current executable, preserving arguments and environment.
func RestartProcess(extraArgs ...string) error {
	exe, err := ResolveExecutablePath()
	if err != nil {
		exe, err = os.Executable()
		if err != nil {
			return fmt.Errorf("failed to resolve executable path for restart: %w", err)
		}
	}

	cleanArgs := CleanRestartArgs(os.Args)
	if len(extraArgs) > 0 {
		cleanArgs = append(cleanArgs, extraArgs...)
	}

	// On Unix platforms, try in-place replacement via syscall.Exec
	if runtime.GOOS != "windows" {
		_ = syscall.Exec(exe, cleanArgs, os.Environ())
	}

	// Cross-platform fallback / Windows: start new process and terminate current
	var cmdArgs []string
	if len(cleanArgs) > 1 {
		cmdArgs = cleanArgs[1:]
	}
	cmd := exec.Command(exe, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to spawn restarted process: %w", err)
	}
	os.Exit(0)
	return nil
}

// PerformAutoUpdate checks for available updates and automatically upgrades the binary.
// Returns (updated bool, err error).
func PerformAutoUpdate(ctx context.Context, out io.Writer) (bool, error) {
	channel := GetStoredChannel()
	if CurrentIsBeta() {
		channel = "beta"
	}

	up := New(Options{
		Channel: channel,
		Out:     out,
	})

	res, err := up.Check(ctx)
	if err != nil {
		return false, err
	}
	if !res.HasNewVersion {
		return false, nil
	}

	if out != nil {
		fmt.Fprintf(out, "==> Auto-update: new version available (%s -> %s). Upgrading...\n", res.CurrentVersion, res.LatestVersion)
	}

	upgradeRes, err := up.Upgrade(ctx, channel == "beta")
	if err != nil {
		return false, err
	}

	return upgradeRes.Updated, nil
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
