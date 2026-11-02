-- Orden manual de las carpetas: 0 = sin posición (orden de siempre).
ALTER TABLE folders ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
