-- Etiquetas de usuario: se guardan en el servidor como keywords IMAP ($pc_…) y se
-- reflejan acá. labels_local = 1 marca la cuenta cuyo servidor no admite keywords.
CREATE TABLE labels (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  name     TEXT NOT NULL,
  color    TEXT NOT NULL,
  keyword  TEXT NOT NULL UNIQUE,
  position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE message_labels (
  message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  label_id   INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  PRIMARY KEY (message_id, label_id)
);
CREATE INDEX idx_message_labels_label ON message_labels(label_id);

-- Keywords de etiquetas borradas que faltan limpiar en el servidor de cada cuenta.
CREATE TABLE label_cleanup (
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  keyword    TEXT NOT NULL,
  PRIMARY KEY (account_id, keyword)
);

-- Etiquetas de un mensaje movido en una cuenta local: se reaplican al reaparecer su Message-ID.
CREATE TABLE label_carry (
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  message_id TEXT NOT NULL,
  label_id   INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  PRIMARY KEY (account_id, message_id, label_id)
);

ALTER TABLE accounts ADD COLUMN labels_local INTEGER NOT NULL DEFAULT 0;
