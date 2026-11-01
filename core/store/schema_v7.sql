-- Contactos (libreta local) y destinatarios usados (para sugerir al escribir).
CREATE TABLE contacts (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL DEFAULT '',
  phones_json TEXT NOT NULL DEFAULT '[]',
  company     TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  updated_at  INTEGER NOT NULL
);
-- Un correo pertenece a una sola ficha (sin distinguir mayúsculas).
CREATE TABLE contact_emails (
  contact_id INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  email      TEXT NOT NULL,
  PRIMARY KEY (contact_id, position)
);
CREATE UNIQUE INDEX idx_contact_emails_email ON contact_emails (email COLLATE NOCASE);

-- A quiénes les escribí: para sugerir destinatarios. email va en minúsculas.
CREATE TABLE recipients (
  email     TEXT PRIMARY KEY,
  name      TEXT NOT NULL DEFAULT '',
  times     INTEGER NOT NULL DEFAULT 0,
  last_used INTEGER NOT NULL DEFAULT 0
);

-- Cada mensaje nuevo en una carpeta Enviados suma 1 a cada destinatario (Para y
-- Cc), una sola vez por mensaje aunque la dirección se repita. UpsertHeaders
-- usa ON CONFLICT DO UPDATE, así que volver a sincronizar un mensaje no lo dispara.
CREATE TRIGGER recipients_from_sent AFTER INSERT ON messages
WHEN (SELECT role FROM folders WHERE id = NEW.folder_id) = 'sent'
BEGIN
  INSERT INTO recipients (email, name, times, last_used)
  SELECT lower(trim(email)), max(name), 1, NEW.date
  FROM (
    SELECT json_extract(value, '$.email') AS email, coalesce(json_extract(value, '$.name'), '') AS name
      FROM json_each(NEW.to_json)
    UNION ALL
    SELECT json_extract(value, '$.email'), coalesce(json_extract(value, '$.name'), '')
      FROM json_each(NEW.cc_json)
  )
  WHERE email LIKE '%@%'
  GROUP BY lower(trim(email))
  ON CONFLICT(email) DO UPDATE SET
    times     = recipients.times + 1,
    last_used = max(recipients.last_used, excluded.last_used),
    name      = CASE WHEN excluded.name <> '' THEN excluded.name ELSE recipients.name END;
END;

-- Relleno con lo que ya está en Enviados: un mensaje cuenta una vez por dirección;
-- el nombre es el del mensaje más reciente que lo trae.
WITH raw AS (
  SELECT m.id AS mid, m.date AS date,
         lower(trim(json_extract(j.value, '$.email'))) AS email,
         coalesce(json_extract(j.value, '$.name'), '') AS name
  FROM messages m
  JOIN folders f ON f.id = m.folder_id AND f.role = 'sent',
       json_each(m.to_json) j
  UNION ALL
  SELECT m.id, m.date, lower(trim(json_extract(j.value, '$.email'))),
         coalesce(json_extract(j.value, '$.name'), '')
  FROM messages m
  JOIN folders f ON f.id = m.folder_id AND f.role = 'sent',
       json_each(m.cc_json) j
), per_msg AS (
  SELECT email, max(name) AS name, date FROM raw WHERE email LIKE '%@%' GROUP BY mid, email
)
INSERT INTO recipients (email, name, times, last_used)
SELECT email,
       coalesce((SELECT p.name FROM per_msg p WHERE p.email = per_msg.email AND p.name <> ''
                 ORDER BY p.date DESC LIMIT 1), ''),
       count(*), max(date)
FROM per_msg
GROUP BY email;
