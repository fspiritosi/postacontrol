import { describe, expect, it } from "vitest";
import {
  accountMime,
  dragAccount,
  flatDrag,
  MSG_MIME,
  readMessageDrag,
  setMessageDrag,
} from "./dnd";
import type { MessageSummary } from "../../bindings/postacontrol/core/model/models";
import { FakeDataTransfer } from "../test/dataTransfer";

const dt = () => new FakeDataTransfer() as unknown as DataTransfer;

describe("dnd", () => {
  it("ida y vuelta del arrastre de correos", () => {
    const d = dt();
    setMessageDrag(d, 7, { ids: [1, 2], folderId: 3 });
    expect(d.types).toContain(MSG_MIME);
    expect(readMessageDrag(d)).toEqual({ ids: [1, 2], folderId: 3 });
  });

  it("ida y vuelta de conversaciones", () => {
    const d = dt();
    setMessageDrag(d, 7, { roots: ["a", "b"], folderId: 3 });
    expect(readMessageDrag(d)).toEqual({ roots: ["a", "b"], folderId: 3 });
  });

  it("la cuenta viaja en el tipo, legible en dragover", () => {
    const d = dt();
    setMessageDrag(d, 7, { ids: [1] });
    expect(d.types).toContain(accountMime(7));
    expect(accountMime(7)).toBe("application/x-postacontrol-account-7");
    expect(dragAccount(d)).toBe(7);
  });

  it("sin datos nuestros no hay arrastre ni cuenta", () => {
    const d = dt();
    d.setData("text/plain", "hola");
    expect(readMessageDrag(d)).toBeNull();
    expect(dragAccount(d)).toBeNull();
  });

  it("datos rotos no rompen", () => {
    const d = dt();
    d.setData(MSG_MIME, "{no es json");
    expect(readMessageDrag(d)).toBeNull();
  });

  describe("flatDrag", () => {
    const m = (id: number, accountId: number, folderId = 3) =>
      ({ id, accountId, folderId }) as MessageSummary;
    const items = [m(1, 7), m(2, 7), m(3, 7), m(4, 8)];

    it("una fila marcada lleva todos los marcados cargados", () => {
      expect(flatDrag(items, [1, 2, 99], items[0])).toEqual({
        accountId: 7,
        payload: { ids: [1, 2], folderId: 3 },
      });
    });

    it("una fila sin marcar va sola", () => {
      expect(flatDrag(items, [1, 2], items[2])?.payload.ids).toEqual([3]);
    });

    it("marcados de varias cuentas no se arrastran", () => {
      expect(flatDrag(items, [1, 4], items[0])).toBeNull();
    });
  });
});
