import { useEffect, useMemo, useRef } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";
import { buildSrcDoc } from "@/lib/srcdoc";

// Intercepta clics en enlaces HTML del documento del correo y los abre afuera.
// Solo <a> HTML con href string (se ignoran los <a> de SVG, cuyo href es un objeto).
export function handleFrameClick(ev: Event): void {
  const a = (ev.target as Element | null)?.closest?.("a[href]");
  if (!(a instanceof HTMLAnchorElement) || typeof a.href !== "string") return;
  ev.preventDefault();
  svc.OpenExternal(a.href).catch((e) => toast.error(errorMessage(e)));
}

type Props = { html: string; text: string; allowRemote: boolean };

// El cuerpo del correo vive en un iframe aislado: sandbox sin scripts ni
// popups, CSP restrictiva en el documento y los enlaces salen al navegador.
export function SafeHtmlFrame({ html, text, allowRemote }: Props) {
  const ref = useRef<HTMLIFrameElement>(null);
  const srcDoc = useMemo(
    () => (html ? buildSrcDoc(html, { allowRemote }) : ""),
    [html, allowRemote],
  );
  const observer = useRef<ResizeObserver | null>(null);
  useEffect(() => () => observer.current?.disconnect(), []);

  if (!html) {
    return (
      <div className="m-6 whitespace-pre-wrap rounded-lg bg-white p-4 text-sm text-neutral-900 shadow-sm">
        {text}
      </div>
    );
  }

  const onLoad = () => {
    const frame = ref.current;
    const doc = frame?.contentDocument;
    if (!frame || !doc) return;
    const fit = () => {
      // documentElement incluye los márgenes del body (body.scrollHeight no).
      frame.style.height = `${doc.documentElement.scrollHeight}px`;
    };
    fit();
    observer.current?.disconnect();
    observer.current = new ResizeObserver(fit);
    observer.current.observe(doc.documentElement);
    doc.addEventListener("load", fit, true); // imágenes que cargan después
    doc.addEventListener("click", handleFrameClick);
  };

  return (
    <div className="m-6 overflow-hidden rounded-lg bg-white shadow-sm ring-1 ring-border">
      <iframe
        ref={ref}
        title="Contenido del correo"
        sandbox="allow-same-origin"
        srcDoc={srcDoc}
        onLoad={onLoad}
        className="block w-full border-0 bg-white"
      />
    </div>
  );
}
