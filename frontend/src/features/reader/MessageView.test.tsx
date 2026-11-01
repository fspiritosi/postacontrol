import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetMessage: vi.fn(),
  ListFolders: vi.fn(),
  ContactForEmail: vi.fn(),
  AddSenderToContacts: vi.fn(),
}));
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => ({ openFrom: vi.fn() }) }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account, Contact, Folder, MessageDetail } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import type { ShellState } from "../layout/MailShell";
import { MessageView } from "./MessageView";

const account: Account = {
  id: 7, email: "yo@b.c", displayName: "Yo", username: "a", color: "#f00",
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
  defaultSignatureId: 0,
};

const ana = { name: "Ana", email: "ana@x.y" };
const contact: Contact = { id: 7, name: "Ana", emails: ["ana@x.y"], phones: [], company: "", notes: "" };

function setup(role: string) {
  const folder: Folder = {
    id: 3, accountId: 7, name: "F", delimiter: "/", role, unread: 0, total: 0, selectable: true,
  };
  const msg: MessageDetail = {
    id: 1, accountId: 7, folderId: 3, uid: 1, from: ana, subject: "Hola", snippet: "",
    date: "2026-10-20T10:00:00Z", seen: true, flagged: false, answered: false, hasAttachments: false,
    messageId: "", inReplyTo: "", references: null, to: null, cc: null, replyTo: null,
    html: "", text: "Cuerpo", hasRemoteImages: false, attachments: null,
  };
  vi.mocked(svc.GetMessage).mockResolvedValue(msg);
  vi.mocked(svc.ListFolders).mockResolvedValue([folder]);
  const openContact = vi.fn();
  const state = {
    view: { kind: "folder", folderId: 3 },
    messageId: 1,
    selectView: vi.fn(),
    selectMessage: vi.fn(),
    search: null,
    setSearch: vi.fn(),
    threadRoot: null,
    selectThread: vi.fn(),
    contactId: null,
    selectContact: vi.fn(),
    openContact,
  } as unknown as ShellState;
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MessageView accounts={[account]} state={state} />
    </QueryClientProvider>,
  );
  return { openContact, user: userEvent.setup() };
}

describe("MessageView: contactos", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ContactForEmail).mockResolvedValue(null);
  });

  it("agrega al remitente a contactos y avisa con el nombre de la ficha", async () => {
    vi.mocked(svc.AddSenderToContacts).mockResolvedValue(contact);
    const { user } = setup("inbox");
    await user.click(await screen.findByRole("button", { name: "Agregar a contactos" }));
    expect(svc.AddSenderToContacts).toHaveBeenCalledWith(ana);
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("«Ana» se agregó a tus contactos."),
    );
  });

  it("con el remitente ya en la libreta muestra Ver contacto", async () => {
    vi.mocked(svc.ContactForEmail).mockResolvedValue(contact);
    const { user, openContact } = setup("inbox");
    await user.click(await screen.findByRole("button", { name: "Ver contacto" }));
    expect(openContact).toHaveBeenCalledWith(7);
    expect(screen.queryByRole("button", { name: "Agregar a contactos" })).toBeNull();
  });

  it("si falla el alta muestra el error", async () => {
    vi.mocked(svc.AddSenderToContacts).mockRejectedValue(new Error("boom"));
    const { user } = setup("inbox");
    await user.click(await screen.findByRole("button", { name: "Agregar a contactos" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("en un borrador no hay botón de contactos", async () => {
    setup("drafts");
    await screen.findByText("Hola");
    await waitFor(() => expect(svc.ListFolders).toHaveBeenCalled());
    await screen.findByText("Ana");
    // Aparece un instante hasta que carga el rol de la carpeta; después se va.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Agregar a contactos" })).toBeNull();
      expect(screen.queryByRole("button", { name: "Ver contacto" })).toBeNull();
    });
  });
});
