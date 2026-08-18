-- v19: WhatsRook custom tables schema
CREATE TABLE IF NOT EXISTS bot_settings (
    our_jid TEXT DEFAULT '',
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (key)
);

CREATE TABLE IF NOT EXISTS call_media_config (
    our_jid TEXT DEFAULT '',
    jid TEXT NOT NULL,
    kind TEXT NOT NULL,
    file_path TEXT NOT NULL,
    PRIMARY KEY (jid, kind)
);
