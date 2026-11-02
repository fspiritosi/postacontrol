import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  AttachmentData: vi.fn(),
  SaveAttachment: vi.fn(() => Promise.resolve(true)),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
// pdf.js no corre en jsdom: el PdfView de prueba avisa 3 páginas al montarse
// y destroy cuenta los desmontajes.
const pdfDestroy = vi.fn();
vi.mock("./PdfView", async () => {
  const { useEffect, useRef } = await import("react");
  return {
    PdfView: ({ data, onPageChange }: { data: Uint8Array; onPageChange?: (p: number, t: number) => void }) => {
      const first = useRef(onPageChange);
      useEffect(() => {
        first.current?.(1, 3);
        return () => pdfDestroy();
      }, []);
      return (
        <div>
          <p>PDF de {data.length} bytes</p>
          <button type="button" onClick={() => onPageChange?.(2, 3)}>
            bajar
          </button>
        </div>
      );
    },
  };
});

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Attachment, AttachmentData } from "../../../bindings/postacontrol/core/model/models";
import { AttachmentViewer } from "./AttachmentViewer";

function att(index: number, filename: string, mimeType: string): Attachment {
  return { index, filename, mimeType, size: 10, part: [index + 1] };
}

const png = (i: number, name = "foto.png") => att(i, name, "image/png");
const pdf = (i: number, name = "factura.pdf") => att(i, name, "application/pdf");
const xlsx = (i: number, name = "planilla.xlsx") =>
  att(i, name, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");

function data(a: Attachment, b64: string | null = "AAEC"): AttachmentData {
  return { filename: a.filename, mimeType: a.mimeType, data: b64 };
}

// AttachmentData responde según el índice pedido.
function serve(list: Attachment[]) {
  vi.mocked(svc.AttachmentData).mockImplementation(((_m: number, i: number) =>
    Promise.resolve(data(list[i]))) as unknown as typeof svc.AttachmentData);
}

let blobs = 0;
const createObjectURL = vi.fn(() => `blob:test-${++blobs}`);
const revokeObjectURL = vi.fn();

function setup(list: Attachment[], index = 0) {
  const onClose = vi.fn();
  const user = userEvent.setup();
  const r = render(
    <AttachmentViewer messageId={7} attachments={list} index={index} onClose={onClose} />,
  );
  return { onClose, user, ...r };
}

describe("AttachmentViewer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    blobs = 0;
    Object.assign(URL, { createObjectURL, revokeObjectURL });
  });
  afterEach(() => vi.restoreAllMocks());

  it("muestra Cargando… y después la imagen", async () => {
    let resolve!: (d: AttachmentData) => void;
    vi.mocked(svc.AttachmentData).mockReturnValue(
      new Promise<AttachmentData>((r) => (resolve = r)) as ReturnType<typeof svc.AttachmentData>,
    );
    setup([png(0)]);
    expect(screen.getByText("Cargando…")).toBeInTheDocument();
    await act(async () => resolve(data(png(0))));
    const img = await screen.findByRole("img", { name: "foto.png" });
    expect(img).toHaveAttribute("src", "blob:test-1");
    expect(screen.queryByText("Cargando…")).not.toBeInTheDocument();
    expect(svc.AttachmentData).toHaveBeenCalledWith(7, 0);
  });

  it("el error del núcleo se muestra y Descargar sigue", async () => {
    vi.mocked(svc.AttachmentData).mockRejectedValue(new Error("Es muy grande para verlo acá."));
    const { user } = setup([png(0), png(1, "otra.png")], 1);
    expect(await screen.findByText("Es muy grande para verlo acá.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Descargar" }));
    expect(svc.SaveAttachment).toHaveBeenCalledWith(7, 1);
  });

  it("un adjunto sin datos muestra un aviso y no rompe", async () => {
    vi.mocked(svc.AttachmentData).mockResolvedValue(data(png(0), null));
    setup([png(0)]);
    expect(await screen.findByText("El adjunto está vacío.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Descargar" })).toBeInTheDocument();
  });

  it("la tira lista todos los adjuntos y los no visibles van apagados", async () => {
    const list = [png(0), xlsx(1)];
    serve(list);
    const { user } = setup(list);
    await screen.findByRole("img", { name: "foto.png" });
    expect(screen.getByText("Adjuntos:")).toBeInTheDocument();
    const cur = screen.getByRole("button", { name: /foto\.png/ });
    expect(cur).toHaveAttribute("data-viewable", "true");
    expect(cur).toHaveAttribute("aria-current", "true");
    const chip = screen.getByRole("button", { name: /planilla\.xlsx/ });
    expect(chip).toHaveAttribute("data-viewable", "false");
    await user.click(chip);
    expect(svc.SaveAttachment).toHaveBeenCalledWith(7, 1);
    expect(svc.AttachmentData).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("img", { name: "foto.png" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "foto.png" })).toBeInTheDocument();
  });

  it("un clic en otro visible lo abre", async () => {
    const list = [png(0), png(1, "plano.png")];
    serve(list);
    const { user } = setup(list);
    await screen.findByRole("img", { name: "foto.png" });
    await user.click(screen.getByRole("button", { name: /plano\.png/ }));
    expect(await screen.findByRole("img", { name: "plano.png" })).toBeInTheDocument();
    expect(svc.AttachmentData).toHaveBeenLastCalledWith(7, 1);
  });

  it("flechas pasan entre los visibles", async () => {
    const list = [pdf(0), xlsx(1), png(2)];
    serve(list);
    const { user } = setup(list);
    expect(await screen.findByText("PDF de 3 bytes")).toBeInTheDocument();
    await user.keyboard("{ArrowRight}");
    expect(await screen.findByRole("img", { name: "foto.png" })).toBeInTheDocument();
    expect(svc.AttachmentData).toHaveBeenLastCalledWith(7, 2);
    // Sin más visibles a la derecha: se queda.
    await user.keyboard("{ArrowRight}");
    expect(svc.AttachmentData).toHaveBeenCalledTimes(2);
    await user.keyboard("{ArrowLeft}");
    expect(await screen.findByText("PDF de 3 bytes")).toBeInTheDocument();
    expect(svc.AttachmentData).toHaveBeenLastCalledWith(7, 0);
    expect(screen.getByRole("heading", { name: "factura.pdf" })).toBeInTheDocument();
  });

  it("PDF: muestra pág. N de M y la reinicia al pasar a otro adjunto", async () => {
    const list = [pdf(0), pdf(1, "otro.pdf"), png(2)];
    serve(list);
    const { user } = setup(list);
    expect(await screen.findByText("pág. 1 de 3")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "bajar" }));
    expect(screen.getByText("pág. 2 de 3")).toBeInTheDocument();

    // Otro PDF: el anterior se desmonta y la página vuelve a 1.
    await user.click(screen.getByRole("button", { name: /otro\.pdf/ }));
    await screen.findByRole("heading", { name: "otro.pdf" });
    expect(await screen.findByText("pág. 1 de 3")).toBeInTheDocument();
    expect(pdfDestroy).toHaveBeenCalled();

    // Una imagen no tiene páginas.
    await user.keyboard("{ArrowRight}");
    await screen.findByRole("img", { name: "foto.png" });
    expect(screen.queryByText(/^pág\./)).not.toBeInTheDocument();
  });

  it("zoom: Ajustar, + y −", async () => {
    serve([png(0)]);
    const { user } = setup([png(0)]);
    await screen.findByRole("img", { name: "foto.png" });
    const plus = screen.getByRole("button", { name: "Acercar" });
    const minus = screen.getByRole("button", { name: "Alejar" });
    expect(screen.getByText("Ajustar")).toBeInTheDocument();
    await user.click(plus);
    expect(screen.getByText("100%")).toBeInTheDocument();
    await user.click(plus);
    expect(screen.getByText("125%")).toBeInTheDocument();
    await user.click(minus);
    expect(screen.getByText("100%")).toBeInTheDocument();
    await user.click(minus);
    expect(screen.getByText("75%")).toBeInTheDocument();
    await user.click(minus);
    expect(screen.getByText("50%")).toBeInTheDocument();
    await user.click(minus);
    expect(screen.getByText("Ajustar")).toBeInTheDocument();
    // Desde Ajustar, − no hace nada.
    await user.click(minus);
    expect(screen.getByText("Ajustar")).toBeInTheDocument();
  });

  it("zoom: + topa en 300%", async () => {
    serve([png(0)]);
    const { user } = setup([png(0)]);
    await screen.findByRole("img", { name: "foto.png" });
    const plus = screen.getByRole("button", { name: "Acercar" });
    for (let i = 0; i < 9; i++) await user.click(plus);
    expect(screen.getByText("300%")).toBeInTheDocument();
  });

  it("Esc cierra", async () => {
    serve([png(0)]);
    const { user, onClose } = setup([png(0)]);
    await screen.findByRole("img", { name: "foto.png" });
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });

  it("✕ cierra", async () => {
    serve([png(0)]);
    const { user, onClose } = setup([png(0)]);
    await screen.findByRole("img", { name: "foto.png" });
    await user.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("revoca el blob URL al cambiar o cerrar", async () => {
    const list = [png(0), png(1, "plano.png")];
    serve(list);
    const { user, unmount } = setup(list);
    expect(await screen.findByRole("img", { name: "foto.png" })).toHaveAttribute("src", "blob:test-1");
    await user.click(screen.getByRole("button", { name: /plano\.png/ }));
    expect(await screen.findByRole("img", { name: "plano.png" })).toHaveAttribute("src", "blob:test-2");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test-1");
    expect(revokeObjectURL).not.toHaveBeenCalledWith("blob:test-2");
    unmount();
    await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith("blob:test-2"));
  });
});
