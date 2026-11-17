import type { ShellState } from "../layout/MailShell";
import { vi } from "vitest";

// Estado de la vista de Tareas para los tests, con lo demás en blanco.
export function taskShellState(taskKey: string | null = null): ShellState {
  return {
    view: { kind: "tasks" },
    messageId: null,
    selectView: vi.fn(),
    selectMessage: vi.fn(),
    search: null,
    setSearch: vi.fn(),
    threadRoot: null,
    selectThread: vi.fn(),
    contactId: null,
    selectContact: vi.fn(),
    openContact: vi.fn(),
    taskKey,
    selectTask: vi.fn(),
    openTask: vi.fn(),
    marked: { keys: [], anchor: null },
    markedThreads: { keys: [], anchor: null },
    setMarked: vi.fn(),
    setMarkedThreads: vi.fn(),
    clearMarks: vi.fn(),
    labelFilter: null,
    setLabelFilter: vi.fn(),
  };
}

export const SETTINGS = {
  unifiedInbox: false, disableNotifications: false, disableSpellcheck: false, disableUndoSend: false,
  autoBackupDir: "", taskReminderMinutes: null, taskappApiUrl: "", taskappWebUrl: "", theme: "",
  threadedList: false, readingPane: "right", background: "",
};

export const localTask = (over: Record<string, unknown> = {}) => ({
  id: 1, title: "Llamar al banco", notes: "", dueDate: "", dueTime: "", remindBefore: null,
  sourceMessageId: "", sourceSubject: "", doneAt: null, createdAt: "2026-11-10T10:00:00Z", ...over,
});

export const remoteTask = (over: Record<string, unknown> = {}) => ({
  id: 5, ticket: true, title: "Arreglar login", priority: "high", dueDate: "", statusName: "En curso",
  projectId: 2, projectName: "Web", projectColor: "#ff0000", url: "https://taskapp.test/t/5",
  description: "<p>Detalle</p>", ...over,
});
