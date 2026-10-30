import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { BoldIcon, ItalicIcon, LinkIcon, ListIcon, ListOrderedIcon } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";

const ALLOWED_URI = /^(https?:|mailto:)/i;

// StarterKit v3 ya incluye Link; se restringe a http, https y mailto.
const extensions = [
  StarterKit.configure({
    link: {
      openOnClick: false,
      autolink: false,
      isAllowedUri: (url) => ALLOWED_URI.test(url),
    },
  }),
];

function promptLink(editor: Editor) {
  const current = editor.getAttributes("link").href as string | undefined;
  const input = window.prompt("URL del enlace", current ?? "https://");
  if (input === null) return;
  const url = input.trim();
  if (url === "") {
    editor.chain().focus().extendMarkRange("link").unsetLink().run();
    return;
  }
  const href = /^[a-z][a-z0-9+.-]*:/i.test(url) ? url : `https://${url}`;
  if (!ALLOWED_URI.test(href)) {
    toast.error("Solo se permiten enlaces http, https o mailto.");
    return;
  }
  editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
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
        {tool("Enlace", <LinkIcon />, editor.isActive("link"), () => promptLink(editor))}
      </div>
      <EditorContent editor={editor} />
    </div>
  );
}
