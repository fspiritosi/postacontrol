ALTER TABLE accounts ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
UPDATE accounts SET position = id;
