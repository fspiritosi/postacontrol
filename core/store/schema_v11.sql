-- Reglas de cada cuenta (como los filtros de Thunderbird). Condiciones y
-- acciones van como JSON: la carpeta y la etiqueta de una acción se validan al
-- guardar y al aplicar, porque pueden desaparecer después.
CREATE TABLE rules (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  enabled         INTEGER NOT NULL DEFAULT 1,
  match_all       INTEGER NOT NULL DEFAULT 1,
  conditions_json TEXT NOT NULL DEFAULT '[]',
  actions_json    TEXT NOT NULL DEFAULT '[]',
  position        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_rules_account ON rules(account_id, position);
