-- Remitentes a los que se les muestran siempre las imágenes remotas.
CREATE TABLE image_senders (
  email    TEXT PRIMARY KEY COLLATE NOCASE,
  added_at INTEGER NOT NULL
);
