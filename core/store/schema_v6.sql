-- Firmas reutilizables y borradores autoguardados del redactor.
CREATE TABLE signatures (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  html       TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Firma por defecto de cada cuenta; NULL = sin firma.
ALTER TABLE accounts ADD COLUMN default_signature_id INTEGER REFERENCES signatures(id) ON DELETE SET NULL;

-- Borradores del redactor. request_json guarda el SendRequest completo.
-- updated_at va en nanosegundos (no segundos) para distinguir guardados seguidos.
-- AUTOINCREMENT: un id no se reutiliza, así un id viejo (de un envío que quedó
-- en la bandeja de salida) nunca apunta a un borrador nuevo.
-- uploaded_account_id es la cuenta en cuya carpeta Borradores está la última
-- copia subida (NULL = nunca se subió): si el borrador cambia de cuenta, la
-- copia vieja se borra de ahí.
CREATE TABLE compose_drafts (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id          INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  request_json        TEXT NOT NULL,
  message_id          TEXT NOT NULL UNIQUE,
  needs_upload        INTEGER NOT NULL DEFAULT 1,
  upload_error        TEXT NOT NULL DEFAULT '',
  updated_at          INTEGER NOT NULL,
  uploaded_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL
);
