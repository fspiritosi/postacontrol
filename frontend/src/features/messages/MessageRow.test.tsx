import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLabels: vi.fn(),
  GetSettings: vi.fn(() => Promise.resolve({ readingPane: "" })),
  SetFlagged: vi.fn(() => Promise.resolve()),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import { MessageRow } from "./MessageRow";

const message = (labels: number[]): MessageSummary => ({
  id: 1, accountId: 7, folderId: 3, uid: 1,
  from: { name: "Ana", email: "ana@x.y" }, subject: "Hola", snippet: "",
  date: "2026-10-30T10:00:00Z", seen: true, flagged: false, answered: false,
  hasAttachments: false, labels,
});

function setup(labels: number[], onToggle = vi.fn()) {
  vi.mocked(svc.ListLabels).mockResolvedValue([
    { id: 1, name: "Facturas", color: "#dc2626", position: 0 },
    { id: 2, name: "Viaje", color: "#2563eb", position: 1 },
  ]);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ul>
        <MessageRow
          message={message(labels)}
          selected={false}
          onSelect={vi.fn()}
          marked={false}
          anyMarked={false}
          onToggle={onToggle}
        />
      </ul>
    </QueryClientProvider>,
  );
  return onToggle;
}

describe("MessageRow con etiquetas", () => {
  it("muestra la línea y la pastilla con el nombre", async () => {
    setup([1]);
    expect(await screen.findByText("Facturas")).toBeTruthy();
    expect(screen.getAllByTestId("label-segment")).toHaveLength(1);
  });

  it("sin etiquetas no hay línea", async () => {
    setup([]);
    await vi.waitFor(() => expect(svc.ListLabels).toHaveBeenCalled());
    expect(screen.queryByTestId("label-line")).toBeNull();
  });

  it("la línea no tapa la casilla de selección", async () => {
    const onToggle = setup([1, 2]);
    await screen.findByText("Facturas");
    expect(screen.getByTestId("label-line").className).toContain("pointer-events-none");
    fireEvent.click(screen.getByRole("checkbox", { name: "Seleccionar" }));
    expect(onToggle).toHaveBeenCalledWith(1);
  });
});
