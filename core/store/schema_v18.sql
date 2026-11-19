-- Calendario: eventos, invitaciones procesadas, avisos reclamados y caché de
-- reuniones de Taskapp. Los instantes van en segundos unix (UTC).
CREATE TABLE events (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  uid                TEXT NOT NULL UNIQUE,
  kind               TEXT NOT NULL CHECK (kind IN ('local','invite','taskapp')),
  title              TEXT NOT NULL DEFAULT '',
  description        TEXT NOT NULL DEFAULT '',
  notes              TEXT NOT NULL DEFAULT '',
  location           TEXT NOT NULL DEFAULT '',
  all_day            INTEGER NOT NULL DEFAULT 0,
  start_at           INTEGER NOT NULL,
  end_at             INTEGER NOT NULL,
  tzid               TEXT NOT NULL DEFAULT '',
  rrule              TEXT NOT NULL DEFAULT '',
  series_end_at      INTEGER,                   -- fin de la última ocurrencia; NULL = sin fin
  remind_before      INTEGER,                   -- minutos; NULL = el de Configuración
  account_id         INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
  organizer_name     TEXT NOT NULL DEFAULT '',
  organizer_email    TEXT NOT NULL DEFAULT '',
  sequence           INTEGER NOT NULL DEFAULT 0,
  status             TEXT NOT NULL DEFAULT 'confirmed',
  my_partstat        TEXT NOT NULL DEFAULT '',
  my_email           TEXT NOT NULL DEFAULT '',
  alias_reply        INTEGER NOT NULL DEFAULT 0,
  needs_review       INTEGER NOT NULL DEFAULT 0,
  raw_ics            TEXT NOT NULL DEFAULT '',
  taskapp_meeting_id INTEGER UNIQUE,
  taskapp_snapshot   TEXT NOT NULL DEFAULT '',
  taskapp_change     TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL,
  updated_at         INTEGER NOT NULL
);
CREATE INDEX idx_events_start ON events (start_at);

CREATE TABLE event_attendees (
  event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  email    TEXT NOT NULL,
  name     TEXT NOT NULL DEFAULT '',
  partstat TEXT NOT NULL DEFAULT 'NEEDS-ACTION',
  PRIMARY KEY (event_id, email)
);

CREATE TABLE event_exceptions (
  event_id      INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  recurrence_at INTEGER NOT NULL,
  cancelled     INTEGER NOT NULL DEFAULT 0,
  start_at      INTEGER NOT NULL DEFAULT 0,
  end_at        INTEGER NOT NULL DEFAULT 0,
  title         TEXT NOT NULL DEFAULT '',
  location      TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (event_id, recurrence_at)
);

CREATE TABLE event_reminders (
  event_key     TEXT NOT NULL,
  occurrence_at INTEGER NOT NULL,
  reminded_at   INTEGER NOT NULL,
  PRIMARY KEY (event_key, occurrence_at)
);

CREATE TABLE invite_messages (
  message_id   INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
  method       TEXT NOT NULL DEFAULT '',
  uid          TEXT NOT NULL DEFAULT '',
  sequence     INTEGER NOT NULL DEFAULT 0,
  sender       TEXT NOT NULL DEFAULT '',
  ics          TEXT NOT NULL DEFAULT '',
  outcome      TEXT NOT NULL DEFAULT '',
  processed_at INTEGER NOT NULL
);
CREATE INDEX idx_invite_messages_seen ON invite_messages (uid, sequence, sender, method);

CREATE TABLE taskapp_meetings (
  id           INTEGER PRIMARY KEY,
  workspace_id INTEGER NOT NULL,
  start_at     INTEGER NOT NULL,
  end_at       INTEGER NOT NULL,
  json         TEXT NOT NULL,
  fetched_at   INTEGER NOT NULL
);
CREATE INDEX idx_taskapp_meetings_start ON taskapp_meetings (start_at);

-- Los correos recientes de la Entrada vuelven a bajar el cuerpo para que se
-- detecten las invitaciones (text/calendar) que antes se descartaban.
UPDATE messages SET body_cached = 0
 WHERE body_cached = 1
   AND date >= CAST(strftime('%s','now','-60 days') AS INTEGER)
   AND folder_id IN (SELECT id FROM folders WHERE role = 'inbox');
