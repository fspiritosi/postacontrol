import { Table, TableCell, TableHeader, TableRow } from "@tiptap/extension-table";
import { NodeSelection } from "@tiptap/pm/state";
import { CellSelection } from "@tiptap/pm/tables";
import { EditorContent, useEditor, useEditorState, type Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import StarterKit from "@tiptap/starter-kit";
import {
  BoldIcon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  RemoveFormattingIcon,
  StrikethroughIcon,
  UnderlineIcon,
} from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { isAllowedLink, toLinkHref } from "@/lib/links";
import { createSlashCommand } from "./SlashMenu";

// StarterKit v3 ya incluye Link; se restringe a http, https y mailto.
const baseExtensions = [
  StarterKit.configure({
    link: {
      openOnClick: false,
      autolink: false,
      isAllowedUri: (url) => isAllowedLink(url),
    },
  }),
  Table.configure({ resizable: false }),
  TableRow,
  TableHeader,
  TableCell,
];

// LinkButton edita el enlace de la selección en un popover propio:
// window.prompt puede no funcionar en WebKitGTK.
// Es controlado porque lo abren también la barra de formato y el menú "/".
function LinkButton({ editor, open, onOpenChange: setOpen }: { editor: Editor; open: boolean; onOpenChange: (open: boolean) => void }) {
  const id = useId();
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  const active = editor.isActive("link");

  const onOpenChange = (next: boolean) => {
    if (next) {
      setUrl((editor.getAttributes("link").href as string | undefined) ?? "");
      setError("");
    }
    setOpen(next);
  };
  const remove = () => {
    editor.chain().focus().extendMarkRange("link").unsetLink().run();
    setOpen(false);
  };
  const apply = (e: FormEvent) => {
    e.preventDefault();
    e.stopPropagation(); // el popover está en un portal: no burbujear al redactor
    if (url.trim() === "") {
      remove();
      return;
    }
    const href = toLinkHref(url);
    if (href == null) {
      setError("Solo se permiten enlaces http, https o mailto.");
      return;
    }
    editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button type="button" variant={active ? "secondary" : "ghost"} size="icon-sm" aria-label="Enlace" aria-pressed={active}>
          <LinkIcon />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 gap-2 p-3">
        <form onSubmit={apply} className="flex flex-col gap-2">
          <Label htmlFor={id}>URL del enlace</Label>
          <Input
            id={id}
            autoFocus
            value={url}
            placeholder="https://"
            onChange={(e) => {
              setUrl(e.target.value);
              setError("");
            }}
            aria-invalid={error !== "" || undefined}
          />
          {error && (
            <p className="text-xs text-destructive" role="alert">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={remove} disabled={!active}>
              Quitar enlace
            </Button>
            <Button type="submit" size="sm">
              Aplicar
            </Button>
          </div>
        </form>
      </PopoverContent>
    </Popover>
  );
}

// Estilos parecidos a lo que sale en el correo enviado.
const EDITOR_CLASS = [
  "min-h-32 px-4 py-3 text-sm outline-none",
  "[&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-5 [&_ol]:pl-5 [&_a]:text-primary [&_a]:underline",
  "[&_h1]:text-2xl [&_h1]:font-bold [&_h2]:text-xl [&_h2]:font-bold [&_h3]:text-lg [&_h3]:font-bold",
  "[&_blockquote]:border-l-4 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground",
  "[&_table]:my-2 [&_table]:w-full [&_table]:border-collapse [&_td]:border [&_th]:border [&_td]:p-2 [&_th]:p-2 [&_td]:align-top",
  "[&_th]:bg-muted [&_th]:text-left [&_th]:font-semibold [&_.selectedCell]:bg-accent",
  "[&_hr]:my-3 [&_hr]:border-border [&_img]:h-auto [&_img]:max-w-full",
].join(" ");

type Props = {
  content: string;
  onChange: (html: string) => void;
  // Abre el selector de imágenes; sin esta prop el menú "/" no ofrece "Imagen".
  onPickImage?: () => void;
};

// La barra de formato sale con texto seleccionado, no con una imagen ni con celdas de tabla.
const showBubble = ({ state }: { state: Editor["state"] }) => {
  const sel = state.selection;
  return !sel.empty && !(sel instanceof NodeSelection) && !(sel instanceof CellSelection);
};

export function RichEditor({ content, onChange, onPickImage }: Props) {
  const [linkOpen, setLinkOpen] = useState(false);
  // El menú "/" se arma una sola vez: lee lo último de `latest`.
  const [latest] = useState(() => {
    let pickImage: (() => void) | undefined;
    return { getPickImage: () => pickImage, setPickImage: (fn?: () => void) => (pickImage = fn) };
  });
  useEffect(() => {
    latest.setPickImage(onPickImage);
  }, [latest, onPickImage]);
  const [extensions] = useState(() => [
    ...baseExtensions,
    createSlashCommand(
      () => ({ pickImage: () => latest.getPickImage()?.(), openLink: () => setLinkOpen(true) }),
      (item) => item.id !== "image" || latest.getPickImage() != null,
    ),
  ]);
  const bubbleRef = useRef<HTMLDivElement>(null);
  const editor = useEditor({
    extensions,
    content,
    onUpdate: ({ editor }) => onChange(editor.getHTML()),
    editorProps: {
      attributes: {
        "aria-label": "Cuerpo del mensaje",
        class: EDITOR_CLASS,
      },
    },
  });

  const inTable = useEditorState({ editor, selector: (ctx) => ctx.editor?.isActive("table") ?? false });
  if (!editor) return null;
  const tool = (label: string, icon: React.ReactNode, active: boolean, run: () => void) => (
    <Button
      type="button"
      variant={active ? "secondary" : "ghost"}
      size="icon-sm"
      aria-label={label}
      aria-pressed={active}
      onClick={run}
    >
      {icon}
    </Button>
  );
  return (
    <div className="rounded-md border">
      <div role="toolbar" aria-label="Herramientas del redactor" className="flex flex-wrap gap-1 border-b p-1">
        {tool("Negrita", <BoldIcon />, editor.isActive("bold"), () => editor.chain().focus().toggleBold().run())}
        {tool("Cursiva", <ItalicIcon />, editor.isActive("italic"), () => editor.chain().focus().toggleItalic().run())}
        {tool("Lista con viñetas", <ListIcon />, editor.isActive("bulletList"), () => editor.chain().focus().toggleBulletList().run())}
        {tool("Lista numerada", <ListOrderedIcon />, editor.isActive("orderedList"), () => editor.chain().focus().toggleOrderedList().run())}
        <LinkButton editor={editor} open={linkOpen} onOpenChange={setLinkOpen} />
        {inTable && (
          <>
            <Button type="button" variant="ghost" size="sm" onClick={() => editor.chain().focus().addRowAfter().run()}>
              Agregar fila
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => editor.chain().focus().addColumnAfter().run()}>
              Agregar columna
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => editor.chain().focus().deleteRow().run()}>
              Eliminar fila
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => editor.chain().focus().deleteColumn().run()}>
              Eliminar columna
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => editor.chain().focus().deleteTable().run()}>
              Eliminar tabla
            </Button>
          </>
        )}
      </div>
      <BubbleMenu
        ref={bubbleRef}
        editor={editor}
        options={{ placement: "top" }}
        shouldShow={({ state, view }) =>
          showBubble({ state }) && (view.hasFocus() || (bubbleRef.current?.contains(document.activeElement) ?? false))
        }
      >
        <div role="toolbar" aria-label="Formato del texto" className="flex gap-1 rounded-md bg-popover p-1 shadow-md ring-1 ring-foreground/10">
          {tool("Negrita", <BoldIcon />, editor.isActive("bold"), () => editor.chain().focus().toggleBold().run())}
          {tool("Cursiva", <ItalicIcon />, editor.isActive("italic"), () => editor.chain().focus().toggleItalic().run())}
          {tool("Subrayado", <UnderlineIcon />, editor.isActive("underline"), () => editor.chain().focus().toggleUnderline().run())}
          {tool("Tachado", <StrikethroughIcon />, editor.isActive("strike"), () => editor.chain().focus().toggleStrike().run())}
          {tool("Enlace", <LinkIcon />, editor.isActive("link"), () => setLinkOpen(true))}
          {tool("Quitar formato", <RemoveFormattingIcon />, false, () => editor.chain().focus().unsetAllMarks().clearNodes().run())}
        </div>
      </BubbleMenu>
      <EditorContent editor={editor} />
    </div>
  );
}
