import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { BoldIcon, ItalicIcon, LinkIcon, ListIcon, ListOrderedIcon } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { isAllowedLink, toLinkHref } from "@/lib/links";

// StarterKit v3 ya incluye Link; se restringe a http, https y mailto.
const extensions = [
  StarterKit.configure({
    link: {
      openOnClick: false,
      autolink: false,
      isAllowedUri: (url) => isAllowedLink(url),
    },
  }),
];

// LinkButton edita el enlace de la selección en un popover propio:
// window.prompt puede no funcionar en WebKitGTK.
function LinkButton({ editor }: { editor: Editor }) {
  const id = useId();
  const [open, setOpen] = useState(false);
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

type Props = {
  content: string;
  onChange: (html: string) => void;
};

export function RichEditor({ content, onChange }: Props) {
  const editor = useEditor({
    extensions,
    content,
    onUpdate: ({ editor }) => onChange(editor.getHTML()),
    editorProps: {
      attributes: {
        "aria-label": "Cuerpo del mensaje",
        class: "min-h-32 px-4 py-3 text-sm outline-none [&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-5 [&_ol]:pl-5 [&_a]:text-primary [&_a]:underline",
      },
    },
  });

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
      <div className="flex gap-1 border-b p-1">
        {tool("Negrita", <BoldIcon />, editor.isActive("bold"), () => editor.chain().focus().toggleBold().run())}
        {tool("Cursiva", <ItalicIcon />, editor.isActive("italic"), () => editor.chain().focus().toggleItalic().run())}
        {tool("Lista con viñetas", <ListIcon />, editor.isActive("bulletList"), () => editor.chain().focus().toggleBulletList().run())}
        {tool("Lista numerada", <ListOrderedIcon />, editor.isActive("orderedList"), () => editor.chain().focus().toggleOrderedList().run())}
        <LinkButton editor={editor} />
      </div>
      <EditorContent editor={editor} />
    </div>
  );
}
