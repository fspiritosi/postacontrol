-- Color elegido para cada carpeta (#rrggbb); vacío = sin color. Es solo local:
-- la sincronización (upsert por nombre) no lo toca.
ALTER TABLE folders ADD COLUMN color TEXT NOT NULL DEFAULT '';
