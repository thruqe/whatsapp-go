package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/thruqe/duosql"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
)

// GetCallMediaConfig retrieves the configured media file path for rejecting incoming calls.
func GetCallMediaConfig(ctx context.Context, s *sqlstore.SQLStore, jid types.JID, kind CallMediaKind) (string, error) {
	if s == nil {
		return "", nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return "", err
	}

	ourJID := ourJIDStr(s)
	jidStr := jid.ToNonAD().String()

	orderExpr := fmt.Sprintf(
		"CASE WHEN our_jid = %s THEN 1 WHEN our_jid = %s THEN 2 ELSE 3 END",
		sqlQuoteLiteral(ourJID), sqlQuoteLiteral(s.JID),
	)

	cfg, err := duosql.Select[CallMediaConfig](duo).
		Where(
			duosql.Or(
				duosql.Eq("our_jid", ourJID),
				duosql.Eq("our_jid", s.JID),
				duosql.Eq("our_jid", ""),
				duosql.IsNull("our_jid"),
			),
			duosql.Eq("jid", jidStr),
			duosql.Eq("kind", string(kind)),
		).
		OrderBy(duosql.RawOrderBy(orderExpr)).
		Limit(1).
		One(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.FilePath, nil
}

// PutCallMediaConfig associates a media file path with a caller or global call-decline action.
func PutCallMediaConfig(ctx context.Context, s *sqlstore.SQLStore, jid types.JID, kind CallMediaKind, filePath string) error {
	if s == nil {
		return nil
	}
	duo, err := getDuoDB(s)
	if err != nil {
		return err
	}

	ourJID := ourJIDStr(s)
	jidStr := jid.ToNonAD().String()

	item := &CallMediaConfig{
		OurJID:   ourJID,
		JID:      jidStr,
		Kind:     string(kind),
		FilePath: filePath,
	}

	_, err = duosql.Insert[CallMediaConfig](duo).
		Values(item).
		OnConflictDoUpdateAll("our_jid", "jid", "kind").
		Exec(ctx)
	return err
}
