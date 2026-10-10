package store

import (
	"testing"

	"github.com/thruqe/duosql"
	"go.mau.fi/whatsmeow/types"
)

func TestPurgeSessionData(t *testing.T) {
	doomed := types.NewJID("1234567890", types.DefaultUserServer)
	kept := types.NewJID("9876543210", types.DefaultUserServer)

	duo, err := getDuoDBFromDB(testDB)
	if err != nil {
		t.Fatal(err)
	}

	seed := func(jid string) {
		if _, err := duosql.Insert[BotSetting](duo).Values(&BotSetting{OurJID: jid, Key: "prefix", Value: "!"}).Exec(testCtx); err != nil {
			t.Fatalf("seed settings: %v", err)
		}
		if _, err := duosql.Insert[BotFilter](duo).Values(&BotFilter{OurJID: jid, TriggerWord: "hi", MessageProto: "x"}).Exec(testCtx); err != nil {
			t.Fatalf("seed filter: %v", err)
		}
		if _, err := duosql.Insert[BotPlatformCookie](duo).Values(&BotPlatformCookie{OurJID: jid, Platform: "yt", Cookies: "c"}).Exec(testCtx); err != nil {
			t.Fatalf("seed cookie: %v", err)
		}
	}
	seed(doomed.String())
	seed(kept.String())

	if err := PurgeSessionData(testCtx, testDB, "+1234567890"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	count := func(jid string) (n int) {
		s, _ := duosql.Select[BotSetting](duo).Where(duosql.Eq("our_jid", jid)).All(testCtx)
		f, _ := duosql.Select[BotFilter](duo).Where(duosql.Eq("our_jid", jid)).All(testCtx)
		c, _ := duosql.Select[BotPlatformCookie](duo).Where(duosql.Eq("our_jid", jid)).All(testCtx)
		return len(s) + len(f) + len(c)
	}
	if n := count(doomed.String()); n != 0 {
		t.Errorf("expected 0 rows for purged session, got %d", n)
	}
	if n := count(kept.String()); n != 3 {
		t.Errorf("expected 3 rows for other session, got %d", n)
	}
}
