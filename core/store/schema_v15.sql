-- Plantillas y respuestas rápidas: se insertan en el redactor con "/".
CREATE TABLE templates (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  subject    TEXT NOT NULL DEFAULT '',
  html       TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);
