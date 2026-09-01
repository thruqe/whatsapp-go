package cliutils

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	utils "whatsrook"
)

// GitRepoTarget encapsulates target repository information.
type GitRepoTarget struct {
	Host     string // e.g. "github.com", "gitlab.com"
	Owner    string // e.g. "Thruqe"
	Repo     string // e.g. "whatsrook"
	FullName string // e.g. "Thruqe/whatsrook"
	Branch   string // e.g. "master", "main"
	Format   string // "zip", "tar", "tar.gz"
	RawURL   string
}

// GitUser represents a GitHub user profile.
type GitUser struct {
	Login             string `json:"login"`
	ID                int64  `json:"id"`
	NodeID            string `json:"node_id"`
	AvatarURL         string `json:"avatar_url"`
	HTMLURL           string `json:"html_url"`
	Name              string `json:"name"`
	Company           string `json:"company"`
	Blog              string `json:"blog"`
	Location          string `json:"location"`
	Email             string `json:"email"`
	Bio               string `json:"bio"`
	TwitterUsername   string `json:"twitter_username"`
	PublicRepos       int    `json:"public_repos"`
	PublicGists       int    `json:"public_gists"`
	Followers         int    `json:"followers"`
	Following         int    `json:"following"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	TotalPrivateRepos int    `json:"total_private_repos"`
	OwnedPrivateRepos int    `json:"owned_private_repos"`
	DiskUsage         int64  `json:"disk_usage"`
	Plan              *struct {
		Name string `json:"name"`
	} `json:"plan,omitempty"`
}

// GitRepo represents a GitHub repository item.
type GitRepo struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	FullName      string   `json:"full_name"`
	Private       bool     `json:"private"`
	HTMLURL       string   `json:"html_url"`
	Description   string   `json:"description"`
	Fork          bool     `json:"fork"`
	CloneURL      string   `json:"clone_url"`
	SSHURL        string   `json:"ssh_url"`
	DefaultBranch string   `json:"default_branch"`
	Stars         int      `json:"stargazers_count"`
	Watchers      int      `json:"watchers_count"`
	Forks         int      `json:"forks_count"`
	OpenIssues    int      `json:"open_issues_count"`
	Language      string   `json:"language"`
	Size          int64    `json:"size"` // size in KB
	Topics        []string `json:"topics"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	PushedAt      string   `json:"pushed_at"`
	Owner         struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
	} `json:"owner"`
	License *struct {
		Key    string `json:"key"`
		Name   string `json:"name"`
		SpdxID string `json:"spdx_id"`
	} `json:"license,omitempty"`
}

// GitBranch represents a branch in a repository.
type GitBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
}

// GitCommit represents a Git commit in a repository.
type GitCommit struct {
	SHA     string `json:"sha"`
	NodeID  string `json:"node_id"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Author struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
		Message string `json:"message"`
	} `json:"commit"`
	Author *struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
	} `json:"author,omitempty"`
}

// GitRelease represents a GitHub release.
type GitRelease struct {
	ID          int64      `json:"id"`
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	CreatedAt   string     `json:"created_at"`
	PublishedAt string     `json:"published_at"`
	HTMLURL     string     `json:"html_url"`
	Assets      []GitAsset `json:"assets"`
}

// GitAsset represents a release asset file.
type GitAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadCount      int    `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// GitContent represents a file or directory in a repository.
type GitContent struct {
	Type        string `json:"type"` // "file", "dir", "symlink"
	Size        int64  `json:"size"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	URL         string `json:"url"`
	GitURL      string `json:"git_url"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
	Content     string `json:"content"`  // base64 encoded
	Encoding    string `json:"encoding"` // "base64"
}

// GitIssue represents an issue in a repository.
type GitIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	Comments  int    `json:"comments"`
	CreatedAt string `json:"created_at"`
	User      struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// ParseGitTarget parses a repository identifier or URL into a structured GitRepoTarget.
func ParseGitTarget(raw string) (*GitRepoTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty repository identifier")
	}

	target := &GitRepoTarget{
		Host:   "github.com",
		Format: "zip",
		RawURL: raw,
	}

	// Clean git prefixes/suffixes
	cleaned := strings.TrimPrefix(raw, "git@github.com:")
	cleaned = strings.TrimPrefix(cleaned, "http://")
	cleaned = strings.TrimPrefix(cleaned, "https://")
	cleaned = strings.TrimSuffix(cleaned, ".git")

	// Parse host if specified
	if strings.HasPrefix(cleaned, "github.com/") {
		target.Host = "github.com"
		cleaned = strings.TrimPrefix(cleaned, "github.com/")
	} else if strings.HasPrefix(cleaned, "gitlab.com/") {
		target.Host = "gitlab.com"
		cleaned = strings.TrimPrefix(cleaned, "gitlab.com/")
	}

	// Handle tree/branch URLs: owner/repo/tree/branch-name/subpath
	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 {
		target.Owner = parts[0]
		target.Repo = parts[1]
		target.FullName = target.Owner + "/" + target.Repo

		if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "commits" || parts[2] == "blob") {
			target.Branch = parts[3]
		}
		return target, nil
	}

	return nil, fmt.Errorf("invalid repository format %q (expected owner/repo or git URL)", raw)
}

func newGitHTTPClient() *http.Client {
	return utils.NewHTTPClient(90 * time.Second)
}

func setGitHeaders(req *http.Request, token string) {
	req.Header.Set("User-Agent", "WhatsRook-Bot/1.0 (+https://github.com/ThruqeLabs/whatsrook)")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		if strings.HasPrefix(token, "ghp_") || strings.HasPrefix(token, "github_pat_") || strings.HasPrefix(token, "gho_") {
			req.Header.Set("Authorization", "Bearer "+token)
		} else if strings.HasPrefix(token, "Bearer ") || strings.HasPrefix(token, "token ") {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

// FetchGitHubUser queries the profile of the authenticated user (or specified username).
func FetchGitHubUser(ctx context.Context, token, username string) (*GitUser, string, error) {
	apiURL := "https://api.github.com/user"
	if username != "" {
		apiURL = "https://api.github.com/users/" + url.PathEscape(username)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	scopes := resp.Header.Get("X-OAuth-Scopes")

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, "", errors.New("invalid or expired personal access token (401 Unauthorized)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", fmt.Errorf("user %q not found on GitHub", username)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("github api error (http %d): %s", resp.StatusCode, string(body))
	}

	var user GitUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, "", fmt.Errorf("failed to decode user json: %w", err)
	}

	return &user, scopes, nil
}

// FetchGitHubRepo queries repository details.
func FetchGitHubRepo(ctx context.Context, token, owner, repo string) (*GitRepo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository %s/%s not found (or private repository requiring authentication)", owner, repo)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github error (http %d): %s", resp.StatusCode, string(body))
	}

	var r GitRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// FetchGitHubBranches queries branch list for a repository.
func FetchGitHubBranches(ctx context.Context, token, owner, repo string, limit int) ([]GitBranch, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?per_page=%d", url.PathEscape(owner), url.PathEscape(repo), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("branches request returned http %d", resp.StatusCode)
	}

	var branches []GitBranch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, err
	}
	return branches, nil
}

// FetchGitHubCommits queries recent commits for a repository branch.
func FetchGitHubCommits(ctx context.Context, token, owner, repo, ref string, limit int) ([]GitCommit, error) {
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?per_page=%d", url.PathEscape(owner), url.PathEscape(repo), limit)
	if ref != "" {
		apiURL += "&sha=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commits request returned http %d", resp.StatusCode)
	}

	var commits []GitCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, err
	}
	return commits, nil
}

// FetchGitHubReleases queries releases for a repository.
func FetchGitHubReleases(ctx context.Context, token, owner, repo string, limit int) ([]GitRelease, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d", url.PathEscape(owner), url.PathEscape(repo), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases request returned http %d", resp.StatusCode)
	}

	var releases []GitRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// FetchGitHubContents queries repository tree contents or a single file content.
func FetchGitHubContents(ctx context.Context, token, owner, repo, path, ref string) ([]GitContent, *GitContent, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), cleanPath)
	if ref != "" {
		apiURL += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("contents request returned http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	// Could be an array of files/dirs or a single file object
	var list []GitContent
	if err := json.Unmarshal(body, &list); err == nil {
		return list, nil, nil
	}

	var single GitContent
	if err := json.Unmarshal(body, &single); err == nil {
		return nil, &single, nil
	}

	return nil, nil, fmt.Errorf("failed to parse repository contents")
}

// SearchGitHubRepos searches repositories matching a query string.
func SearchGitHubRepos(ctx context.Context, token, query string, limit int) ([]GitRepo, error) {
	if limit <= 0 || limit > 20 {
		limit = 6
	}
	apiURL := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&per_page=%d", url.QueryEscape(query), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search request returned http %d", resp.StatusCode)
	}

	var result struct {
		Items []GitRepo `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

// FetchUserRepos lists public and private repositories of the user.
func FetchUserRepos(ctx context.Context, token, username string, limit int) ([]GitRepo, error) {
	if limit <= 0 || limit > 30 {
		limit = 12
	}
	apiURL := "https://api.github.com/user/repos?sort=updated&per_page=" + fmt.Sprintf("%d", limit)
	if username != "" {
		apiURL = fmt.Sprintf("https://api.github.com/users/%s/repos?sort=updated&per_page=%d", url.PathEscape(username), limit)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repos request returned http %d", resp.StatusCode)
	}

	var repos []GitRepo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// StarGitHubRepo stars a repository on behalf of the authenticated user.
func StarGitHubRepo(ctx context.Context, token, owner, repo string) error {
	if token == "" {
		return errors.New("authentication required: please log in first via `git login <token>`")
	}
	apiURL := fmt.Sprintf("https://api.github.com/user/starred/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, nil)
	if err != nil {
		return err
	}
	setGitHeaders(req, token)
	req.Header.Set("Content-Length", "0")

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("star operation returned http %d", resp.StatusCode)
	}
	return nil
}

// UnstarGitHubRepo unstars a repository on behalf of the authenticated user.
func UnstarGitHubRepo(ctx context.Context, token, owner, repo string) error {
	if token == "" {
		return errors.New("authentication required: please log in first via `git login <token>`")
	}
	apiURL := fmt.Sprintf("https://api.github.com/user/starred/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unstar operation returned http %d", resp.StatusCode)
	}
	return nil
}

// ForkGitHubRepo forks a repository to the authenticated user's account.
func ForkGitHubRepo(ctx context.Context, token, owner, repo string) (*GitRepo, error) {
	if token == "" {
		return nil, errors.New("authentication required: please log in first via `git login <token>`")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/forks", url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fork failed (http %d): %s", resp.StatusCode, string(body))
	}

	var r GitRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateGitHubRepo creates a new repository under the authenticated user's account.
func CreateGitHubRepo(ctx context.Context, token, name, description string, isPrivate, autoInit bool) (*GitRepo, error) {
	if token == "" {
		return nil, errors.New("authentication required: please log in first via `git login <token>`")
	}
	payload := map[string]any{
		"name":        name,
		"description": description,
		"private":     isPrivate,
		"auto_init":   autoInit,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/user/repos", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("create repo failed (http %d): %s", resp.StatusCode, string(respBody))
	}

	var r GitRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteGitHubRepo deletes a repository owned by the authenticated user.
func DeleteGitHubRepo(ctx context.Context, token, owner, repo string) error {
	if token == "" {
		return errors.New("authentication required: please log in first via `git login <token>`")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("delete repo failed (http %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// FetchGitHubIssues queries open issues for a repository.
func FetchGitHubIssues(ctx context.Context, token, owner, repo string, limit int) ([]GitIssue, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=open&per_page=%d", url.PathEscape(owner), url.PathEscape(repo), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("issues request returned http %d", resp.StatusCode)
	}

	var issues []GitIssue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// CreateGitHubIssue creates a new issue in a repository.
func CreateGitHubIssue(ctx context.Context, token, owner, repo, title, body string) (*GitIssue, error) {
	if token == "" {
		return nil, errors.New("authentication required: please log in first via `git login <token>`")
	}
	payload := map[string]string{
		"title": title,
		"body":  body,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	setGitHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")

	client := newGitHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("create issue failed (http %d): %s", resp.StatusCode, string(respBody))
	}

	var issue GitIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// DownloadRepoArchive downloads a repository branch/ref archive as .zip or .tar.gz.
func DownloadRepoArchive(ctx context.Context, token string, target *GitRepoTarget) (data []byte, filename string, mime string, err error) {
	if target == nil {
		return nil, "", "", errors.New("nil target")
	}

	format := strings.ToLower(target.Format)
	isTar := format == "tar" || format == "tar.gz" || format == "tgz"

	// Resolve default branch if branch is not specified
	branch := target.Branch
	if branch == "" && target.Host == "github.com" {
		if repoInfo, err := FetchGitHubRepo(ctx, token, target.Owner, target.Repo); err == nil && repoInfo != nil && repoInfo.DefaultBranch != "" {
			branch = repoInfo.DefaultBranch
		} else {
			branch = "main" // fallback default
		}
	} else if branch == "" {
		branch = "main"
	}

	// Prepare filename
	safeRepo := strings.ReplaceAll(target.Repo, "/", "-")
	safeBranch := strings.ReplaceAll(branch, "/", "-")
	if isTar {
		filename = fmt.Sprintf("%s-%s.tar.gz", safeRepo, safeBranch)
		mime = "application/gzip"
	} else {
		filename = fmt.Sprintf("%s-%s.zip", safeRepo, safeBranch)
		mime = "application/zip"
	}

	// 1. Direct GitHub Archive API / HTTP
	if target.Host == "github.com" {
		endpoint := "zipball"
		if isTar {
			endpoint = "tarball"
		}
		archiveURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/%s/%s", url.PathEscape(target.Owner), url.PathEscape(target.Repo), endpoint, url.PathEscape(branch))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
		if err == nil {
			setGitHeaders(req, token)
			client := &http.Client{
				Timeout: 120 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					// Follow redirects normally
					if len(via) >= 10 {
						return errors.New("stopped after 10 redirects")
					}
					return nil
				},
			}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				if err == nil && len(data) > 0 {
					return data, filename, mime, nil
				}
			} else if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// 2. Direct GitLab Archive HTTP
	if target.Host == "gitlab.com" {
		ext := "zip"
		if isTar {
			ext = "tar.gz"
		}
		archiveURL := fmt.Sprintf("https://gitlab.com/%s/%s/-/archive/%s/%s-%s.%s", target.Owner, target.Repo, branch, target.Repo, branch, ext)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
		if err == nil {
			client := utils.NewHTTPClient(120 * time.Second)
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				if err == nil && len(data) > 0 {
					return data, filename, mime, nil
				}
			} else if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// 3. Fallback: Clone via Git CLI and Archive
	if _, lookErr := exec.LookPath("git"); lookErr == nil {
		cloneURL := fmt.Sprintf("https://%s/%s/%s.git", target.Host, target.Owner, target.Repo)
		if token != "" && target.Host == "github.com" {
			cloneURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, target.Owner, target.Repo)
		}

		tempDir, err := os.MkdirTemp("", "whatsrook-git-*")
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to create temp workspace: %w", err)
		}
		defer os.RemoveAll(tempDir)

		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "-b", branch, cloneURL, tempDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			// If shallow branch failed, try standard clone
			cmdAll := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, tempDir)
			if outAll, errAll := cmdAll.CombinedOutput(); errAll != nil {
				return nil, "", "", fmt.Errorf("git clone failed: %s | %s", string(out), string(outAll))
			}
		}

		var archiveBuf bytes.Buffer
		if isTar {
			if err := CreateTarGzFromDir(tempDir, &archiveBuf); err != nil {
				return nil, "", "", fmt.Errorf("failed creating tar.gz: %w", err)
			}
		} else {
			if err := CreateZipFromDir(tempDir, &archiveBuf); err != nil {
				return nil, "", "", fmt.Errorf("failed creating zip: %w", err)
			}
		}

		return archiveBuf.Bytes(), filename, mime, nil
	}

	return nil, "", "", fmt.Errorf("failed to download repository archive for %s (branch %s)", target.FullName, branch)
}

// CreateZipFromDir creates a zip archive in memory from a directory.
func CreateZipFromDir(sourceDir string, w io.Writer) error {
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	baseDirName := filepath.Base(sourceDir)

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		// Skip internal .git metadata to keep archive clean and small
		if strings.HasPrefix(relPath, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		zipEntryName := filepath.ToSlash(filepath.Join(baseDirName, relPath))

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipEntryName
		header.Method = zip.Deflate

		if info.IsDir() {
			header.Name += "/"
			_, err = zipWriter.CreateHeader(header)
			return err
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// CreateTarGzFromDir creates a tar.gz archive in memory from a directory.
func CreateTarGzFromDir(sourceDir string, w io.Writer) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	baseDirName := filepath.Base(sourceDir)

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		if strings.HasPrefix(relPath, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		header.Name = filepath.ToSlash(filepath.Join(baseDirName, relPath))
		if info.IsDir() {
			header.Name += "/"
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}

// DecodeBase64Content decodes base64 string content from GitHub API.
func DecodeBase64Content(raw string) ([]byte, error) {
	clean := strings.ReplaceAll(raw, "\n", "")
	clean = strings.ReplaceAll(clean, "\r", "")
	return base64.StdEncoding.DecodeString(clean)
}
