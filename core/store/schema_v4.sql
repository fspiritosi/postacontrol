-- Índice de búsqueda local (FTS5). rowid = messages.id.
CREATE VIRTUAL TABLE messages_fts USING fts5(subject, sender, recipients, body, tokenize='unicode61 remove_diacritics 2');

CREATE TRIGGER messages_fts_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, subject, sender, recipients, body)
  VALUES (new.id, new.subject, new.from_name || ' ' || new.from_email, new.to_json || ' ' || new.cc_json, new.body_text);
END;

CREATE TRIGGER messages_fts_au AFTER UPDATE OF subject, from_name, from_email, to_json, cc_json, body_text ON messages BEGIN
  DELETE FROM messages_fts WHERE rowid = old.id;
  INSERT INTO messages_fts(rowid, subject, sender, recipients, body)
  VALUES (new.id, new.subject, new.from_name || ' ' || new.from_email, new.to_json || ' ' || new.cc_json, new.body_text);
END;

CREATE TRIGGER messages_fts_ad AFTER DELETE ON messages BEGIN
  DELETE FROM messages_fts WHERE rowid = old.id;
END;

INSERT INTO messages_fts(rowid, subject, sender, recipients, body)
SELECT id, subject, from_name || ' ' || from_email, to_json || ' ' || cc_json, body_text FROM messages;

ALTER TABLE messages ADD COLUMN index_failures INTEGER NOT NULL DEFAULT 0;
