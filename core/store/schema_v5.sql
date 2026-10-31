-- Adjuntos preparados: archivos que el redactor eligió y todavía no salieron.
-- Se guardan acá para no depender del archivo original hasta el envío.
CREATE TABLE staged_attachments (
  id         INTEGER PRIMARY KEY,
  filename   TEXT NOT NULL,
  mime_type  TEXT NOT NULL,
  size       INTEGER NOT NULL,
  data       BLOB NOT NULL,
  created_at INTEGER NOT NULL
);
