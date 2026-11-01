import { Editor } from "@tiptap/core";
import { NodeSelection } from "@tiptap/pm/state";
import StarterKit from "@tiptap/starter-kit";
import { afterEach, describe, expect, it } from "vitest";
import { EditorImage, moveSelectedNode, selectedImage } from "./editorImage";

const PNG = "data:image/png;base64,iVBORw0KGgo=";
let editor: Editor | undefined;
afterEach(() => editor?.destroy());

const make = (content: string) => {
  editor = new Editor({ extensions: [StarterKit, EditorImage], content });
  return editor;
};

// Selecciona la primera imagen del documento.
const selectImage = (ed: Editor) => {
  let pos = -1;
  ed.state.doc.descendants((n, p) => {
    if (pos < 0 && n.type.name === "image") pos = p;
  });
  ed.view.dispatch(ed.state.tr.setSelection(NodeSelection.create(ed.state.doc, pos)));
};

describe("EditorImage", () => {
  it("conserva align left/right, ancho y alto", () => {
    expect(make(`<img src="${PNG}" align="left" width="120" height="40">`).getHTML()).toBe(
      `<img src="${PNG}" width="120" height="40" align="left">`,
    );
    expect(make(`<img src="${PNG}" align="RIGHT">`).getHTML()).toContain('align="right"');
  });

  it("descarta otros valores de align", () => {
    expect(make(`<img src="${PNG}" align="middle">`).getHTML()).not.toContain("align");
  });

  it("descarta las imágenes remotas", () => {
    expect(make('<p>a</p><img src="https://x.test/a.png">').getHTML()).not.toContain("<img");
  });

  it("el contenedor del node view lleva data-align y se actualiza", () => {
    const ed = make(`<img src="${PNG}">`);
    const container = () => ed.view.dom.querySelector("[data-resize-container]") as HTMLElement;
    expect(container().dataset.align).toBeUndefined();
    selectImage(ed);
    ed.commands.updateAttributes("image", { align: "left" });
    expect(container().dataset.align).toBe("left");
    ed.commands.updateAttributes("image", { align: null });
    expect(container().dataset.align).toBeUndefined();
  });
});

describe("moveSelectedNode", () => {
  it("sube y baja la imagen de a un bloque y la deja seleccionada", () => {
    const ed = make(`<p>a</p><img src="${PNG}"><p>b</p>`);
    selectImage(ed);
    expect(moveSelectedNode(ed, -1)).toBe(true);
    expect(ed.getHTML()).toBe(`<img src="${PNG}"><p>a</p><p>b</p>`);
    expect(selectedImage(ed)).not.toBeNull();
    expect(moveSelectedNode(ed, -1)).toBe(false);
    expect(moveSelectedNode(ed, 1)).toBe(true);
    expect(moveSelectedNode(ed, 1)).toBe(true);
    // TipTap deja un renglón vacío al final para seguir escribiendo; no se pasa.
    expect(ed.getHTML()).toBe(`<p>a</p><p>b</p><img src="${PNG}"><p></p>`);
    expect(moveSelectedNode(ed, 1)).toBe(false);
    expect(ed.getHTML()).toBe(`<p>a</p><p>b</p><img src="${PNG}"><p></p>`);
  });

  it("sin un nodo seleccionado no hace nada", () => {
    const ed = make("<p>a</p><p>b</p>");
    expect(moveSelectedNode(ed, 1)).toBe(false);
    expect(selectedImage(ed)).toBeNull();
  });
});
