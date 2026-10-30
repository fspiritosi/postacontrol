CREATE TABLE accounts (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  email          TEXT NOT NULL,
  display_name   TEXT NOT NULL DEFAULT '',
  username       TEXT NOT NULL,
  color          TEXT NOT NULL DEFAULT '',
  imap_host      TEXT NOT NULL,
  imap_port      INTEGER NOT NULL,
  imap_security  TEXT NOT NULL,
  smtp_host      TEXT NOT NULL,
  smtp_port      INTEGER NOT NULL,
  smtp_security  TEXT NOT NULL,
  save_sent_copy INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE folders (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id   INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  delimiter    TEXT NOT NULL DEFAULT '',
  role         TEXT NOT NULL DEFAULT '',
  selectable   INTEGER NOT NULL DEFAULT 1,
  unread       INTEGER NOT NULL DEFAULT 0,
  total        INTEGER NOT NULL DEFAULT 0,
  uid_validity INTEGER NOT NULL DEFAULT 0,
  uid_next     INTEGER NOT NULL DEFAULT 0,
  UNIQUE (account_id, name)
);

CREATE TABLE messages (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  folder_id         INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  uid               INTEGER NOT NULL CHECK (uid > 0),
  message_id        TEXT NOT NULL DEFAULT '',
  in_reply_to       TEXT NOT NULL DEFAULT '',
  references_json   TEXT NOT NULL DEFAULT '[]',
  from_name         TEXT NOT NULL DEFAULT '',
  from_email        TEXT NOT NULL DEFAULT '',
  to_json           TEXT NOT NULL DEFAULT '[]',
  cc_json           TEXT NOT NULL DEFAULT '[]',
  reply_to_json     TEXT NOT NULL DEFAULT '[]',
  subject           TEXT NOT NULL DEFAULT '',
  snippet           TEXT NOT NULL DEFAULT '',
  date              INTEGER NOT NULL DEFAULT 0, -- segundos unix UTC
  size              INTEGER NOT NULL DEFAULT 0,
  seen              INTEGER NOT NULL DEFAULT 0,
  flagged           INTEGER NOT NULL DEFAULT 0,
  answered          INTEGER NOT NULL DEFAULT 0,
  has_attachments   INTEGER NOT NULL DEFAULT 0,
  body_cached       INTEGER NOT NULL DEFAULT 0,
  body_html         TEXT NOT NULL DEFAULT '',
  body_text         TEXT NOT NULL DEFAULT '',
  has_remote_images INTEGER NOT NULL DEFAULT 0,
  attachments_json  TEXT NOT NULL DEFAULT '[]',
  UNIQUE (folder_id, uid)
);
CREATE INDEX idx_messages_folder_date ON messages (folder_id, date DESC);
CREATE INDEX idx_messages_message_id ON messages (message_id);
CREATE INDEX idx_messages_in_reply_to ON messages (in_reply_to);

CREATE TABLE local_drafts (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id   INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  request_json TEXT NOT NULL,
  error        TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL
);
