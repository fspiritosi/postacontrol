-- Tareas locales, vínculos hilo↔Taskapp y caché de la lista de Taskapp.
-- Los instantes van en segundos unix (UTC), como en el resto del esquema.
CREATE TABLE local_tasks (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  title             TEXT NOT NULL,
  notes             TEXT NOT NULL DEFAULT '',
  due_date          TEXT NOT NULL DEFAULT '',  -- AAAA-MM-DD o ''
  due_time          TEXT NOT NULL DEFAULT '',  -- HH:MM o ''; solo con due_date
  remind_before     INTEGER,                   -- minutos; NULL = el de Configuración
  source_message_id TEXT NOT NULL DEFAULT '',
  source_subject    TEXT NOT NULL DEFAULT '',
  done_at           INTEGER,
  reminded_at       INTEGER,
  created_at        INTEGER NOT NULL
);

CREATE TABLE task_links (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  thread_root       TEXT NOT NULL,
  task_id           INTEGER NOT NULL,
  ticket            INTEGER NOT NULL DEFAULT 0,
  project_name      TEXT NOT NULL DEFAULT '',
  title             TEXT NOT NULL DEFAULT '',
  source_message_id TEXT NOT NULL DEFAULT '',
  attach_pending    INTEGER NOT NULL DEFAULT 0,
  created_at        INTEGER NOT NULL
);
CREATE INDEX idx_task_links_thread ON task_links (thread_root);

-- Correos ya sumados a un vínculo, por Message-ID.
CREATE TABLE task_link_messages (
  link_id    INTEGER NOT NULL REFERENCES task_links(id) ON DELETE CASCADE,
  message_id TEXT NOT NULL,
  PRIMARY KEY (link_id, message_id)
);

CREATE TABLE taskapp_cache (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  json       TEXT NOT NULL,
  fetched_at INTEGER NOT NULL
);
