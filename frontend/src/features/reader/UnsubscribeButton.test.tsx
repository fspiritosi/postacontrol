import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  UnsubscribeInfo: vi.fn(),
  Unsubscribe: vi.fn(),
  OpenExternal: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { UnsubscribeButton } from "./UnsubscribeButton";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
});

const none = { available: false, oneClick: false, url: "", mailto: "" };

function setup(info = { ...none, available: true, oneClick: true, url: "https://x.com/u" }) {
  vi.mocked(svc.UnsubscribeInfo).mockResolvedValue(info);
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <UnsubscribeButton messageId={5} sender="news@tienda.com" />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("UnsubscribeButton", () => {
  beforeEach(() => vi.clearAllMocks());

  it("sin cómo desuscribirse no aparece", async () => {
    setup(none);
    await waitFor(() => expect(svc.UnsubscribeInfo).toHaveBeenCalledWith(5));
    expect(screen.queryByRole("button", { name: "Desuscribirme" })).toBeNull();
  });

  it("con un clic: confirma, da de baja y avisa", async () => {
    vi.mocked(svc.Unsubscribe).mockResolvedValue({ method: "post", url: "" });
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Desuscribirme" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("news@tienda.com");
    await user.click(screen.getByRole("button", { name: "Darme de baja" }));
    await waitFor(() => expect(svc.Unsubscribe).toHaveBeenCalledWith(5));
    expect(toast.success).toHaveBeenCalledWith("Te diste de baja de news@tienda.com.");
  });

  it("cancelar no da de baja", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Desuscribirme" }));
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(svc.Unsubscribe).not.toHaveBeenCalled();
  });

  it("si solo hay una página, la abre en el navegador", async () => {
    vi.mocked(svc.Unsubscribe).mockResolvedValue({ method: "web", url: "https://x.com/pref" });
    const user = setup({ ...none, available: true, url: "https://x.com/pref" });
    await user.click(await screen.findByRole("button", { name: "Desuscribirme" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("se abre la página del remitente");
    await user.click(screen.getByRole("button", { name: "Abrir la página" }));
    await waitFor(() => expect(svc.OpenExternal).toHaveBeenCalledWith("https://x.com/pref"));
  });

  it("por correo avisa que se mandó el pedido", async () => {
    vi.mocked(svc.Unsubscribe).mockResolvedValue({ method: "mail", url: "" });
    const user = setup({ ...none, available: true, mailto: "mailto:baja@x.com" });
    await user.click(await screen.findByRole("button", { name: "Desuscribirme" }));
    await user.click(screen.getByRole("button", { name: "Darme de baja" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Mandamos el pedido de baja a news@tienda.com."));
  });

  it("si falla lo avisa", async () => {
    vi.mocked(svc.Unsubscribe).mockRejectedValue(new Error("El remitente no confirmó la baja."));
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Desuscribirme" }));
    await user.click(screen.getByRole("button", { name: "Darme de baja" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("El remitente no confirmó la baja."));
  });
});
