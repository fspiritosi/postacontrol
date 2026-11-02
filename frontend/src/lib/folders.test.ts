import { describe, expect, it } from "vitest";
import type { Folder } from "../../bindings/postacontrol/core/model/models";
import {
  buildFolderTree,
  canModify,
  folderLabel,
  isFirstLevel,
  isSpecialTree,
  moveTargets,
  siblingsOf,
} from "./folders";

let nextId = 1;
const f = (name: string, role = ""): Folder => ({
  id: nextId++,
  accountId: 1,
  name,
  delimiter: ".",
  role,
  unread: 0,
  total: 0,
  selectable: true, position: 0,
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

describe("folderLabel", () => {
  it("traduce por rol y usa el último segmento sin rol", () => {
    expect(folderLabel(f("INBOX.Sent", "sent"))).toBe("Enviados");
    expect(folderLabel(f("INBOX.Clientes.ACME"))).toBe("ACME");
    expect(folderLabel({ name: "Plano", delimiter: "", role: "" })).toBe("Plano");
  });
});

describe("orden por position", () => {
  const p = (name: string, position: number, role = ""): Folder => ({ ...f(name, role), position });

  it("los de position > 0 van primero y los de 0 al final", () => {
    const tree = buildFolderTree([p("INBOX", 0, "inbox"), p("A", 0), p("B", 2), p("C", 1)]);
    expect(tree.map((n) => n.name)).toEqual(["C", "B", "INBOX", "A"]);
  });

  it("en la raíz sin posiciones queda el orden de siempre", () => {
    const tree = buildFolderTree([f("Zeta"), f("Trash", "trash"), f("INBOX", "inbox"), f("Alfa")]);
    expect(tree.map((n) => n.name)).toEqual(["INBOX", "Trash", "Alfa", "Zeta"]);
  });

  it("también ordena los hijos", () => {
    const tree = buildFolderTree([f("P"), p("P.b", 1), p("P.a", 2), f("P.c")]);
    expect(tree[0].children.map((n) => n.name)).toEqual(["P.b", "P.a", "P.c"]);
  });
});

describe("isSpecialTree, canModify, siblingsOf y moveTargets", () => {
  const list = [
    f("INBOX", "inbox"),
    f("Clientes"),
    f("Clientes.Acme"),
    f("Clientes.Acme.2025"),
    f("Otra"),
    f("Mixta"),
    f("Mixta.Enviados", "sent"),
    f("MixtaVieja"),
    f("Sola.Hija"),
    f("[Gmail]"),
    f("[Gmail].Sent Mail", "sent"),
    f("[Gmail].Trash", "trash"),
  ];
  const tree = () => buildFolderTree(list);
  const find = (list: ReturnType<typeof tree>, name: string): ReturnType<typeof tree>[number] => {
    for (const n of list) {
      if (n.name === name) return n;
      const r = n.children.length ? find(n.children, name) : undefined;
      if (r) return r;
    }
    return undefined as never;
  };

  it("isSpecialTree mira al nodo y a lo que cuelga de él por nombre", () => {
    const t = tree();
    expect(isSpecialTree(list, find(t, "INBOX"))).toBe(true);
    // Sus especiales se ven en la raíz, pero siguen colgando de ella.
    expect(isSpecialTree(list, find(t, "Mixta"))).toBe(true);
    expect(isSpecialTree(list, find(t, "[Gmail]"))).toBe(true);
    expect(isSpecialTree(list, find(t, "MixtaVieja"))).toBe(false);
    expect(isSpecialTree(list, find(t, "Clientes"))).toBe(false);
    expect(canModify(list, find(t, "Clientes"))).toBe(true);
    expect(canModify(list, find(t, "MixtaVieja"))).toBe(true);
    expect(canModify(list, find(t, "INBOX"))).toBe(false);
    expect(canModify(list, find(t, "Mixta"))).toBe(false);
    expect(canModify(list, find(t, "[Gmail]"))).toBe(false);
    expect(canModify(list, find(t, "Sola"))).toBe(false); // sintetizada, id 0
  });

  it("siblingsOf devuelve el nivel del nodo", () => {
    const t = tree();
    const acme = find(t, "Clientes.Acme");
    expect(siblingsOf(t, acme).map((n) => n.name)).toEqual(["Clientes.Acme"]);
    expect(siblingsOf(t, find(t, "Otra")).map((n) => n.name)).toContain("Clientes");
  });

  it("moveTargets excluye al nodo, sus descendientes, su padre y los sintetizados", () => {
    const t = tree();
    const names = moveTargets(t, find(t, "Clientes.Acme")).map((n) => n.name);
    expect(names).not.toContain("Clientes.Acme");
    expect(names).not.toContain("Clientes.Acme.2025");
    expect(names).not.toContain("Clientes");
    expect(names).not.toContain("Sola");
    expect(names).toContain("Otra");
    expect(moveTargets(t, find(t, "Clientes")).map((n) => n.name)).not.toContain("Clientes.Acme");
  });
});

describe("isFirstLevel", () => {
  it("sin padre o colgando de INBOX (el prefijo personal) ya está en el primer nivel", () => {
    expect(isFirstLevel(f("Clientes"))).toBe(true);
    expect(isFirstLevel(f("INBOX.Clientes"))).toBe(true);
    expect(isFirstLevel(f("inbox.Clientes"))).toBe(true);
    expect(isFirstLevel({ name: "Plano.Algo", delimiter: "" })).toBe(true);
    expect(isFirstLevel(f("Clientes.Acme"))).toBe(false);
    expect(isFirstLevel(f("INBOX.Clientes.Acme"))).toBe(false);
  });
});
