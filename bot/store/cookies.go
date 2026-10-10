package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/thruqe/duosql"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

// PutPlatformCookie saves or updates Netscape authentication cookies for media extraction sites.
func PutPlatformCookie(ctx context.Context, s *sqlstore.SQLStore, platform, domain, cookies string) error {
	if s == nil {
		return nil
	}
	ourJID := ourJIDStr(s)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		platform = "generic"
	}

	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}

	item := &BotPlatformCookie{
		OurJID:   ourJID,
		Platform: platform,
		Domain:   domain,
		Cookies:  cookies,
	}

	_, err = duosql.Insert[BotPlatformCookie](duo).
		Values(item).
		OnConflictDoUpdateAll("our_jid", "platform").
		Exec(ctx)
	return err
}

// GetPlatformCookie retrieves active cookie text for a specific streaming or content platform.
func GetPlatformCookie(ctx context.Context, s *sqlstore.SQLStore, platform string) (string, error) {
	if s == nil {
		return "", nil
	}
	ourJID := ourJIDStr(s)
	platform = strings.ToLower(strings.TrimSpace(platform))

	duo, err := getDuoDB(s)
	if err != nil {
		return "", err
	}

	orderExpr := fmt.Sprintf(
		"CASE WHEN our_jid = %s THEN 1 WHEN our_jid = %s THEN 2 ELSE 3 END",
		sqlQuoteLiteral(ourJID), sqlQuoteLiteral(s.JID),
	)

	cookie, err := duosql.Select[BotPlatformCookie](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("platform", platform),
		).
		OrderBy(duosql.RawOrderBy(orderExpr)).
		Limit(1).
		One(ctx)
	if err != nil {
		return "", err
	}
	return cookie.Cookies, nil
}

// DeletePlatformCookie purges stored authentication cookies for a platform.
func DeletePlatformCookie(ctx context.Context, s *sqlstore.SQLStore, platform string) error {
	if s == nil {
		return nil
	}
	ourJID := ourJIDStr(s)
	platform = strings.ToLower(strings.TrimSpace(platform))

	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}

	_, err = duosql.Delete[BotPlatformCookie](duo).
		Where(
			duosql.Or(duosql.Eq("our_jid", ourJID), duosql.Eq("our_jid", s.JID)),
			duosql.Eq("platform", platform),
		).
		Exec(ctx)
	return err
}

// DeleteAllPlatformCookies purges all stored media platform cookies for the current bot session.
func DeleteAllPlatformCookies(ctx context.Context, s *sqlstore.SQLStore) error {
	if s == nil {
		return nil
	}
	ourJID := ourJIDStr(s)

	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}

	_, err = duosql.Delete[BotPlatformCookie](duo).
		Where(duosql.Or(duosql.Eq("our_jid", ourJID), duosql.Eq("our_jid", s.JID))).
		Exec(ctx)
	return err
}

// ListPlatformCookies returns unique cookie entries across all configured platforms.
func ListPlatformCookies(ctx context.Context, s *sqlstore.SQLStore) ([]BotPlatformCookie, error) {
	if s == nil {
		return nil, nil
	}
	ourJID := ourJIDStr(s)

	duo, err := getDuoDB(s)
	if err != nil {
		return nil, err
	}

	items, err := duosql.Select[BotPlatformCookie](duo).
		Where(duosql.Or(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("our_jid", s.JID),
			duosql.Eq("our_jid", ""),
			duosql.IsNull("our_jid"),
		)).
		OrderBy(duosql.Asc("platform")).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]BotPlatformCookie, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if !seen[item.Platform] {
			seen[item.Platform] = true
			result = append(result, item)
		}
	}
	return result, nil
}

// GetAllPlatformCookiesMerged concatenates all platform cookies into Netscape cookie format.
func GetAllPlatformCookiesMerged(ctx context.Context, s *sqlstore.SQLStore) (string, error) {
	cookiesList, err := ListPlatformCookies(ctx, s)
	if err != nil || len(cookiesList) == 0 {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Netscape HTTP Cookie File\n# Merged yt-dlp cookies for all configured platforms\n\n")
	for _, pc := range cookiesList {
		lines := strings.SplitSeq(pc.Cookies, "\n")
		for line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "# Netscape HTTP Cookie") {
				continue
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String(), nil
}
