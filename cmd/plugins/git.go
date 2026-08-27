package plugins

import (
	"errors"
	"slices"
	"strings"
	"time"

	cliutils "whatsrook/cmd/utils"
	utils "whatsrook/src"
	Logger "whatsrook/src/logger"
)

func init() {
	Register(&Command{
		Name:        "git",
		Alias:       "github,gh,gitclone",
		Description: "Download full repository archives (.zip/.tar.gz), select branches, authenticate GitHub, and perform Git actions.",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleGit,
	})
}

func getUserGitToken(ctx *Context) string {
	s, ok := getStore(ctx)
	if !ok || s == nil {
		return ""
	}
	userJID := ctx.Sender.ToNonAD().String()
	if userJID == "" {
		userJID = ctx.Sender.String()
	}
	token, _ := s.GetSetting(ctx.Ctx, "git_token:"+userJID)
	return strings.TrimSpace(token)
}

func setUserGitToken(ctx *Context, token, username string) error {
	s, ok := getStore(ctx)
	if !ok || s == nil {
		return errors.New("database store unavailable")
	}
	userJID := ctx.Sender.ToNonAD().String()
	if userJID == "" {
		userJID = ctx.Sender.String()
	}
	if err := s.PutSetting(ctx.Ctx, "git_token:"+userJID, token); err != nil {
		return err
	}
	if username != "" {
		_ = s.PutSetting(ctx.Ctx, "git_user:"+userJID, username)
	}
	return nil
}

func deleteUserGitToken(ctx *Context) error {
	s, ok := getStore(ctx)
	if !ok || s == nil {
		return errors.New("database store unavailable")
	}
	userJID := ctx.Sender.ToNonAD().String()
	if userJID == "" {
		userJID = ctx.Sender.String()
	}
	_ = s.DeleteSetting(ctx.Ctx, "git_token:"+userJID)
	_ = s.DeleteSetting(ctx.Ctx, "git_user:"+userJID)
	return nil
}

func handleGit(ctx *Context) error {
	args := ctx.Args
	if len(args) == 0 {
		return showGitHelp(ctx)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "help":
		return showGitHelp(ctx)
	case "auth", "login", "token":
		return handleGitAuth(ctx)
	case "whoami", "profile", "status", "me":
		return handleGitWhoami(ctx)
	case "logout", "unauth", "unlink":
		return handleGitLogout(ctx)
	case "zip", "tar", "targz", "clone", "download", "dl":
		return handleGitDownload(ctx, sub)
	case "info", "repo", "view", "stats":
		return handleGitInfo(ctx)
	case "branches", "branch":
		return handleGitBranches(ctx)
	case "commits", "commit", "log":
		return handleGitCommits(ctx)
	case "releases", "release", "tags", "tag":
		return handleGitReleases(ctx)
	case "tree", "files", "ls", "dir":
		return handleGitTree(ctx)
	case "file", "cat", "read", "raw":
		return handleGitFile(ctx)
	case "search", "find":
		return handleGitSearch(ctx)
	case "user", "userinfo":
		return handleGitUser(ctx)
	case "repos", "myrepos", "list":
		return handleGitRepos(ctx)
	case "star":
		return handleGitStar(ctx, true)
	case "unstar":
		return handleGitStar(ctx, false)
	case "fork":
		return handleGitFork(ctx)
	case "create", "new":
		return handleGitCreate(ctx)
	case "delete", "rm":
		return handleGitDelete(ctx)
	case "issues", "issue":
		return handleGitIssues(ctx)
	default:
		if strings.Contains(args[0], "/") || strings.HasPrefix(args[0], "http") || strings.HasPrefix(args[0], "git@") {
			return handleGitDownload(ctx, "download")
		}
		return showGitHelp(ctx)
	}
}

func showGitHelp(ctx *Context) error {
	p := ctx.GetPrefix()
	tb := ctx.NewText().
		Line(Bold("WhatsRook Git & GitHub Suite")).
		Line(Italic("Download repos as ZIP/TAR.GZ, manage branches, and perform GitHub actions.")).
		NewLine().
		Line(Bold("Download & Archives:")).
		Linef("• %sgit zip <owner/repo> [branch] - Download repository as .zip", p).
		Linef("• %sgit tar <owner/repo> [branch] - Download repository as .tar.gz", p).
		Linef("• %sgit clone <owner/repo> [branch] - Clone/download archive", p).
		Linef("• %sgit download <owner/repo> - Interactive branch & format picker", p).
		NewLine().
		Line(Bold("Authentication:")).
		Linef("• %sgit login <token> - Link personal access token (PAT)", p).
		Linef("• %sgit whoami - View logged in profile and token status", p).
		Linef("• %sgit logout - Unlink your GitHub token", p).
		NewLine().
		Line(Bold("Explore & Search:")).
		Linef("• %sgit info <owner/repo> - Repository details, stars & stats", p).
		Linef("• %sgit branches <owner/repo> - List branches", p).
		Linef("• %sgit commits <owner/repo> [branch] - Recent commit history", p).
		Linef("• %sgit releases <owner/repo> - Releases and asset downloads", p).
		Linef("• %sgit tree <owner/repo> [path] [branch] - Explore directory tree", p).
		Linef("• %sgit file <owner/repo> <path> [branch] - View file contents", p).
		Linef("• %sgit search <query> - Search GitHub repositories", p).
		Linef("• %sgit user <username> - GitHub profile details", p).
		Linef("• %sgit repos [username] - List user repositories", p).
		Linef("• %sgit issues <owner/repo> - List open issues", p).
		NewLine().
		Line(Bold("GitHub Actions (Authenticated):")).
		Linef("• %sgit star <owner/repo> / unstar - Star/unstar a repository", p).
		Linef("• %sgit fork <owner/repo> - Fork repo to your account", p).
		Linef("• %sgit create <name> [--private] [desc] - Create repository", p).
		Linef("• %sgit delete <owner/repo> - Delete your repository", p).
		Linef("• %sgit issue <owner/repo> create <title> | <body> - Open issue", p)

	return ctx.Reply(tb.String())
}

func handleGitAuth(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please provide your GitHub Personal Access Token (PAT).\n\nUsage: %sgit login <token>\nExample: %sgit login ghp_xxxxxxxxxxxx", p, p)
	}

	token := strings.TrimSpace(args[1])
	ctx.StartAutoLoader()

	user, scopes, err := cliutils.FetchGitHubUser(ctx.Ctx, token, "")
	if err != nil {
		return ctx.ReplyErrorf("GitHub authentication failed: %v", err)
	}

	if err := setUserGitToken(ctx, token, user.Login); err != nil {
		return ctx.ReplyErrorf("Failed saving token to database: %v", err)
	}

	tb := ctx.NewText().
		Line(Bold("GitHub Authentication Successful")).
		NewLine().
		Linef("• "+Bold("Username:")+" @%s", user.Login)

	if user.Name != "" {
		tb.Linef("• "+Bold("Name:")+" %s", user.Name)
	}
	if user.Email != "" {
		tb.Linef("• "+Bold("Email:")+" %s", user.Email)
	}
	if scopes != "" {
		tb.Linef("• "+Bold("Token Scopes:")+" %s", scopes)
	}
	tb.Linef("• "+Bold("Public Repos:")+" %d", user.PublicRepos)
	if user.TotalPrivateRepos > 0 {
		tb.Linef("• "+Bold("Private Repos:")+" %d", user.TotalPrivateRepos)
	}
	tb.NewLine().
		Line(Italic("Your token is securely stored and will be used for private repo downloads, stars, forks, and actions."))

	return ctx.Reply(tb.String())
}

func handleGitWhoami(ctx *Context) error {
	p := ctx.GetPrefix()
	token := getUserGitToken(ctx)
	if token == "" {
		return ctx.Replyf("You are currently not logged in to GitHub.\n\nAuthenticate with your token to access private repositories, star/fork repos, and bypass rate limits:\n%sgit login <token>", p)
	}

	ctx.StartAutoLoader()
	user, scopes, err := cliutils.FetchGitHubUser(ctx.Ctx, token, "")
	if err != nil {
		return ctx.ReplyErrorf("Failed retrieving profile: %v\n\nYour token may have expired. Try logging in again with `%sgit login <token>`.", err, p)
	}

	tb := ctx.NewText().
		Line(Bold("GitHub Authenticated Profile")).
		NewLine().
		Linef("• "+Bold("Username:")+" @%s", user.Login)

	if user.Name != "" {
		tb.Linef("• "+Bold("Name:")+" %s", user.Name)
	}
	if user.Bio != "" {
		tb.Linef("• "+Bold("Bio:")+" %s", user.Bio)
	}
	if user.Company != "" {
		tb.Linef("• "+Bold("Company:")+" %s", user.Company)
	}
	if user.Location != "" {
		tb.Linef("• "+Bold("Location:")+" %s", user.Location)
	}
	if user.Email != "" {
		tb.Linef("• "+Bold("Email:")+" %s", user.Email)
	}
	tb.Linef("• "+Bold("Public Repos:")+" %d", user.PublicRepos)
	if user.TotalPrivateRepos > 0 {
		tb.Linef("• "+Bold("Private Repos:")+" %d", user.TotalPrivateRepos)
	}
	tb.Linef("• "+Bold("Followers:")+" %d | "+Bold("Following:")+" %d", user.Followers, user.Following)
	if scopes != "" {
		tb.Linef("• "+Bold("OAuth Scopes:")+" %s", scopes)
	}
	if user.HTMLURL != "" {
		tb.Linef("• "+Bold("URL:")+" %s", user.HTMLURL)
	}

	return ctx.Reply(tb.String())
}

func handleGitLogout(ctx *Context) error {
	if err := deleteUserGitToken(ctx); err != nil {
		return ctx.ReplyErrorf("Failed logging out: %v", err)
	}
	return ctx.Reply("GitHub token unlinked successfully. You are now in anonymous mode.")
}

func handleGitDownload(ctx *Context, mode string) error {
	p := ctx.GetPrefix()
	args := ctx.Args

	targetArg := ""
	branchArg := ""
	format := "zip"

	if strings.ToLower(mode) == "tar" || strings.ToLower(mode) == "targz" {
		format = "tar.gz"
	}

	if len(args) >= 2 && (strings.EqualFold(args[0], "zip") || strings.EqualFold(args[0], "tar") || strings.EqualFold(args[0], "targz") || strings.EqualFold(args[0], "clone") || strings.EqualFold(args[0], "download") || strings.EqualFold(args[0], "dl")) {
		targetArg = args[1]
		if len(args) >= 3 {
			branchArg = args[2]
		}
		if len(args) >= 4 {
			f := strings.ToLower(args[3])
			if f == "tar" || f == "tar.gz" || f == "tgz" {
				format = "tar.gz"
			} else if f == "zip" {
				format = "zip"
			}
		}
	} else if len(args) >= 1 {
		targetArg = args[0]
		if len(args) >= 2 {
			branchArg = args[1]
		}
	}

	if targetArg == "" {
		return ctx.Replyf("Please specify the repository to download.\n\nUsage:\n• %sgit zip <owner/repo> [branch]\n• %sgit tar <owner/repo> [branch]\n• %sgit download <owner/repo>", p, p, p)
	}

	target, err := cliutils.ParseGitTarget(targetArg)
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}
	target.Format = format
	if branchArg != "" {
		target.Branch = branchArg
	}

	token := getUserGitToken(ctx)

	if target.Branch == "" && (mode == "download" || mode == "dl") {
		branches, err := cliutils.FetchGitHubBranches(ctx.Ctx, token, target.Owner, target.Repo, 6)
		if err == nil && len(branches) > 0 {
			poll := ctx.Rook().NewPoll(Sprintf("Download %s/%s:", target.Owner, target.Repo))
			for i, b := range branches {
				if i >= 4 {
					break
				}
				poll.AddOption(Sprintf("%s (ZIP)", b.Name))
				poll.AddOption(Sprintf("%s (TAR.GZ)", b.Name))
			}

			_ = ctx.Reply(Sprintf("Select branch and archive format for %s/%s from the poll below:", target.Owner, target.Repo))
			return poll.Reply(func(req utils.PollRequest, res *utils.Response) {
				if len(req.SelectedOptions) > 0 {
					opt := req.SelectedOptions[0]
					selectedBranch := strings.Fields(opt)[0]
					selectedFormat := "zip"
					if strings.Contains(opt, "TAR") {
						selectedFormat = "tar.gz"
					}

					pollTarget := &cliutils.GitRepoTarget{
						Host:     target.Host,
						Owner:    target.Owner,
						Repo:     target.Repo,
						FullName: target.FullName,
						Branch:   selectedBranch,
						Format:   selectedFormat,
					}

					_ = res.Reply(Sprintf("Preparing %s (%s) archive for %s/%s...", selectedBranch, selectedFormat, target.Owner, target.Repo))
					data, filename, mime, err := cliutils.DownloadRepoArchive(req.Ctx, token, pollTarget)
					if err != nil {
						_ = res.Reply(Sprintf("Failed to download repository: %v", err))
						return
					}

					caption := Sprintf("Repository: %s/%s\nBranch: %s\nSize: %s\nFormat: %s",
						target.Owner, target.Repo, selectedBranch, cliutils.FormatBytes(uint64(len(data))), strings.ToUpper(selectedFormat))

					_ = res.Document(data, filename, mime)
					_ = res.Reply(caption)
				}
			})
		}
	}

	ctx.StartAutoLoader(800 * time.Millisecond)

	data, filename, mime, err := cliutils.DownloadRepoArchive(ctx.Ctx, token, target)
	if err != nil {
		return ctx.ReplyErrorf("Failed to download repository: %v", err)
	}

	usedBranch := target.Branch
	if usedBranch == "" {
		usedBranch = "default"
	}

	caption := Sprintf("Repository: %s/%s\nBranch: %s\nSize: %s\nFormat: %s",
		target.Owner, target.Repo, usedBranch, cliutils.FormatBytes(uint64(len(data))), strings.ToUpper(target.Format))

	if err := ctx.ReplyWithDocument(data, mime, filename, caption); err != nil {
		Logger.Error("failed sending git archive document", "err", err)
		return ctx.ReplyErrorf("Failed sending document: %v", err)
	}
	return nil
}

func handleGitInfo(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify a repository.\nUsage: %sgit info <owner/repo>", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	repo, err := cliutils.FetchGitHubRepo(ctx.Ctx, token, target.Owner, target.Repo)
	if err != nil {
		return ctx.ReplyErrorf("Failed retrieving repo info: %v", err)
	}

	tb := ctx.NewText().
		Linef("%s", Bold(repo.FullName)).
		NewLine()

	if repo.Description != "" {
		tb.Line(repo.Description).NewLine()
	}

	tb.Linef("• "+Bold("Default Branch:")+" %s", repo.DefaultBranch).
		Linef("• "+Bold("Stars:")+" %s", cliutils.FormatNumberWithCommas(int64(repo.Stars))).
		Linef("• "+Bold("Forks:")+" %s", cliutils.FormatNumberWithCommas(int64(repo.Forks))).
		Linef("• "+Bold("Watchers:")+" %s", cliutils.FormatNumberWithCommas(int64(repo.Watchers))).
		Linef("• "+Bold("Open Issues:")+" %s", cliutils.FormatNumberWithCommas(int64(repo.OpenIssues)))

	if repo.Language != "" {
		tb.Linef("• "+Bold("Language:")+" %s", repo.Language)
	}
	if repo.License != nil && repo.License.SpdxID != "" {
		tb.Linef("• "+Bold("License:")+" %s", repo.License.SpdxID)
	}
	if repo.Size > 0 {
		tb.Linef("• "+Bold("Repo Size:")+" %s", cliutils.FormatBytes(uint64(repo.Size*1024)))
	}
	if len(repo.Topics) > 0 {
		tb.Linef("• "+Bold("Topics:")+" %s", strings.Join(repo.Topics, ", "))
	}
	if repo.UpdatedAt != "" {
		tb.Linef("• "+Bold("Last Updated:")+" %s", repo.UpdatedAt)
	}
	tb.Linef("• "+Bold("URL:")+" %s", repo.HTMLURL)

	return ctx.Reply(tb.String())
}

func handleGitBranches(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify a repository.\nUsage: %sgit branches <owner/repo>", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	branches, err := cliutils.FetchGitHubBranches(ctx.Ctx, token, target.Owner, target.Repo, 20)
	if err != nil {
		return ctx.ReplyErrorf("Failed listing branches: %v", err)
	}

	if len(branches) == 0 {
		return ctx.Reply("No branches found in this repository.")
	}

	tb := ctx.NewText().
		Linef("%s (Branches: %d)", Bold(target.FullName), len(branches)).
		NewLine()

	for _, b := range branches {
		sha := b.Commit.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		tb.Linef("• %s (%s)", Bold(b.Name), Code(sha))
	}

	return ctx.Reply(tb.String())
}

func handleGitCommits(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify a repository.\nUsage: %sgit commits <owner/repo> [branch]", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	branch := target.Branch
	if len(args) >= 3 {
		branch = args[2]
	}

	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	commits, err := cliutils.FetchGitHubCommits(ctx.Ctx, token, target.Owner, target.Repo, branch, 8)
	if err != nil {
		return ctx.ReplyErrorf("Failed fetching commits: %v", err)
	}

	if len(commits) == 0 {
		return ctx.Reply("No commits found.")
	}

	tb := ctx.NewText().
		Linef("%s (Recent Commits)", Bold(target.FullName)).
		NewLine()

	for _, c := range commits {
		sha := c.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		msg, _, _ := strings.Cut(c.Commit.Message, "\n")
		author := c.Commit.Author.Name
		date := c.Commit.Author.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		tb.Linef("• %s - %s", Code(sha), msg)
		tb.Linef("  ↳ %s on %s", author, date)
	}

	return ctx.Reply(tb.String())
}

func handleGitReleases(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify a repository.\nUsage: %sgit releases <owner/repo>", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	releases, err := cliutils.FetchGitHubReleases(ctx.Ctx, token, target.Owner, target.Repo, 5)
	if err != nil {
		return ctx.ReplyErrorf("Failed fetching releases: %v", err)
	}

	if len(releases) == 0 {
		return ctx.Reply("No releases found for this repository.")
	}

	tb := ctx.NewText().
		Linef("%s (Latest Releases)", Bold(target.FullName)).
		NewLine()

	for _, rel := range releases {
		name := rel.Name
		if name == "" {
			name = rel.TagName
		}
		tb.Linef("• %s (%s)", Bold(name), rel.TagName)
		if rel.PublishedAt != "" {
			date := rel.PublishedAt
			if len(date) >= 10 {
				date = date[:10]
			}
			tb.Linef("  ↳ Published: %s", date)
		}
		if len(rel.Assets) > 0 {
			tb.Linef("  ↳ Assets (%d):", len(rel.Assets))
			for _, asset := range rel.Assets {
				tb.Linef("    - %s (%s, %s downloads)", asset.Name, cliutils.FormatBytes(uint64(asset.Size)), cliutils.FormatNumberWithCommas(int64(asset.DownloadCount)))
			}
		}
		tb.NewLine()
	}

	return ctx.Reply(tb.String())
}

func handleGitTree(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify a repository.\nUsage: %sgit tree <owner/repo> [path] [branch]", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	path := ""
	branch := target.Branch
	if len(args) >= 3 {
		path = args[2]
	}
	if len(args) >= 4 {
		branch = args[3]
	}

	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	list, single, err := cliutils.FetchGitHubContents(ctx.Ctx, token, target.Owner, target.Repo, path, branch)
	if err != nil {
		return ctx.ReplyErrorf("Failed fetching directory tree: %v", err)
	}

	if single != nil {
		return ctx.Replyf("%s (%s, %s)", single.Path, single.Type, cliutils.FormatBytes(uint64(single.Size)))
	}

	if len(list) == 0 {
		return ctx.Reply("Directory is empty.")
	}

	displayPath := "/"
	if path != "" {
		displayPath = "/" + strings.TrimPrefix(path, "/")
	}

	tb := ctx.NewText().
		Linef("%s:%s", Bold(target.FullName), displayPath).
		NewLine()

	for _, item := range list {
		if item.Type == "dir" {
			tb.Linef("• %s/ (dir)", Bold(item.Name))
		} else {
			tb.Linef("• %s (%s)", item.Name, cliutils.FormatBytes(uint64(item.Size)))
		}
	}

	return ctx.Reply(tb.String())
}

func handleGitFile(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 3 {
		return ctx.Replyf("Please specify repository and file path.\nUsage: %sgit file <owner/repo> <filepath> [branch]", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	filePath := args[2]
	branch := target.Branch
	if len(args) >= 4 {
		branch = args[3]
	}

	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	_, fileItem, err := cliutils.FetchGitHubContents(ctx.Ctx, token, target.Owner, target.Repo, filePath, branch)
	if err != nil || fileItem == nil {
		return ctx.ReplyErrorf("Failed fetching file %q: %v", filePath, err)
	}

	data, err := cliutils.DecodeBase64Content(fileItem.Content)
	if err != nil {
		return ctx.ReplyErrorf("Failed decoding file content: %v", err)
	}

	if len(data) <= 2500 && isUTF8Text(data) {
		tb := ctx.NewText().
			Linef("%s (%s)", Bold(fileItem.Path), cliutils.FormatBytes(uint64(fileItem.Size))).
			NewLine().
			Line(CodeBlock(string(data)))
		return ctx.Reply(tb.String())
	}

	caption := Sprintf("%s (%s)", fileItem.Path, cliutils.FormatBytes(uint64(fileItem.Size)))
	return ctx.ReplyWithDocument(data, "application/octet-stream", fileItem.Name, caption)
}

func isUTF8Text(b []byte) bool {
	return !slices.Contains(b, 0)
}

func handleGitSearch(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify search query.\nUsage: %sgit search <query>", p)
	}

	query := strings.Join(args[1:], " ")
	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	repos, err := cliutils.SearchGitHubRepos(ctx.Ctx, token, query, 6)
	if err != nil {
		return ctx.ReplyErrorf("Search failed: %v", err)
	}

	if len(repos) == 0 {
		return ctx.Replyf("No repositories matching %q found.", query)
	}

	tb := ctx.NewText().
		Linef("GitHub Search: %s", Bold(query)).
		NewLine()

	for _, r := range repos {
		tb.Linef("• %s (Stars: %s)", Bold(r.FullName), cliutils.FormatNumberWithCommas(int64(r.Stars)))
		if r.Description != "" {
			desc := r.Description
			if len(desc) > 100 {
				desc = desc[:97] + "..."
			}
			tb.Linef("  ↳ %s", desc)
		}
		if r.Language != "" {
			tb.Linef("  ↳ Language: %s", r.Language)
		}
		tb.NewLine()
	}

	return ctx.Reply(tb.String())
}

func handleGitUser(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify GitHub username.\nUsage: %sgit user <username>", p)
	}

	username := strings.TrimPrefix(args[1], "@")
	token := getUserGitToken(ctx)
	ctx.StartAutoLoader()

	user, _, err := cliutils.FetchGitHubUser(ctx.Ctx, token, username)
	if err != nil {
		return ctx.ReplyErrorf("User lookup failed: %v", err)
	}

	tb := ctx.NewText().
		Linef("%s (@%s)", Bold(user.Name), user.Login).
		NewLine()

	if user.Bio != "" {
		tb.Line(user.Bio).NewLine()
	}
	if user.Company != "" {
		tb.Linef("• "+Bold("Company:")+" %s", user.Company)
	}
	if user.Location != "" {
		tb.Linef("• "+Bold("Location:")+" %s", user.Location)
	}
	if user.Blog != "" {
		tb.Linef("• "+Bold("Website:")+" %s", user.Blog)
	}
	tb.Linef("• "+Bold("Public Repos:")+" %d", user.PublicRepos).
		Linef("• "+Bold("Followers:")+" %d | "+Bold("Following:")+" %d", user.Followers, user.Following)
	if user.CreatedAt != "" {
		date := user.CreatedAt
		if len(date) >= 10 {
			date = date[:10]
		}
		tb.Linef("• "+Bold("Joined:")+" %s", date)
	}
	tb.Linef("• "+Bold("URL:")+" %s", user.HTMLURL)

	return ctx.Reply(tb.String())
}

func handleGitRepos(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	token := getUserGitToken(ctx)

	targetUser := ""
	if len(args) >= 2 {
		targetUser = strings.TrimPrefix(args[1], "@")
	}

	if targetUser == "" && token == "" {
		return ctx.Replyf("Please specify a username or log in.\nUsage: %sgit repos <username>\nOr authenticate with: %sgit login <token>", p, p)
	}

	ctx.StartAutoLoader()
	repos, err := cliutils.FetchUserRepos(ctx.Ctx, token, targetUser, 12)
	if err != nil {
		return ctx.ReplyErrorf("Failed listing repos: %v", err)
	}

	if len(repos) == 0 {
		return ctx.Reply("No repositories found.")
	}

	title := "Your Repositories"
	if targetUser != "" {
		title = Sprintf("@%s's Repositories", targetUser)
	}

	tb := ctx.NewText().
		Linef("%s (%d)", Bold(title), len(repos)).
		NewLine()

	for _, r := range repos {
		visibility := "Public"
		if r.Private {
			visibility = "Private"
		}
		tb.Linef("• %s (%s, Stars: %d)", Bold(r.Name), visibility, r.Stars)
		if r.Description != "" {
			desc := r.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			tb.Linef("  ↳ %s", desc)
		}
	}

	return ctx.Reply(tb.String())
}

func handleGitStar(ctx *Context, star bool) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		action := "star"
		if !star {
			action = "unstar"
		}
		return ctx.Replyf("Please specify repository to %s.\nUsage: %sgit %s <owner/repo>", action, p, action)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)
	if token == "" {
		return ctx.Replyf("Authentication required to star/unstar repositories.\nPlease log in first with `%sgit login <token>`.", p)
	}

	ctx.StartAutoLoader()
	if star {
		if err := cliutils.StarGitHubRepo(ctx.Ctx, token, target.Owner, target.Repo); err != nil {
			return ctx.ReplyErrorf("Failed starring repo: %v", err)
		}
		return ctx.Replyf("Starred %s successfully.", Bold(target.FullName))
	}

	if err := cliutils.UnstarGitHubRepo(ctx.Ctx, token, target.Owner, target.Repo); err != nil {
		return ctx.ReplyErrorf("Failed unstarring repo: %v", err)
	}
	return ctx.Replyf("Removed star from %s.", Bold(target.FullName))
}

func handleGitFork(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify repository to fork.\nUsage: %sgit fork <owner/repo>", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)
	if token == "" {
		return ctx.Replyf("Authentication required to fork repositories.\nPlease log in first with `%sgit login <token>`.", p)
	}

	ctx.StartAutoLoader()
	forked, err := cliutils.ForkGitHubRepo(ctx.Ctx, token, target.Owner, target.Repo)
	if err != nil {
		return ctx.ReplyErrorf("Failed forking repo: %v", err)
	}

	return ctx.Replyf("Forked %s to %s.\nURL: %s", Bold(target.FullName), Bold(forked.FullName), forked.HTMLURL)
}

func handleGitCreate(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify repository name.\nUsage: %sgit create <name> [--private] [description]", p)
	}

	name := args[1]
	isPrivate := false
	var descParts []string

	for _, a := range args[2:] {
		if a == "--private" || a == "-p" {
			isPrivate = true
		} else {
			descParts = append(descParts, a)
		}
	}
	description := strings.Join(descParts, " ")

	token := getUserGitToken(ctx)
	if token == "" {
		return ctx.Replyf("Authentication required to create repositories.\nPlease log in first with `%sgit login <token>`.", p)
	}

	ctx.StartAutoLoader()
	repo, err := cliutils.CreateGitHubRepo(ctx.Ctx, token, name, description, isPrivate, true)
	if err != nil {
		return ctx.ReplyErrorf("Failed creating repository: %v", err)
	}

	visibility := "Public"
	if repo.Private {
		visibility = "Private"
	}

	return ctx.Replyf("Repository created successfully.\n\n• "+Bold("Name:")+" %s (%s)\n• "+Bold("URL:")+" %s", repo.FullName, visibility, repo.HTMLURL)
}

func handleGitDelete(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify repository to delete.\nUsage: %sgit delete <owner/repo>", p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)
	if token == "" {
		return ctx.Replyf("Authentication required to delete repositories.\nPlease log in first with `%sgit login <token>`.", p)
	}

	ctx.StartAutoLoader()
	if err := cliutils.DeleteGitHubRepo(ctx.Ctx, token, target.Owner, target.Repo); err != nil {
		return ctx.ReplyErrorf("Failed deleting repository: %v", err)
	}

	return ctx.Replyf("Repository %s was permanently deleted.", Bold(target.FullName))
}

func handleGitIssues(ctx *Context) error {
	p := ctx.GetPrefix()
	args := ctx.Args
	if len(args) < 2 {
		return ctx.Replyf("Please specify a repository.\nUsage:\n• %sgit issues <owner/repo>\n• %sgit issue <owner/repo> create <title> | <body>", p, p)
	}

	target, err := cliutils.ParseGitTarget(args[1])
	if err != nil {
		return ctx.ReplyErrorf("Invalid repository: %v", err)
	}

	token := getUserGitToken(ctx)

	if len(args) >= 4 && strings.EqualFold(args[2], "create") {
		if token == "" {
			return ctx.Replyf("Authentication required to open issues.\nPlease log in first with `%sgit login <token>`.", p)
		}

		rawIssue := strings.Join(args[3:], " ")
		parts := strings.Split(rawIssue, "|")
		title := strings.TrimSpace(parts[0])
		body := ""
		if len(parts) > 1 {
			body = strings.TrimSpace(parts[1])
		}

		if title == "" {
			return ctx.Replyf("Please specify an issue title.\nUsage: %sgit issue <owner/repo> create <title> | [body]", p)
		}

		ctx.StartAutoLoader()
		issue, err := cliutils.CreateGitHubIssue(ctx.Ctx, token, target.Owner, target.Repo, title, body)
		if err != nil {
			return ctx.ReplyErrorf("Failed opening issue: %v", err)
		}

		return ctx.Replyf("Issue #%d created successfully.\n\n• "+Bold("Title:")+" %s\n• "+Bold("URL:")+" %s", issue.Number, issue.Title, issue.HTMLURL)
	}

	ctx.StartAutoLoader()
	issues, err := cliutils.FetchGitHubIssues(ctx.Ctx, token, target.Owner, target.Repo, 8)
	if err != nil {
		return ctx.ReplyErrorf("Failed fetching issues: %v", err)
	}

	if len(issues) == 0 {
		return ctx.Replyf("No open issues found for %s.", Bold(target.FullName))
	}

	tb := ctx.NewText().
		Linef("%s (Open Issues)", Bold(target.FullName)).
		NewLine()

	for _, issue := range issues {
		tb.Linef("• #%d: %s (@%s, %d comments)", issue.Number, Bold(issue.Title), issue.User.Login, issue.Comments)
	}

	return ctx.Reply(tb.String())
}
