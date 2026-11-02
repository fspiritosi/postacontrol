import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLabels: vi.fn(),
  SetLabel: vi.fn(() => Promise.resolve()),
  SetSeen: vi.fn(() => Promise.resolve()),
  SetFlagged: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Target } from "@/lib/bulk";
import { RowContextMenu } from "./RowContextMenu";

const LABELS = [
  { id: 11, name: "Clientes", color: "#dc2626", position: 0 },
  { id: 12, name: "Urgente", color: "#ea580c", position: 1 },
];

const t = (id: number, extra: Partial<Target> = {}): Target => ({
  id,
  accountId: 7,
  folderId: 3,
  seen: true,
  flagged: false,
  labels: [],
  ...extra,
});

function setup(targets: Target[], onUnread = vi.fn()) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RowContextMenu targets={() => Promise.resolve(targets)} onUnread={onUnread}>
        <div data-testid="fila">fila</div>
      </RowContextMenu>
    </QueryClientProvider>,
  );
  fireEvent.contextMenu(screen.getByTestId("fila"));
  return { onUnread };
}

describe("RowContextMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListLabels).mockResolvedValue(LABELS as never);
  });

  it("bajo Etiquetas están las etiquetas y elegir una la pone", async () => {
    setup([t(5)]);
    const sub = await screen.findByRole("menuitem", { name: "Etiquetas" });
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "Destacar" })
        .getAttribute("data-disabled")).toBeNull(),
    );
    fireEvent.keyDown(sub, { key: "ArrowRight" });
    await userEvent.click(await screen.findByRole("menuitem", { name: /Urgente/ }));
    await waitFor(() => expect(svc.SetLabel).toHaveBeenCalledWith([5], 12, true));
  });

  it("sobre varios marcados actúa sobre todos y con estado mixto la pone en todos", async () => {
    setup([t(5, { labels: [11] }), t(6)]);
    const sub = await screen.findByRole("menuitem", { name: "Etiquetas" });
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "Destacar" })
        .getAttribute("data-disabled")).toBeNull(),
    );
    fireEvent.keyDown(sub, { key: "ArrowRight" });
    expect(await screen.findByTestId("label-some")).toBeTruthy();
    await userEvent.click(screen.getByRole("menuitem", { name: /Clientes/ }));
    await waitFor(() =>
      expect(svc.SetLabel).toHaveBeenCalledWith([5, 6], 11, true),
    );
  });

  it("Marcar como no leído llama SetSeen(ids, false) y avisa", async () => {
    const { onUnread } = setup([t(5), t(6)]);
    const item = await screen.findByRole("menuitem", { name: "Marcar como no leído" });
    await waitFor(() => expect(item.getAttribute("data-disabled")).toBeNull());
    await userEvent.click(item);
    await waitFor(() => expect(svc.SetSeen).toHaveBeenCalledWith([5, 6], false));
    expect(onUnread).toHaveBeenCalled();
  });

  it("con algún no leído ofrece Marcar como leído, y Destacar / Quitar destacado", async () => {
    setup([t(5, { seen: false, flagged: true }), t(6, { flagged: true })]);
    const item = await screen.findByRole("menuitem", { name: "Marcar como leído" });
    await waitFor(() => expect(item.getAttribute("data-disabled")).toBeNull());
    await userEvent.click(item);
    await waitFor(() => expect(svc.SetSeen).toHaveBeenCalledWith([5, 6], true));
  });

  it("Quitar destacado cuando todos lo están", async () => {
    setup([t(5, { flagged: true })]);
    const item = await screen.findByRole("menuitem", { name: "Quitar destacado" });
    await waitFor(() => expect(item.getAttribute("data-disabled")).toBeNull());
    await userEvent.click(item);
    await waitFor(() => expect(svc.SetFlagged).toHaveBeenCalledWith([5], false));
  });
});
