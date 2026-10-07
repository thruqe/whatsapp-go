package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"whatsrook/pkg/addons/sdk"
)

type repoInfo struct {
	FullName        string  `json:"full_name"`
	Description     *string `json:"description"`
	StargazersCount uint64  `json:"stargazers_count"`
	ForksCount      uint64  `json:"forks_count"`
	OpenIssuesCount uint64  `json:"open_issues_count"`
	Language        *string `json:"language"`
	DefaultBranch   string  `json:"default_branch"`
	HTMLURL         string  `json:"html_url"`
}

type commitItem struct {
	SHA    string       `json:"sha"`
	Commit commitDetail `json:"commit"`
}

type commitDetail struct {
	Message string        `json:"message"`
	Author  *commitAuthor `json:"author"`
}

type commitAuthor struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

type branchItem struct {
	Name string `json:"name"`
}

type releaseItem struct {
	TagName     string  `json:"tag_name"`
	Name        *string `json:"name"`
	PublishedAt *string `json:"published_at"`
}

type userInfo struct {
	Login       string  `json:"login"`
	Name        *string `json:"name"`
	Bio         *string `json:"bio"`
	PublicRepos uint64  `json:"public_repos"`
	Followers   uint64  `json:"followers"`
	Following   uint64  `json:"following"`
	HTMLURL     string  `json:"html_url"`
}

type searchResult struct {
	Items []repoInfo `json:"items"`
}

func parseOwnerRepo(input string) (string, string, bool) {
	cleaned := strings.TrimPrefix(input, "https://github.com/")
	cleaned = strings.TrimPrefix(cleaned, "http://github.com/")
	cleaned = strings.TrimSuffix(cleaned, ".git")
	cleaned = strings.Trim(cleaned, "/")

	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func sendGitHubReq(client *http.Client, reqURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "WhatsRook/1.0")
	return client.Do(req)
}

func main() {
	req := sdk.Load()
	query := req.Query()

	if query == "" {
		p := req.EffectivePrefix()
		sdk.Respond(fmt.Sprintf(
			"*GitHub Plugin Usage:*\n\n"+
				"• `%sgit <owner/repo>` : Download repo as .zip archive\n"+
				"• `%sgit info <owner/repo>` : Repository metadata & metrics\n"+
				"• `%sgit commits <owner/repo>` : Recent commits\n"+
				"• `%sgit branches <owner/repo>` : List repository branches\n"+
				"• `%sgit releases <owner/repo>` : Release history & tags\n"+
				"• `%sgit user <username>` : User profile & statistics\n"+
				"• `%sgit search <query>` : Search GitHub repositories\n\n"+
				"*Example:* `%sgit Thruqe/whatsrook`",
			p, p, p, p, p, p, p, p,
		))
		return
	}

	client := sdk.CreateHTTPClient(30)
	sub := ""
	if len(req.Args) > 0 {
		sub = strings.ToLower(req.Args[0])
	}

	switch sub {
	case "info":
		if len(req.Args) < 2 {
			sdk.RespondErr(fmt.Sprintf("Usage: %sgit info <owner/repo>", req.EffectivePrefix()))
			return
		}
		target := req.Args[1]
		if owner, repo, ok := parseOwnerRepo(target); ok {
			apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
			resp, err := sendGitHubReq(client, apiURL)
			if err != nil {
				sdk.RespondErr(fmt.Sprintf("Network error: %s", err))
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var info repoInfo
				if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
					lang := "N/A"
					if info.Language != nil {
						lang = *info.Language
					}
					text := fmt.Sprintf(
						"*GitHub: %s*\n\n"+
							"⭐ *Stars:* %d\n"+
							"🍴 *Forks:* %d\n"+
							"❗ *Open Issues:* %d\n"+
							"🌿 *Default Branch:* %s\n"+
							"💻 *Language:* %s\n"+
							"🔗 *URL:* %s",
						info.FullName, info.StargazersCount, info.ForksCount,
						info.OpenIssuesCount, info.DefaultBranch, lang, info.HTMLURL,
					)
					if info.Description != nil && *info.Description != "" {
						text += fmt.Sprintf("\n\n*Description:*\n%s", *info.Description)
					}
					sdk.Respond(text)
					return
				}
			}
		}
		sdk.RespondErr(fmt.Sprintf("Could not fetch repository info for `%s`", target))

	case "commits":
		if len(req.Args) < 2 {
			sdk.RespondErr(fmt.Sprintf("Usage: %sgit commits <owner/repo>", req.EffectivePrefix()))
			return
		}
		target := req.Args[1]
		if owner, repo, ok := parseOwnerRepo(target); ok {
			apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?per_page=5", owner, repo)
			resp, err := sendGitHubReq(client, apiURL)
			if err != nil {
				sdk.RespondErr(fmt.Sprintf("Network error: %s", err))
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var commits []commitItem
				if err := json.NewDecoder(resp.Body).Decode(&commits); err == nil {
					if len(commits) == 0 {
						sdk.Respond("No commits found.")
						return
					}
					var sb strings.Builder
					fmt.Fprintf(&sb, "*Recent Commits (%s/%s):*\n\n", owner, repo)
					for _, c := range commits {
						shortSHA := c.SHA
						if len(shortSHA) > 7 {
							shortSHA = shortSHA[:7]
						}
						firstLine := strings.TrimSpace(strings.Split(c.Commit.Message, "\n")[0])
						authorInfo := "Unknown"
						if c.Commit.Author != nil {
							if c.Commit.Author.Date != "" {
								datePart, _, _ := strings.Cut(c.Commit.Author.Date, "T")
								authorInfo = fmt.Sprintf("%s [%s]", c.Commit.Author.Name, datePart)
							} else {
								authorInfo = c.Commit.Author.Name
							}
						}
						fmt.Fprintf(&sb, "• `%s` %s - _%s_\n", shortSHA, firstLine, authorInfo)
					}
					sdk.Respond(sb.String())
					return
				}
			}
		}
		sdk.RespondErr(fmt.Sprintf("Could not fetch commits for `%s`", target))

	case "branches":
		if len(req.Args) < 2 {
			sdk.RespondErr(fmt.Sprintf("Usage: %sgit branches <owner/repo>", req.EffectivePrefix()))
			return
		}
		target := req.Args[1]
		if owner, repo, ok := parseOwnerRepo(target); ok {
			apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?per_page=15", owner, repo)
			resp, err := sendGitHubReq(client, apiURL)
			if err != nil {
				sdk.RespondErr(fmt.Sprintf("Network error: %s", err))
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var branches []branchItem
				if err := json.NewDecoder(resp.Body).Decode(&branches); err == nil {
					if len(branches) == 0 {
						sdk.Respond("No branches found.")
						return
					}
					var names []string
					for _, b := range branches {
						names = append(names, fmt.Sprintf("• %s", b.Name))
					}
					sdk.Respond(fmt.Sprintf("*Branches (%s/%s):*\n\n%s", owner, repo, strings.Join(names, "\n")))
					return
				}
			}
		}
		sdk.RespondErr(fmt.Sprintf("Could not fetch branches for `%s`", target))

	case "releases":
		if len(req.Args) < 2 {
			sdk.RespondErr(fmt.Sprintf("Usage: %sgit releases <owner/repo>", req.EffectivePrefix()))
			return
		}
		target := req.Args[1]
		if owner, repo, ok := parseOwnerRepo(target); ok {
			apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=5", owner, repo)
			resp, err := sendGitHubReq(client, apiURL)
			if err != nil {
				sdk.RespondErr(fmt.Sprintf("Network error: %s", err))
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var releases []releaseItem
				if err := json.NewDecoder(resp.Body).Decode(&releases); err == nil {
					if len(releases) == 0 {
						sdk.Respond("No releases found.")
						return
					}
					var sb strings.Builder
					fmt.Fprintf(&sb, "*Releases (%s/%s):*\n\n", owner, repo)
					for _, r := range releases {
						name := r.TagName
						if r.Name != nil && *r.Name != "" {
							name = *r.Name
						}
						dateStr := ""
						if r.PublishedAt != nil {
							dateStr = strings.Split(*r.PublishedAt, "T")[0]
						}
						if dateStr != "" {
							fmt.Fprintf(&sb, "• *%s* (`%s`) - _%s_\n", name, r.TagName, dateStr)
						} else {
							fmt.Fprintf(&sb, "• *%s* (`%s`)\n", name, r.TagName)
						}
					}
					sdk.Respond(sb.String())
					return
				}
			}
		}
		sdk.RespondErr(fmt.Sprintf("Could not fetch releases for `%s`", target))

	case "user":
		if len(req.Args) < 2 {
			sdk.RespondErr(fmt.Sprintf("Usage: %sgit user <username>", req.EffectivePrefix()))
			return
		}
		username := req.Args[1]
		apiURL := fmt.Sprintf("https://api.github.com/users/%s", username)
		resp, err := sendGitHubReq(client, apiURL)
		if err != nil {
			sdk.RespondErr(fmt.Sprintf("Network error: %s", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var u userInfo
			if err := json.NewDecoder(resp.Body).Decode(&u); err == nil {
				name := "N/A"
				if u.Name != nil && *u.Name != "" {
					name = *u.Name
				}
				text := fmt.Sprintf(
					"*GitHub User: %s*\n\n"+
						"👤 *Name:* %s\n"+
						"📦 *Public Repos:* %d\n"+
						"👥 *Followers:* %d | *Following:* %d\n"+
						"🔗 *Profile:* %s",
					u.Login, name, u.PublicRepos, u.Followers, u.Following, u.HTMLURL,
				)
				if u.Bio != nil && *u.Bio != "" {
					text += fmt.Sprintf("\n\n*Bio:*\n%s", *u.Bio)
				}
				sdk.Respond(text)
				return
			}
		}
		sdk.RespondErr(fmt.Sprintf("Could not fetch user info for `%s`", username))

	case "search":
		if len(req.Args) < 2 {
			sdk.RespondErr(fmt.Sprintf("Usage: %sgit search <query>", req.EffectivePrefix()))
			return
		}
		searchTerm := strings.TrimSpace(req.RawArgs[len(req.Args[0]):])
		apiURL := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&per_page=5", url.QueryEscape(searchTerm))
		resp, err := sendGitHubReq(client, apiURL)
		if err != nil {
			sdk.RespondErr(fmt.Sprintf("Network error: %s", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var res searchResult
			if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
				if len(res.Items) == 0 {
					sdk.Respond(fmt.Sprintf("No repositories found for `%s`", searchTerm))
					return
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "*GitHub Search Results for `%s`:*\n\n", searchTerm)
				for _, item := range res.Items {
					desc := ""
					if item.Description != nil {
						desc = *item.Description
					}
					fmt.Fprintf(&sb, "• *%s* (⭐ %d)\n  _%s_\n  %s\n\n",
						item.FullName, item.StargazersCount, desc, item.HTMLURL)
				}
				sdk.Respond(strings.TrimSpace(sb.String()))
				return
			}
		}
		sdk.RespondErr(fmt.Sprintf("Search failed for `%s`", searchTerm))

	default:
		// Default action: download repository archive (.zip)
		target := query
		if sub == "download" || sub == "clone" {
			if len(req.Args) > 1 {
				target = req.Args[1]
			} else {
				target = ""
			}
		}

		if owner, repo, ok := parseOwnerRepo(target); ok {
			zipURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball", owner, repo)
			resp, err := sendGitHubReq(client, zipURL)
			if err != nil {
				sdk.RespondErr(fmt.Sprintf("Network error downloading repo: %s", err))
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				bytes, err := io.ReadAll(resp.Body)
				if err == nil && len(bytes) > 0 {
					dataURL := sdk.ToDataURL("application/zip", sdk.EncodeBase64(bytes))
					filename := fmt.Sprintf("%s-%s.zip", owner, repo)
					caption := fmt.Sprintf("📦 Repository: %s/%s", owner, repo)
					sdk.SendDocument(dataURL, filename, caption)
					return
				}
			}
		}

		sdk.RespondErr(fmt.Sprintf(
			"Invalid repository format. Please specify `<owner/repo>` (e.g. `%sgit Thruqe/whatsrook`)",
			req.EffectivePrefix(),
		))
	}
}
