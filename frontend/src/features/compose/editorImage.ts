import Image from "@tiptap/extension-image";
import { NodeSelection } from "@tiptap/pm/state";
import type { Node as PMNode } from "@tiptap/pm/model";
import type { Editor } from "@tiptap/react";

// left y right: la imagen flota y el texto la rodea. null: va sola en su renglón.
export type ImageAlign = "left" | "right" | null;

const parseAlign = (v: string | null): ImageAlign => {
  const a = v?.toLowerCase();
  return a === "left" || a === "right" ? a : null;
};

// El node view redimensionable envuelve el <img>: el flotado va en ese contenedor.
const syncAlign = (dom: HTMLElement, node: PMNode) => {
  const align = node.attrs.align as ImageAlign;
  if (align) dom.dataset.align = align;
  else delete dom.dataset.align;
};

// EditorImage es la imagen del redactor: solo embebida (una remota pegada se
// descarta al parsear), redimensionable desde las esquinas y con alineación.
// La alineación viaja como el atributo align de HTML: lo respetan el saneo,
// Outlook y los demás clientes; el núcleo le suma el float al enviar.
export const EditorImage = Image.extend({
  parseHTML: () => [{ tag: 'img[src^="data:image/"]' }],

  addAttributes() {
    return {
      ...this.parent?.(),
      align: {
        default: null,
        parseHTML: (el) => parseAlign(el.getAttribute("align")),
        renderHTML: (attrs) => (attrs.align ? { align: attrs.align } : {}),
      },
    };
  },

  addNodeView() {
    const make = this.parent?.();
    if (!make) return null;
    return (props) => {
      const view = make(props);
      const dom = view.dom as HTMLElement;
      syncAlign(dom, props.node);
      const update = view.update?.bind(view);
      view.update = (node, decorations, inner) => {
        const ok = update ? update(node, decorations, inner) : node.type === props.node.type;
        if (ok) syncAlign(dom, node);
        return ok;
      };
      return view;
    };
  },
}).configure({
  inline: false,
  allowBase64: true,
  resize: {
    enabled: true,
    directions: ["top-left", "top-right", "bottom-left", "bottom-right"],
    minWidth: 24,
    minHeight: 24,
    alwaysPreserveAspectRatio: true,
  },
});

// selectedImage devuelve la imagen seleccionada, o null.
export function selectedImage(editor: Editor): PMNode | null {
  const sel = editor.state.selection;
  return sel instanceof NodeSelection && sel.node.type.name === "image" ? sel.node : null;
}

// moveSelectedNode cambia de lugar el nodo seleccionado con el bloque de
// arriba (-1) o el de abajo (1), y lo deja seleccionado. Devuelve false si no
// hay nodo seleccionado o ya está en el borde.
export function moveSelectedNode(editor: Editor, dir: -1 | 1): boolean {
  const { state } = editor;
  const sel = state.selection;
  if (!(sel instanceof NodeSelection)) return false;
  const parent = sel.$from.parent;
  const index = sel.$from.index() + dir;
  const sibling = parent.maybeChild(index);
  if (!sibling) return false;
  // El renglón vacío del final lo vuelve a poner TipTap: pasarlo sumaría otro.
  if (index === parent.childCount - 1 && sibling.isTextblock && sibling.content.size === 0) return false;
  // Al borrar el nodo, el bloque de abajo pasa a empezar en sel.from.
  const target = sel.from + dir * sibling.nodeSize;
  const tr = state.tr.delete(sel.from, sel.to).insert(target, sel.node);
  tr.setSelection(NodeSelection.create(tr.doc, target));
  editor.view.dispatch(tr.scrollIntoView());
  editor.commands.focus();
  return true;
}
