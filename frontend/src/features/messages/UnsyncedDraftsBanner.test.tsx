import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListPendingDrafts: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
const composer = vi.hoisted(() => ({ openComposeDraft: vi.fn(() => Promise.resolve()) }));
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => composer }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Draft } from "../../../bindings/postacontrol/core/model/models";
import { UnsyncedDraftsBanner } from "./UnsyncedDraftsBanner";

function draft(id: number, accountId: number, subject: string): Draft {
  return {
    id, messageId: `<${id}@x>`, needsUpload: true, uploadError: "Sin conexión.", updatedAt: "",
    request: { accountId, subject } as Draft["request"],
  };
}

function setup(accountId: number | null) {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <UnsyncedDraftsBanner accountId={accountId} />
    </QueryClientProvider>,
  );
  return user;
}

describe("UnsyncedDraftsBanner", () => {
  beforeEach(() => vi.clearAllMocks());

  it("el aviso cuenta los pendientes de la cuenta y Abrir los abre", async () => {
    const mine = [draft(1, 7, "Presupuesto"), draft(2, 7, "")];
    vi.mocked(svc.ListPendingDrafts).mockResolvedValue([...mine, draft(3, 8, "De la otra cuenta")]);
    const user = setup(7);
    expect(await screen.findByText("2 borradores todavía no se subieron al servidor.")).toBeTruthy();
    expect(screen.queryByText("De la otra cuenta")).toBeNull();
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("Presupuesto")).toBeTruthy();
    expect(within(items[1]).getByText("(Sin asunto)")).toBeTruthy();
    await user.click(within(items[1]).getByRole("button", { name: "Abrir" }));
    await waitFor(() => expect(composer.openComposeDraft).toHaveBeenCalledWith(mine[1]));
  });

  it("singular con un solo pendiente", async () => {
    vi.mocked(svc.ListPendingDrafts).mockResolvedValue([draft(1, 7, "Presupuesto")]);
    setup(null);
    expect(await screen.findByText("1 borrador todavía no se subió al servidor.")).toBeTruthy();
  });

  it("en la vista unificada cuenta los de todas las cuentas", async () => {
    vi.mocked(svc.ListPendingDrafts).mockResolvedValue([draft(1, 7, "a"), draft(2, 8, "b")]);
    setup(null);
    expect(await screen.findByText("2 borradores todavía no se subieron al servidor.")).toBeTruthy();
  });

  it("sin pendientes no muestra nada", async () => {
    vi.mocked(svc.ListPendingDrafts).mockResolvedValue(null);
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <UnsyncedDraftsBanner accountId={7} />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(svc.ListPendingDrafts).toHaveBeenCalled());
    expect(container.textContent).toBe("");
  });
});
