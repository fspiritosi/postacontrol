import FileHandler from "@tiptap/extension-file-handler";
import { Table, TableCell, TableHeader, TableRow } from "@tiptap/extension-table";
import { FontSize, TextStyle } from "@tiptap/extension-text-style";
import { NodeSelection } from "@tiptap/pm/state";
import { CellSelection } from "@tiptap/pm/tables";
import { EditorContent, useEditor, useEditorState, type Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import StarterKit from "@tiptap/starter-kit";
import {
  ArrowDownIcon,
  ArrowUpIcon,
  BoldIcon,
  ImageIcon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  PanelLeftIcon,
  PanelRightIcon,
  RectangleHorizontalIcon,
  RemoveFormattingIcon,
  StrikethroughIcon,
  UnderlineIcon,
} from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { isAllowedLink, toLinkHref } from "@/lib/links";
import { EditorImage, moveSelectedNode, selectedImage, type ImageAlign } from "./editorImage";
import { IMAGE_TYPES, imageError, readAsDataURL } from "./editorImages";
import { createSlashCommand } from "./SlashMenu";
import type { Template } from "../../../bindings/postacontrol/core/model/models";

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
  TextStyle,
  FontSize,
  EditorImage,
];

// Tamaños del texto. "Normal" quita el tamaño: queda el del correo (14px).
const NORMAL_SIZE = "normal";
const FONT_SIZES = [
  { label: "Chico", value: "12px" },
  { label: "Normal", value: NORMAL_SIZE },
  { label: "Grande", value: "18px" },
  { label: "Muy grande", value: "24px" },
];

// LinkButton edita el enlace de la selección en un popover propio:
// window.prompt puede no funcionar en WebKitGTK.
// Es controlado porque lo abren también la barra de formato y el menú "/".
function LinkButton({ editor, open, onOpenChange: setOpen }: { editor: Editor; open: boolean; onOpenChange: (open: boolean) => void }) {
  const id = useId();
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  const active = editor.isActive("link");

  // Al abrir, sea quien sea que lo abra (botón, barra de formato o "/"), se carga el enlace actual.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setUrl((editor.getAttributes("link").href as string | undefined) ?? "");
      setError("");
    }
  }
  const onOpenChange = setOpen;
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
    if (editor.state.selection.empty && !active) {
      // Sin texto seleccionado (p. ej. desde "/"), la URL misma queda como texto del enlace.
      editor.chain().focus().insertContent({ type: "text", text: href, marks: [{ type: "link", attrs: { href } }] }).run();
    } else {
      editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
    }
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
  // Avisa que el contenido cambió y entrega cómo leerlo: con capturas en el
  // cuerpo serializar el HTML pesa decenas de MB, así que lo hace quien lo necesita.
  onChange: (getHTML: () => string) => void;
  // Abre el selector de imágenes; sin esta prop el menú "/" no ofrece "Imagen".
  onPickImage?: (editor: Editor) => void;
  // Errores al pegar una imagen (tipo o tamaño no soportado).
  onError?: (msg: string) => void;
  // Avisa cuando el menú "/" abre (con cómo cerrarlo) o cierra (null): su Esc no debe cerrar el diálogo.
  onSlashMenuChange?: (close: (() => void) | null) => void;
  // Pone el foco al principio del cuerpo al crearse.
  autoFocus?: boolean;
  // Plantillas que ofrece el menú "/"; onApplyTemplate avisa cuál se insertó
  // (para completar el asunto).
  templates?: Template[];
  onApplyTemplate?: (t: Template) => void;
};

// La barra de formato sale con texto seleccionado, no con una imagen ni con celdas de tabla.
const showBubble = ({ state }: { state: Editor["state"] }) => {
  const sel = state.selection;
  return !sel.empty && !(sel instanceof NodeSelection) && !(sel instanceof CellSelection);
};

// La barra de la imagen sale con una imagen seleccionada.
const showImageBubble = ({ state }: { state: Editor["state"] }) => {
  const sel = state.selection;
  return sel instanceof NodeSelection && sel.node.type.name === "image";
};

export function RichEditor({
  content,
  onChange,
  onPickImage,
  onError,
  onSlashMenuChange,
  autoFocus = false,
  templates,
  onApplyTemplate,
}: Props) {
  const [linkOpen, setLinkOpen] = useState(false);
  // El menú "/" se arma una sola vez: lee lo último de `latest`.
  type Latest = {
    pickImage?: (editor: Editor) => void;
    error?: (msg: string) => void;
    slashMenuChange?: (close: (() => void) | null) => void;
    templates?: Template[];
    applyTemplate?: (t: Template) => void;
  };
  const [latest] = useState(() => {
    let cur: Latest = {};
    return {
      getPickImage: () => cur.pickImage,
      getError: () => cur.error,
      getSlashMenuChange: () => cur.slashMenuChange,
      getTemplates: () => cur.templates ?? [],
      getApplyTemplate: () => cur.applyTemplate,
      set: (next: Latest) => {
        cur = next;
      },
    };
  });
  useEffect(() => {
    latest.set({
      pickImage: onPickImage,
      slashMenuChange: onSlashMenuChange,
      error: onError,
      templates,
      applyTemplate: onApplyTemplate,
    });
  }, [latest, onPickImage, onSlashMenuChange, onError, templates, onApplyTemplate]);
  const [extensions] = useState(() => [
    ...baseExtensions,
    FileHandler.configure({
      allowedMimeTypes: IMAGE_TYPES,
      onPaste: (editor, files, html) => {
        // Word, LibreOffice y las planillas ponen un PNG de la selección junto al HTML: ese HTML ya se pega solo.
        if (html && !/<img/i.test(html)) return;
        for (const file of files) {
          const msg = imageError(file);
          if (msg) {
            latest.getError()?.(msg);
            continue;
          }
          void readAsDataURL(file)
            .then((src) => editor.chain().focus().setImage({ src, alt: file.name }).run())
            .catch(() => latest.getError()?.("No se pudo leer la imagen."));
        }
      },
    }),
    createSlashCommand(
      () => ({
        pickImage: (editor) => latest.getPickImage()?.(editor),
        openLink: () => setLinkOpen(true),
        templates: latest.getTemplates,
        applyTemplate: (t) => latest.getApplyTemplate()?.(t),
      }),
      (item) => item.id !== "image" || latest.getPickImage() != null,
      (close) => latest.getSlashMenuChange()?.(close),
    ),
  ]);
  const bubbleRef = useRef<HTMLDivElement>(null);
  const editor = useEditor({
    extensions,
    content,
    autofocus: autoFocus ? "start" : false,
    onUpdate: ({ editor }) => onChange(() => editor.getHTML()),
    editorProps: {
      attributes: {
        "aria-label": "Cuerpo del mensaje",
        class: EDITOR_CLASS,
      },
    },
  });

  const inTable = useEditorState({ editor, selector: (ctx) => ctx.editor?.isActive("table") ?? false });
  const fontSize = useEditorState({
    editor,
    selector: (ctx) => (ctx.editor?.getAttributes("textStyle").fontSize as string | undefined) ?? NORMAL_SIZE,
  });
  const imageAlign = useEditorState({
    editor,
    selector: (ctx) => (ctx.editor ? ((selectedImage(ctx.editor)?.attrs.align as ImageAlign | undefined) ?? null) : null),
  });
  const imageBubbleRef = useRef<HTMLDivElement>(null);
  if (!editor) return null;
  const setFontSize = (v: string) => {
    if (v === NORMAL_SIZE) editor.chain().focus().unsetFontSize().run();
    else editor.chain().focus().setFontSize(v).run();
  };
  const setImageAlign = (align: ImageAlign) => editor.chain().focus().updateAttributes("image", { align }).run();
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
        <Select value={FONT_SIZES.some((f) => f.value === fontSize) ? fontSize : NORMAL_SIZE} onValueChange={setFontSize}>
          <SelectTrigger size="sm" className="w-32" aria-label="Tamaño del texto">
            <SelectValue />
          </SelectTrigger>
          {/* Al cerrar, el foco vuelve al texto y no al selector. */}
          <SelectContent onCloseAutoFocus={(e) => e.preventDefault()}>
            {FONT_SIZES.map((f) => (
              <SelectItem key={f.value} value={f.value}>
                {f.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {tool("Negrita", <BoldIcon />, editor.isActive("bold"), () => editor.chain().focus().toggleBold().run())}
        {tool("Cursiva", <ItalicIcon />, editor.isActive("italic"), () => editor.chain().focus().toggleItalic().run())}
        {tool("Lista con viñetas", <ListIcon />, editor.isActive("bulletList"), () => editor.chain().focus().toggleBulletList().run())}
        {tool("Lista numerada", <ListOrderedIcon />, editor.isActive("orderedList"), () => editor.chain().focus().toggleOrderedList().run())}
        <LinkButton editor={editor} open={linkOpen} onOpenChange={setLinkOpen} />
        {onPickImage && tool("Imagen", <ImageIcon />, false, () => onPickImage(editor))}
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
      <BubbleMenu
        ref={imageBubbleRef}
        pluginKey="imageBubbleMenu"
        editor={editor}
        options={{ placement: "top" }}
        shouldShow={({ state, view }) =>
          showImageBubble({ state }) && (view.hasFocus() || (imageBubbleRef.current?.contains(document.activeElement) ?? false))
        }
      >
        <div role="toolbar" aria-label="Ubicación de la imagen" className="flex gap-1 rounded-md bg-popover p-1 shadow-md ring-1 ring-foreground/10">
          {tool("Imagen sola en su renglón", <RectangleHorizontalIcon />, imageAlign === null, () => setImageAlign(null))}
          {tool("Imagen a la izquierda del texto", <PanelLeftIcon />, imageAlign === "left", () => setImageAlign("left"))}
          {tool("Imagen a la derecha del texto", <PanelRightIcon />, imageAlign === "right", () => setImageAlign("right"))}
          {tool("Subir imagen", <ArrowUpIcon />, false, () => moveSelectedNode(editor, -1))}
          {tool("Bajar imagen", <ArrowDownIcon />, false, () => moveSelectedNode(editor, 1))}
        </div>
      </BubbleMenu>
      <EditorContent editor={editor} />
    </div>
  );
}
