import type { Editor } from "@tiptap/react";
import type { Template } from "../../../bindings/postacontrol/core/model/models";

// Lo que el menú "/" necesita de afuera: abrir el selector de imagen y el
// popover de enlace, las plantillas y qué más hacer al elegir una (el asunto).
export type SlashContext = {
  pickImage: (editor: Editor) => void;
  openLink: () => void;
  templates: () => Template[];
  applyTemplate: (t: Template) => void;
};

export type SlashItem = {
  id: string;
  title: string;
  keywords: string[];
  run: (editor: Editor, ctx: SlashContext) => void;
};

const heading = (level: 1 | 2 | 3) => (editor: Editor) => editor.chain().focus().setNode("heading", { level }).run();

export const SLASH_ITEMS: SlashItem[] = [
  { id: "h1", title: "Título 1", keywords: ["titulo", "encabezado", "h1"], run: heading(1) },
  { id: "h2", title: "Título 2", keywords: ["titulo", "encabezado", "h2"], run: heading(2) },
  { id: "h3", title: "Título 3", keywords: ["titulo", "encabezado", "h3"], run: heading(3) },
  {
    id: "bulletList",
    title: "Lista con viñetas",
    keywords: ["vinetas", "lista", "puntos"],
    run: (editor) => editor.chain().focus().toggleBulletList().run(),
  },
  {
    id: "orderedList",
    title: "Lista numerada",
    keywords: ["numerada", "lista", "numeros"],
    run: (editor) => editor.chain().focus().toggleOrderedList().run(),
  },
  { id: "quote", title: "Cita", keywords: ["cita", "quote"], run: (editor) => editor.chain().focus().toggleBlockquote().run() },
  {
    id: "table",
    title: "Tabla",
    keywords: ["tabla", "grilla"],
    run: (editor) => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
  },
  { id: "image", title: "Imagen", keywords: ["imagen", "foto"], run: (editor, ctx) => ctx.pickImage(editor) },
  {
    id: "rule",
    title: "Separador",
    keywords: ["separador", "linea"],
    run: (editor) => editor.chain().focus().setHorizontalRule().run(),
  },
  { id: "link", title: "Enlace", keywords: ["enlace", "link"], run: (_editor, ctx) => ctx.openLink() },
];

// Minúsculas y sin acentos, para que "VIÑETAS" encuentre "vinetas".
const normalize = (s: string) =>
  s
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase();

// Un ítem por plantilla: inserta su cuerpo donde está el cursor.
export function templateItems(templates: Template[]): SlashItem[] {
  return templates.map((t) => ({
    id: `template-${t.id}`,
    title: `Plantilla: ${t.name}`,
    keywords: ["plantilla", "respuesta", t.name],
    run: (editor, ctx) => {
      editor.chain().focus().insertContent(t.html).run();
      ctx.applyTemplate(t);
    },
  }));
}

export function filterSlashItems(query: string, extra: SlashItem[] = []): SlashItem[] {
  const all = [...SLASH_ITEMS, ...extra];
  const q = normalize(query.trim());
  if (q === "") return all;
  return all.filter((item) => [item.title, ...item.keywords].some((t) => normalize(t).includes(q)));
}
