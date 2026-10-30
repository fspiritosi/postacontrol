import { describe, expect, it } from "vitest";
import type { Folder } from "../../bindings/postacontrol/core/model/models";
import { buildFolderTree } from "./folders";

let nextId = 1;
const f = (name: string, role = ""): Folder => ({
  id: nextId++,
  accountId: 1,
  name,
  delimiter: ".",
  role,
  unread: 0,
  total: 0,
  selectable: true,
});

describe("buildFolderTree", () => {
  it("ordena por rol y traduce", () => {
    const tree = buildFolderTree([
      f("Trash", "trash"),
      f("INBOX", "inbox"),
      f("Zeta"),
      f("INBOX.Sent", "sent"),
      f("Alfa"),
    ]);
    expect(tree.map((n) => n.label)).toEqual([
      "Entrada",
      "Enviados",
      "Papelera",
      "Alfa",
      "Zeta",
    ]);
  });

  it("anida subcarpetas", () => {
    const tree = buildFolderTree([
      f("INBOX", "inbox"),
      f("INBOX.Clientes"),
      f("INBOX.Clientes.ACME"),
    ]);
    expect(tree).toHaveLength(1);
    const clientes = tree[0].children[0];
    expect(clientes.label).toBe("Clientes");
    expect(clientes.children.map((c) => c.label)).toEqual(["ACME"]);
  });

  it("anida bajo la carpeta de rol y sintetiza padres faltantes", () => {
    const tree = buildFolderTree([f("INBOX", "inbox"), f("INBOX.A.B")]);
    const a = tree[0].children[0];
    expect(a.label).toBe("A");
    expect(a.selectable).toBe(false);
    expect(a.children[0].label).toBe("B");
  });
});
