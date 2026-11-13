-- Cómo desuscribirse de un mensaje (encabezados List-Unsubscribe), como JSON.
-- NULL = todavía no se miró en el servidor.
ALTER TABLE messages ADD COLUMN list_unsub TEXT;
