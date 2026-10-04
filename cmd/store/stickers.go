package store

import (
	"context"
	"database/sql"
	"errors"

	"whatsrook/util/cache"

	"github.com/thruqe/duosql"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

// GetStickerCmd returns the command name bound to a sticker file's SHA-256 hash.
func GetStickerCmd(ctx context.Context, s *sqlstore.SQLStore, shaHex string) (string, error) {
	if s == nil {
		return "", nil
	}
	ourJID := ourJIDStr(s)
	cacheKey := stickerCacheKey(ourJID, shaHex)

	if val, ok, _ := cache.Get(ctx, cacheKey); ok {
		if val == cacheSentinelNil {
			return "", nil
		}
		return val, nil
	}

	duo, err := getDuoDB(s)
	if err != nil {
		return "", err
	}

	cmd, err := duosql.Select[BotStickerCmd](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("sticker_sha256", shaHex),
		).
		Limit(1).
		One(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		_ = cache.Set(ctx, cacheKey, cacheSentinelNil, filterNegativeCacheTTL)
		return "", nil
	}
	if err != nil {
		return "", err
	}

	_ = cache.Set(ctx, cacheKey, cmd.CommandName, filterCacheTTL)
	return cmd.CommandName, nil
}

// PutStickerCmd associates a sticker SHA-256 hash with a bot command name.
func PutStickerCmd(ctx context.Context, s *sqlstore.SQLStore, shaHex, cmdName string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	cacheKey := stickerCacheKey(ourJID, shaHex)
	_ = cache.Set(ctx, cacheKey, cmdName, filterCacheTTL)

	item := &BotStickerCmd{
		OurJID:        ourJID,
		StickerSHA256: shaHex,
		CommandName:   cmdName,
	}

	_, err = duosql.Insert[BotStickerCmd](duo).
		Values(item).
		OnConflictDoUpdateAll("our_jid", "sticker_sha256").
		Exec(ctx)
	return err
}

// DeleteStickerCmdBySHA removes a sticker shortcut mapping using its SHA-256 hash.
func DeleteStickerCmdBySHA(ctx context.Context, s *sqlstore.SQLStore, shaHex string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	_ = cache.Delete(ctx, stickerCacheKey(ourJID, shaHex))

	_, err = duosql.Delete[BotStickerCmd](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Eq("sticker_sha256", shaHex),
		).
		Exec(ctx)
	return err
}

// DeleteStickerCmdByName removes all sticker shortcuts pointing to a specific command.
func DeleteStickerCmdByName(ctx context.Context, s *sqlstore.SQLStore, cmdName string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}
	ourJID := ourJIDStr(s)

	_ = cache.DeletePrefix(ctx, "stkcmd:"+ourJID+":")

	_, err = duosql.Delete[BotStickerCmd](duo).
		Where(
			duosql.Eq("our_jid", ourJID),
			duosql.Or(
				duosql.Eq("command_name", cmdName),
				duosql.Like("command_name", cmdName+" %"),
			),
		).
		Exec(ctx)
	return err
}

// ListStickerCmds retrieves all registered sticker shortcuts for the current bot session.
func ListStickerCmds(ctx context.Context, s *sqlstore.SQLStore) ([]BotStickerCmd, error) {
	if s == nil {
		return nil, nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return nil, err
	}
	ourJID := ourJIDStr(s)

	return duosql.Select[BotStickerCmd](duo).
		Where(duosql.Eq("our_jid", ourJID)).
		All(ctx)
}
