import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLabels: vi.fn(),
  SetLabel: vi.fn(() => Promise.resolve()),
  GetMessage: vi.fn(),
  GetThread: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import { NO_MARKS } from "@/lib/selection";
import type { ShellState } from "../layout/MailShell";
import { useLabelKeys } from "./useLabelKeys";

const LABELS = [
  { id: 11, name: "A", color: "#dc2626", position: 0 },
  { id: 12, name: "B", color: "#ea580c", position: 1 },
];

const summary = (id: number, labels: number[]): MessageSummary => ({
  id,
  accountId: 7,
  folderId: 3,
  uid: id,
  from: { name: "x", email: "x@y.z" },
  subject: "s",
  snippet: "",
  date: "2026-10-30T10:00:00Z",
  seen: true,
  flagged: false,
  answered: false,
  hasAttachments: false,
  labels,
});

function shell(over: Partial<ShellState> = {}): ShellState {
  return {
    view: { kind: "folder", folderId: 3 },
    messageId: null,
    threadRoot: null,
    marked: NO_MARKS,
    markedThreads: NO_MARKS,
    ...over,
  } as ShellState;
}

function Harness({ state }: { state: ShellState }) {
  useLabelKeys(state);
  return (
    <div>
      <input aria-label="caja" />
      <div contentEditable suppressContentEditableWarning data-testid="ce" />
    </div>
  );
}

function setup(state: ShellState, items: MessageSummary[]) {
  const qc = new QueryClient();
  qc.setQueryData(["labels"], LABELS);
  qc.setQueryData(["messages", 3, false], { pages: [items], pageParams: [0] });
  const view = render(
    <QueryClientProvider client={qc}>
      <Harness state={state} />
    </QueryClientProvider>,
  );
  return { qc, ...view };
}

describe("useLabelKeys", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListLabels).mockResolvedValue(LABELS as never);
  });

  it("con un correo abierto, 2 pone la segunda etiqueta", async () => {
    setup(shell({ messageId: 1 }), [summary(1, [])]);
    await userEvent.keyboard("2");
    await waitFor(() => expect(svc.SetLabel).toHaveBeenCalledWith([1], 12, true));
  });

  it("si ya la tiene, la saca", async () => {
    setup(shell({ messageId: 1 }), [summary(1, [12])]);
    await userEvent.keyboard("2");
    await waitFor(() =>
      expect(svc.SetLabel).toHaveBeenCalledWith([1], 12, false),
    );
  });

  it("con dos marcados y estado mixto, 1 la pone en los dos", async () => {
    setup(
      shell({ marked: { keys: [1, 2], anchor: 1 }, messageId: 1 }),
      [summary(1, [11]), summary(2, [])],
    );
    await userEvent.keyboard("1");
    await waitFor(() =>
      expect(svc.SetLabel).toHaveBeenCalledWith([1, 2], 11, true),
    );
    expect(svc.SetLabel).toHaveBeenCalledTimes(1);
  });

  it("con una conversación abierta actúa sobre todos sus mensajes", async () => {
    vi.mocked(svc.GetThread).mockResolvedValue([
      summary(1, []),
      summary(2, []),
    ] as never);
    setup(shell({ threadRoot: "<a@x>" }), []);
    await userEvent.keyboard("1");
    await waitFor(() =>
      expect(svc.SetLabel).toHaveBeenCalledWith([1, 2], 11, true),
    );
    expect(svc.GetThread).toHaveBeenCalledWith(3, "<a@x>");
  });

  it("no hace nada escribiendo, con modificadores, con diálogo o sin esa etiqueta", async () => {
    const { getByLabelText, getByTestId } = setup(shell({ messageId: 1 }), [
      summary(1, []),
    ]);
    const settle = () => new Promise((r) => setTimeout(r, 20));

    getByLabelText("caja").focus();
    await userEvent.keyboard("2");
    getByTestId("ce").focus();
    await userEvent.keyboard("2");
    (document.activeElement as HTMLElement).blur();

    await userEvent.keyboard("{Control>}2{/Control}");
    await userEvent.keyboard("{Alt>}2{/Alt}");
    await userEvent.keyboard("{Meta>}2{/Meta}");
    await userEvent.keyboard("9");

    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    document.body.appendChild(dialog);
    await userEvent.keyboard("2");
    dialog.remove();

    await settle();
    expect(svc.SetLabel).not.toHaveBeenCalled();
  });
});
