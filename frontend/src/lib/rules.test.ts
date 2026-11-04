import { describe, expect, it } from "vitest";
import type { Folder, Label, Rule } from "../../bindings/postacontrol/core/model/models";
import { describeRule, moveTargets, ruleDraftFromSender } from "./rules";

const f = (id: number, name: string, role = "", selectable = true) =>
  ({ id, accountId: 1, name, delimiter: "/", role, selectable, unread: 0, total: 0, position: 0 }) as Folder;

describe("reglas", () => {
  it("moveTargets da la ruta completa y saltea lo que no admite correos", () => {
    const folders = [f(1, "INBOX", "inbox"), f(2, "Clientes", "", false), f(3, "Clientes/Acme"), f(4, "Trash", "trash")];
    const paths = moveTargets(folders).map((t) => t.path);
    expect(paths).toContain("Entrada");
    expect(paths).toContain("Clientes / Acme");
    expect(paths).not.toContain("Clientes");
  });

  it("describeRule une condiciones con y/o y nombra carpeta y etiqueta", () => {
    const labels = [{ id: 7, name: "Banco", color: "#000000", position: 1 }] as Label[];
    const r = {
      matchAll: false,
      conditions: [
        { field: "from", op: "is", value: "a@ej.com" },
        { field: "subject", op: "contains", value: "factura" },
      ],
      actions: [
        { type: "markRead", folderId: 0, labelId: 0 },
        { type: "label", folderId: 0, labelId: 7 },
        { type: "move", folderId: 3, labelId: 0 },
      ],
    } as Rule;
    expect(describeRule(r, [f(3, "Clientes/Acme")], labels)).toBe(
      "Si De es «a@ej.com» o Asunto contiene «factura» → Marcar como leído, Etiquetar Banco, Mover a Clientes / Acme",
    );
  });

  it("ruleDraftFromSender usa el nombre o, si no hay, el correo", () => {
    expect(ruleDraftFromSender(2, { name: "Bea", email: "bea@ej.com" })).toEqual({
      accountId: 2,
      name: "Bea",
      conditions: [{ field: "from", op: "is", value: "bea@ej.com" }],
    });
    expect(ruleDraftFromSender(2, { name: "", email: "bea@ej.com" }).name).toBe("bea@ej.com");
  });
});
