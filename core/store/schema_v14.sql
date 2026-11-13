-- Correos pospuestos: están en la carpeta Pospuestos del servidor y vuelven a
-- return_folder (por nombre) a la hora until (segundos unix). Se los
-- reconoce por su Message-ID.
CREATE TABLE snoozes (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  message_id    TEXT NOT NULL,
  return_folder TEXT NOT NULL,
  until         INTEGER NOT NULL
);
CREATE INDEX idx_snoozes_until ON snoozes(until);
CREATE INDEX idx_snoozes_message ON snoozes(account_id, message_id);
