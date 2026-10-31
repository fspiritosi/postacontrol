import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { ComposerProvider, useComposer } from "./ComposerContext";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(),
  PrepareCompose: vi.fn(),
  QueueSend: vi.fn(),
  ListLocalDrafts: vi.fn(),
  DeleteLocalDraft: vi.fn(),
  OpenExternal: vi.fn(),
  PickImage: vi.fn(),
  PickAttachments: vi.fn(),
  StagedInfo: vi.fn(),
}));
vi.mock("sonner", () => ({
  toast: { loading: vi.fn(), success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));
// Eventos de Wails que el test puede disparar a mano (files-staged).
const wails = vi.hoisted(() => ({ handlers: new Map<string, (ev: { data: unknown }) => void>() }));
vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, cb: (ev: { data: unknown }) => void) => {
      wails.handlers.set(name, cb);
      return () => wails.handlers.delete(name);
    },
  },
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { toast } from "sonner";

const account = {
  id: 1, email: "yo@ej.com", displayName: "Yo", username: "yo@ej.com", color: "#000",
  imap: { host: "i", port: 993, security: "tls" },
  smtp: { host: "s", port: 465, security: "tls" },
  saveSentCopy: true,
};

function Harness() {
  const c = useComposer();
  return (
    <>
      <button onClick={() => c.openNew(1)}>nuevo</button>
      <button onClick={() => void c.openFrom(7, "reply")}>responder</button>
      <button onClick={() => void c.openFrom(7, "forward")}>reenviar</button>
    </>
  );
}

const draft = {
  id: 5,
  createdAt: "2026-10-30T10:00:00Z",
  request: {
    accountId: 1,
    to: [{ name: "", email: "z@x.com" }],
    cc: [],
    bcc: [],
    subject: "Pendiente",
    bodyHtml: "<p>hola</p>",
    reply: null,
  },
  status: "failed",
  error: "x",
};

function DraftHarness() {
  const c = useComposer();
  return (
    <>
      <button onClick={() => void c.openDraft(draft as never)}>borrador</button>
      <button
        onClick={() =>
          void c.openDraft({ ...draft, request: { ...draft.request, reply: { mode: "forward", originalMessageId: 7 }, skipOriginalAttachments: [0] } } as never)
        }
      >
        reenvío guardado
      </button>
      <button
        onClick={() =>
          void c.openDraft({ ...draft, request: { ...draft.request, attachments: [4], skipOriginalAttachments: [2] } } as never)
        }
      >
        borrador con adjuntos
      </button>
    </>
  );
}

const MB = 1024 * 1024;
const staged = (id: number, filename: string, size = 1000) => ({ id, filename, mimeType: "application/pdf", size });

function setup() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ComposerProvider>
        <Harness />
        <DraftHarness />
      </ComposerProvider>
    </QueryClientProvider>,
  );
  return user;
}

// jsdom no calcula layout: ProseMirror pide rectángulos al mover la selección.
beforeAll(() => {
  const noRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
  Range.prototype.getClientRects = noRects;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
});

describe("Composer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([]);
    vi.mocked(svc.DeleteLocalDraft).mockResolvedValue();
    vi.mocked(svc.PrepareCompose).mockResolvedValue({
      accountId: 1,
      to: [{ name: "Ana", email: "ana@x.com" }],
      cc: [],
      subject: "Re: Hola",
      quotedHtml: "",
      reply: { mode: "reply", originalMessageId: 7 },
    } as never);
  });

  it("pone el correo en cola, muestra Enviando… y se cierra solo", async () => {
    vi.mocked(svc.QueueSend).mockResolvedValue(12);
    const user = setup();
    await user.click(screen.getByText("responder"));
    expect(await screen.findByLabelText("Para")).toHaveValue("Ana <ana@x.com>");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    expect(await screen.findByRole("button", { name: "Enviando…" })).toBeDisabled();
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.reply).toEqual({ mode: "reply", originalMessageId: 7 });
    expect(req.to).toEqual([{ name: "Ana", email: "ana@x.com" }]);
    expect(req.subject).toBe("Re: Hola");
    expect(req.draftId).toBeUndefined();
    expect(toast.loading).toHaveBeenCalledWith("Enviando correo…", { id: "send-12" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument(), {
      timeout: 2000,
    });
  });

  it("si no se puede poner en cola queda abierto con el error", async () => {
    vi.mocked(svc.QueueSend).mockRejectedValue(new Error("Agregá al menos un destinatario."));
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    expect(await screen.findByText("Agregá al menos un destinatario.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enviar" })).toBeEnabled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(toast.loading).not.toHaveBeenCalled();
  });

  it("deshabilita Enviar sin destinatarios", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    const send = await screen.findByRole("button", { name: "Enviar" });
    expect(send).toBeDisabled();
    await user.type(screen.getByLabelText("Para"), "a@b.com");
    expect(send).toBeEnabled();
  });

  it("un correo de la bandeja de salida se reintenta con su id", async () => {
    vi.mocked(svc.QueueSend).mockResolvedValue(5);
    const user = setup();
    await user.click(screen.getByText("borrador"));
    expect(await screen.findByLabelText("Asunto")).toHaveValue("Pendiente");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].draftId).toBe(5);
    expect(svc.DeleteLocalDraft).not.toHaveBeenCalled();
  });

  it("una dirección inválida deshabilita Enviar y la señala", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Para"), "a@b.com, sin-arroba");
    expect(screen.getByRole("button", { name: "Enviar" })).toBeDisabled();
    expect(screen.getByText("Revisá estas direcciones: sin-arroba")).toBeInTheDocument();
  });

  it("pide confirmación al cerrar con cambios", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Asunto"), "algo");
    await user.keyboard("{Escape}");
    expect(await screen.findByText("¿Descartar este correo?")).toBeInTheDocument();
  });

  it("Esc con el menú / abierto lo cierra y no cierra el redactor", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    const body = await screen.findByLabelText("Cuerpo del mensaje");
    act(() => body.focus());
    await user.keyboard("/ta");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("dialog", { name: "Nuevo correo" })).toBeInTheDocument();
    expect(screen.queryByText("¿Descartar este correo?")).toBeNull();
  });

  it("insertar imagen agrega el img al cuerpo enviado", async () => {
    vi.mocked(svc.PickImage).mockResolvedValue({ dataUrl: "data:image/png;base64,iVBORw0KGgo=", filename: "foto.png" } as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(3);
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    const body = await screen.findByLabelText("Cuerpo del mensaje");
    act(() => body.focus());
    await user.click(screen.getByRole("button", { name: "Imagen" }));
    await waitFor(() => expect(body.querySelector("img")).not.toBeNull());
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].bodyHtml).toContain("data:image/png");
  });

  it("cancelar el diálogo no cambia nada", async () => {
    vi.mocked(svc.PickImage).mockResolvedValue({ dataUrl: "", filename: "" } as never);
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    const body = await screen.findByLabelText("Cuerpo del mensaje");
    act(() => body.focus());
    await user.click(screen.getByRole("button", { name: "Imagen" }));
    await waitFor(() => expect(svc.PickImage).toHaveBeenCalled());
    expect(body.querySelector("img")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("Adjuntar suma chips y QueueSend lleva sus ids", async () => {
    vi.mocked(svc.PickAttachments).mockResolvedValue([staged(1, "a.pdf"), staged(2, "b.pdf")] as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(3);
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Adjuntar" }));
    expect(await screen.findByText("a.pdf")).toBeInTheDocument();
    expect(screen.getByText("b.pdf")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.attachments).toEqual([1, 2]);
    expect(req.skipOriginalAttachments).toEqual([]);
  });

  it("quitar un chip lo saca del envío", async () => {
    vi.mocked(svc.PickAttachments).mockResolvedValue([staged(1, "a.pdf"), staged(2, "b.pdf")] as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(3);
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Adjuntar" }));
    await screen.findByText("a.pdf");
    await user.click(screen.getByRole("button", { name: "Quitar a.pdf" }));
    expect(screen.queryByText("a.pdf")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].attachments).toEqual([2]);
  });

  it("cancelar el diálogo no suma nada", async () => {
    vi.mocked(svc.PickAttachments).mockResolvedValue([]);
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Adjuntar" }));
    await waitFor(() => expect(svc.PickAttachments).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: /^Quitar / })).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("reenviar muestra los adjuntos del original y quitarlos los manda en skipOriginalAttachments", async () => {
    vi.mocked(svc.PrepareCompose).mockResolvedValue({
      accountId: 1, to: [], cc: [], subject: "Fwd: Hola", quotedHtml: "",
      reply: { mode: "forward", originalMessageId: 7 },
      originalAttachments: [
        { index: 0, filename: "uno.pdf", mimeType: "application/pdf", size: 10, part: "2" },
        { index: 1, filename: "dos.pdf", mimeType: "application/pdf", size: 10, part: "3" },
      ],
    } as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(3);
    const user = setup();
    await user.click(screen.getByText("reenviar"));
    await screen.findByText("uno.pdf");
    expect(screen.getByText("dos.pdf")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Quitar dos.pdf" }));
    await user.type(screen.getByLabelText("Para"), "a@b.com");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.skipOriginalAttachments).toEqual([1]);
    expect(req.attachments).toEqual([]);
  });

  it("más de 25 MB muestra el aviso y deshabilita Enviar", async () => {
    vi.mocked(svc.PickAttachments).mockResolvedValue([staged(1, "grande.bin", 26 * MB)] as never);
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Adjuntar" }));
    expect(await screen.findByText("Los adjuntos suman 26.0 MB; el máximo es 25.0 MB.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enviar" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Quitar grande.bin" }));
    expect(screen.queryByText(/Los adjuntos suman/)).toBeNull();
    expect(screen.getByRole("button", { name: "Enviar" })).toBeEnabled();
  });

  it("files-staged suma adjuntos; con error muestra el mensaje", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await screen.findByLabelText("Para");
    act(() => wails.handlers.get("files-staged")?.({ data: { attachments: [staged(9, "soltado.pdf")], error: "" } }));
    expect(await screen.findByText("soltado.pdf")).toBeInTheDocument();
    act(() => wails.handlers.get("files-staged")?.({ data: { attachments: [], error: "No se pudo leer x.txt" } }));
    expect(await screen.findByText("No se pudo leer x.txt")).toBeInTheDocument();
  });

  it("reabrir un correo de la Bandeja de salida muestra sus adjuntos", async () => {
    vi.mocked(svc.StagedInfo).mockResolvedValue([staged(4, "guardado.pdf")] as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(5);
    const user = setup();
    await user.click(screen.getByText("borrador con adjuntos"));
    expect(await screen.findByText("guardado.pdf")).toBeInTheDocument();
    expect(svc.StagedInfo).toHaveBeenCalledWith([4]);
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.attachments).toEqual([4]);
    expect(req.skipOriginalAttachments).toEqual([2]);
    expect(req.draftId).toBe(5);
  });

  it("adjuntar marca el redactor como modificado", async () => {
    vi.mocked(svc.PickAttachments).mockResolvedValue([staged(1, "a.pdf")] as never);
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Adjuntar" }));
    await screen.findByText("a.pdf");
    await user.keyboard("{Escape}");
    expect(await screen.findByText("¿Descartar este correo?")).toBeInTheDocument();
  });

  it("reabrir un reenvío de la Bandeja de salida muestra los originales no salteados", async () => {
    vi.mocked(svc.PrepareCompose).mockResolvedValue({
      accountId: 1, to: [], cc: [], subject: "Fwd: Hola", quotedHtml: "",
      reply: { mode: "forward", originalMessageId: 7 },
      originalAttachments: [
        { index: 0, filename: "uno.pdf", mimeType: "application/pdf", size: 10, part: "2" },
        { index: 1, filename: "dos.pdf", mimeType: "application/pdf", size: 10, part: "3" },
      ],
    } as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(5);
    const user = setup();
    await user.click(screen.getByText("reenvío guardado"));
    await screen.findByText("dos.pdf");
    expect(screen.queryByText("uno.pdf")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Quitar dos.pdf" }));
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].skipOriginalAttachments).toEqual([0, 1]);
  });
});
