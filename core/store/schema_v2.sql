-- Bandeja de salida: los correos entran antes de enviarse ("sending") y quedan
-- como "failed" si el envío falla. Los existentes ya habían fallado.
ALTER TABLE local_drafts ADD COLUMN status TEXT NOT NULL DEFAULT 'failed';
