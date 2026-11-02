import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  AttachmentData: vi.fn(),
  SaveAttachment: vi.fn(() => Promise.resolve(true)),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Attachment } from "../../../bindings/postacontrol/core/model/models";
import { toast } from "sonner";
import { AttachmentList } from "./AttachmentList";

function att(index: number, filename: string, mimeType: string): Attachment {
  return { index, filename, mimeType, size: 10, part: [index + 1] };
}

const list = [
  att(0, "factura.pdf", "application/pdf"),
  att(1, "foto.jpg", "image/jpeg"),
  att(2, "fotos.zip", "application/zip"),
];

describe("AttachmentList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:x"), revokeObjectURL: vi.fn() });
    vi.mocked(svc.AttachmentData).mockImplementation(((_m: number, i: number) =>
      Promise.resolve({ filename: list[i].filename, mimeType: list[i].mimeType, data: "AAEC" })) as unknown as typeof svc.AttachmentData);
  });

  it("un clic en un PDF abre el visor y no descarga", async () => {
    const user = userEvent.setup();
    render(<AttachmentList messageId={7} attachments={list} />);
    await user.click(screen.getByRole("button", { name: /factura\.pdf/ }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAccessibleName("factura.pdf");
    expect(svc.AttachmentData).toHaveBeenCalledWith(7, 0);
    expect(svc.SaveAttachment).not.toHaveBeenCalled();
  });

  it("un clic en una imagen abre el visor en su índice", async () => {
    const user = userEvent.setup();
    render(<AttachmentList messageId={7} attachments={list} />);
    await user.click(screen.getByRole("button", { name: /foto\.jpg/ }));
    expect(await screen.findByRole("dialog")).toHaveAccessibleName("foto.jpg");
    expect(await screen.findByRole("img", { name: "foto.jpg" })).toBeInTheDocument();
    expect(svc.SaveAttachment).not.toHaveBeenCalled();
  });

  it("cerrar el visor lo saca de pantalla", async () => {
    const user = userEvent.setup();
    render(<AttachmentList messageId={7} attachments={list} />);
    await user.click(screen.getByRole("button", { name: /foto\.jpg/ }));
    await screen.findByRole("dialog");
    await user.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("un clic en un .zip lo guarda como hoy", async () => {
    const user = userEvent.setup();
    render(<AttachmentList messageId={7} attachments={list} />);
    await user.click(screen.getByRole("button", { name: /fotos\.zip/ }));
    expect(svc.SaveAttachment).toHaveBeenCalledWith(7, 2);
    expect(toast.success).toHaveBeenCalledWith("Adjunto guardado");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
