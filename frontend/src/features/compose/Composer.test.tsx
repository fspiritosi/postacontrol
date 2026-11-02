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
  ListSignatures: vi.fn(),
  SaveDraft: vi.fn(),
  UploadDraft: vi.fn(),
  DeleteDraft: vi.fn(),
  SuggestRecipients: vi.fn(),
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
import type { Draft } from "../../../bindings/postacontrol/core/model/models";

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
        onClick={() => void c.openDraft({ ...draft, request: { ...draft.request, reply: { mode: "reply", originalMessageId: 7 } } } as never)}
      >
        respuesta guardada
      </button>
      <button
        onClick={() =>
          void c.openDraft({ ...draft, request: { ...draft.request, attachments: [4], skipOriginalAttachments: [2] } } as never)
        }
      >
        borrador con adjuntos
      </button>
      <button onClick={() => void c.openComposeDraft(composeDraft as never)}>seguir editando</button>
      <button onClick={() => void c.openComposeDraft(replyComposeDraft as never)}>seguir respuesta</button>
    </>
  );
}

// Un borrador autoguardado (no un correo de la bandeja de salida).
const composeDraft = {
  id: 8,
  messageId: "<b8@ej.com>",
  needsUpload: false,
  uploadError: "",
  updatedAt: "2026-10-31T10:00:00Z",
  request: {
    accountId: 1,
    to: [{ name: "", email: "z@x.com" }],
    cc: [],
    bcc: [],
    subject: "A medio escribir",
    bodyHtml: "<p>hola</p>",
    reply: null,
    signatureId: 0,
    composeDraftId: 8,
    draftMessageId: "<b8@ej.com>",
  },
};

// Un borrador de respuesta, con los encabezados de hilo que armó PrepareCompose.
const replyComposeDraft = {
  ...composeDraft,
  request: {
    ...composeDraft.request,
    subject: "Re: Hola",
    reply: { mode: "reply", originalMessageId: 7, inReplyTo: "<hola@ej.com>", references: ["<hola@ej.com>"] },
  },
};

const MB = 1024 * 1024;
const staged = (id: number, filename: string, size = 1000) => ({ id, filename, mimeType: "application/pdf", size });

let qc: QueryClient;

function setup() {
  const user = userEvent.setup();
  qc = new QueryClient();
  render(
    <QueryClientProvider client={qc}>
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
  // La cita se muestra en SafeHtmlFrame, que mide su contenido con ResizeObserver.
  // Radix Select (el selector de firma) usa APIs que jsdom no trae.
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});

describe("Composer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([]);
    vi.mocked(svc.ListSignatures).mockResolvedValue([]);
    vi.mocked(svc.DeleteLocalDraft).mockResolvedValue();
    vi.mocked(svc.SaveDraft).mockResolvedValue({ ...composeDraft, id: 9, messageId: "<b9@ej.com>" } as never);
    vi.mocked(svc.UploadDraft).mockResolvedValue({ ...composeDraft, id: 9, messageId: "<b9@ej.com>" } as never);
    vi.mocked(svc.DeleteDraft).mockResolvedValue();
    vi.mocked(svc.SuggestRecipients).mockResolvedValue([]);
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
    expect(await screen.findByTitle("Ana <ana@x.com>")).toHaveTextContent("Ana");
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

  it("Esc con las sugerencias abiertas las cierra y no cierra el redactor", async () => {
    vi.mocked(svc.SuggestRecipients).mockResolvedValue([{ name: "Juan", email: "juan@x.com", contactId: 1 }] as never);
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Para"), "jua");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("dialog", { name: "Nuevo correo" })).toBeInTheDocument();
    expect(screen.getByLabelText("Para")).toHaveValue("jua");
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
    await waitFor(() => expect(svc.SaveDraft).toHaveBeenCalled());
    expect(vi.mocked(svc.SaveDraft).mock.calls[0][1].attachments).toEqual([1]);
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

  it("reabrir una respuesta de la Bandeja de salida vuelve a mostrar la cita", async () => {
    vi.mocked(svc.PrepareCompose).mockResolvedValue({
      accountId: 1, to: [], cc: [], subject: "Re: Hola", quotedHtml: "<p>El original</p>",
      reply: { mode: "reply", originalMessageId: 7 }, originalAttachments: null,
    } as never);
    const user = setup();
    await user.click(screen.getByText("respuesta guardada"));
    expect(await screen.findByLabelText("Mensaje original")).toBeInTheDocument();
    expect(svc.PrepareCompose).toHaveBeenCalledWith(7, "reply");
  });

  it("si el original ya no está, la respuesta reabierta abre igual, sin cita", async () => {
    vi.mocked(svc.PrepareCompose).mockRejectedValue(new Error("no existe"));
    const user = setup();
    await user.click(screen.getByText("respuesta guardada"));
    await screen.findByLabelText("Para");
    expect(screen.queryByLabelText("Mensaje original")).toBeNull();
  });
});

describe("Composer: borradores", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([]);
    vi.mocked(svc.ListSignatures).mockResolvedValue([]);
    vi.mocked(svc.SaveDraft).mockResolvedValue({ ...composeDraft, id: 9, messageId: "<b9@ej.com>" } as never);
    vi.mocked(svc.UploadDraft).mockResolvedValue({ ...composeDraft, id: 9, messageId: "<b9@ej.com>" } as never);
    vi.mocked(svc.DeleteDraft).mockResolvedValue();
    vi.mocked(svc.QueueSend).mockResolvedValue(30);
  });

  it("cerrar con contenido guarda y avisa Guardado en Borradores", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Asunto"), "algo");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    const [id, req] = vi.mocked(svc.SaveDraft).mock.calls[0];
    expect(id).toBe(0);
    expect(req.subject).toBe("algo");
    expect(toast.success).toHaveBeenCalledWith("Guardado en Borradores.");
    expect(screen.queryByText("¿Descartar este correo?")).toBeNull();
    // Y lo sube ya, sin esperar el timer.
    await waitFor(() => expect(svc.UploadDraft).toHaveBeenCalledWith(9));
  });

  it("cerrar vacío no crea borrador", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    const subject = await screen.findByLabelText("Asunto");
    await user.type(subject, "a");
    await user.clear(subject);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("Descartar pide confirmación y llama a DeleteDraft", async () => {
    const user = setup();
    await user.click(screen.getByText("seguir editando"));
    expect(await screen.findByLabelText("Asunto")).toHaveValue("A medio escribir");
    await user.click(screen.getByRole("button", { name: "Descartar" }));
    expect(await screen.findByText("¿Descartar este borrador?")).toBeInTheDocument();
    expect(screen.getByText("Se borra también de la carpeta Borradores del servidor.")).toBeInTheDocument();
    expect(svc.DeleteDraft).not.toHaveBeenCalled();
    const confirm = screen.getAllByRole("button", { name: "Descartar" }).slice(-1)[0];
    await user.click(confirm);
    await waitFor(() => expect(svc.DeleteDraft).toHaveBeenCalledWith(8));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("si no se pudo borrar del servidor avisa y cierra igual", async () => {
    vi.mocked(svc.DeleteDraft).mockRejectedValue(
      new Error("Borramos el borrador de esta compu, pero no pudimos borrarlo del servidor."),
    );
    const user = setup();
    await user.click(screen.getByText("seguir editando"));
    await screen.findByLabelText("Asunto");
    await user.click(screen.getByRole("button", { name: "Descartar" }));
    await screen.findByText("¿Descartar este borrador?");
    await user.click(screen.getAllByRole("button", { name: "Descartar" }).slice(-1)[0]);
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "Borramos el borrador de esta compu, pero no pudimos borrarlo del servidor.",
      ),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("enviar lleva composeDraftId y draftMessageId", async () => {
    const user = setup();
    await user.click(screen.getByText("seguir editando"));
    expect(await screen.findByTitle("z@x.com")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.composeDraftId).toBe(8);
    expect(req.draftMessageId).toBe("<b8@ej.com>");
    expect(req.draftId).toBeUndefined();
    expect(req.subject).toBe("A medio escribir");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument(), { timeout: 2000 });
    expect(svc.SaveDraft).not.toHaveBeenCalled();
  });

  it("un borrador de respuesta cuyo original ya no está se abre igual, sin cita, y conserva el hilo", async () => {
    vi.mocked(svc.PrepareCompose).mockRejectedValue(new Error("No encontramos lo que buscabas."));
    const user = setup();
    await user.click(screen.getByText("seguir respuesta"));
    expect(await screen.findByTitle("z@x.com")).toBeInTheDocument();
    expect(screen.queryByLabelText("Mensaje original")).toBeNull();
    expect(toast.error).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].reply).toEqual({
      mode: "reply", originalMessageId: 7, inReplyTo: "<hola@ej.com>", references: ["<hola@ej.com>"],
    });
  });

  it("enviar espera el guardado en vuelo antes de QueueSend", async () => {
    let finishSave: (d: Draft) => void = () => {};
    vi.mocked(svc.SaveDraft).mockReturnValueOnce(
      new Promise<Draft>((r) => (finishSave = r)) as ReturnType<typeof svc.SaveDraft>,
    );
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Para"), "a@b.com");
    // El guardado local arranca a los 1500 ms y queda en vuelo.
    await waitFor(() => expect(svc.SaveDraft).toHaveBeenCalledTimes(1), { timeout: 3000 });
    expect(await screen.findByText("Guardando…")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    expect(svc.QueueSend).not.toHaveBeenCalled();
    await act(async () => finishSave({ ...composeDraft, id: 21, messageId: "<n21@ej.com>" } as never));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalledTimes(1));
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.composeDraftId).toBe(21);
    expect(req.draftMessageId).toBe("<n21@ej.com>");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument(), { timeout: 2000 });
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    expect(svc.UploadDraft).not.toHaveBeenCalled();
  });

  it("muestra el estado del borrador en el pie", async () => {
    vi.mocked(svc.UploadDraft).mockResolvedValue({ ...composeDraft, needsUpload: true } as never);
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Asunto"), "algo");
    expect(await screen.findByText("Guardado", {}, { timeout: 3000 })).toBeInTheDocument();
  });

  it("el reintento de la Bandeja de salida no autoguarda y sigue preguntando", async () => {
    const user = setup();
    await user.click(screen.getByText("borrador"));
    await user.type(await screen.findByLabelText("Asunto"), " más");
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Descartar" })).toBeNull();
    await user.keyboard("{Escape}");
    expect(await screen.findByText("¿Descartar este correo?")).toBeInTheDocument();
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    expect(svc.UploadDraft).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
    expect(screen.queryByText(/^Guardado/)).toBeNull();
  });
});

describe("Composer: firma", () => {
  const sigs = [
    { id: 10, name: "Trabajo", html: "<p>Saludos, Yo (trabajo)</p>" },
    { id: 11, name: "Personal", html: "<p>Abrazo, Yo (personal)</p>" },
  ];
  const acc2 = { ...account, id: 2, email: "otra@ej.com", displayName: "Otra", defaultSignatureId: 11 };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([{ ...account, defaultSignatureId: 10 }, acc2] as never);
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([]);
    vi.mocked(svc.ListSignatures).mockResolvedValue(sigs as never);
    vi.mocked(svc.QueueSend).mockResolvedValue(9);
    vi.mocked(svc.SaveDraft).mockResolvedValue({ ...composeDraft, id: 9 } as never);
    vi.mocked(svc.UploadDraft).mockResolvedValue({ ...composeDraft, id: 9 } as never);
  });

  const preview = () => screen.queryByRole("region", { name: "Vista previa de la firma" });

  it("un correo nuevo arranca con la firma por defecto de la cuenta", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    expect(await screen.findByLabelText("Firma")).toHaveTextContent("Trabajo");
    await waitFor(() => expect(preview()).toBeInTheDocument());
    expect(preview()!.querySelector("iframe")!.getAttribute("srcdoc")).toContain("Saludos, Yo (trabajo)");
    await user.type(screen.getByLabelText("Para"), "a@b.com");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].signatureId).toBe(10);
  });

  it("cambiar De cambia la firma salvo que se haya elegido a mano", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await screen.findByLabelText("Firma");
    await waitFor(() => expect(screen.getByLabelText("Firma")).toHaveTextContent("Trabajo"));
    await user.click(screen.getByLabelText("De"));
    await user.click(await screen.findByRole("option", { name: /otra@ej.com/ }));
    await waitFor(() => expect(screen.getByLabelText("Firma")).toHaveTextContent("Personal"));
    // Elegida a mano: ya no sigue a la cuenta.
    await user.click(screen.getByLabelText("Firma"));
    await user.click(await screen.findByRole("option", { name: "Trabajo" }));
    await user.click(screen.getByLabelText("De"));
    await user.click(await screen.findByRole("option", { name: /yo@ej.com/ }));
    expect(screen.getByLabelText("Firma")).toHaveTextContent("Trabajo");
  });

  it("Sin firma oculta la vista previa y manda signatureId 0", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await waitFor(() => expect(preview()).toBeInTheDocument());
    await user.click(screen.getByLabelText("Firma"));
    await user.click(await screen.findByRole("option", { name: "Sin firma" }));
    expect(preview()).toBeNull();
    await user.type(screen.getByLabelText("Para"), "a@b.com");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].signatureId).toBe(0);
  });

  it("responder usa la firma de PrepareCompose", async () => {
    vi.mocked(svc.PrepareCompose).mockResolvedValue({
      accountId: 1, to: [{ name: "Ana", email: "ana@x.com" }], cc: [], subject: "Re: Hola",
      quotedHtml: "", reply: { mode: "reply", originalMessageId: 7 }, signatureId: 11,
    } as never);
    const user = setup();
    await user.click(screen.getByText("responder"));
    await waitFor(() => expect(screen.getByLabelText("Firma")).toHaveTextContent("Personal"));
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalled());
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].signatureId).toBe(11);
  });

  it("una firma borrada pasa a Sin firma", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await waitFor(() => expect(preview()).toBeInTheDocument());
    vi.mocked(svc.ListSignatures).mockResolvedValue([sigs[1]] as never);
    await act(() => qc.invalidateQueries({ queryKey: ["signatures"] }));
    await waitFor(() => expect(screen.getByLabelText("Firma")).toHaveTextContent("Sin firma"));
    expect(preview()).toBeNull();
  });
});
