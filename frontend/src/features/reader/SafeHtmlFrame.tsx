import { useEffect, useMemo, useRef } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";
import { buildSrcDoc } from "@/lib/srcdoc";

// Intercepta clics en enlaces HTML del documento del correo y los abre afuera.
// Solo <a> HTML con href string (los <a> de SVG tienen un SVGAnimatedString).
// Sin instanceof: el nodo vive en otro realm (el del iframe).
export function handleFrameClick(ev: Event): void {
  const a = (ev.target as Element | null)?.closest?.("a");
  if (!a || a.namespaceURI !== "http://www.w3.org/1999/xhtml" || a.localName !== "a") return;
  const href = (a as unknown as { href?: unknown }).href;
  if (typeof href !== "string" || href === "") return;
  ev.preventDefault();
  svc.OpenExternal(href).catch((e) => toast.error(errorMessage(e)));
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
  // Corta el observer al cambiar el HTML, pasar a texto plano o desmontar.
  useEffect(() => {
    return () => {
      observer.current?.disconnect();
      observer.current = null;
    };
  }, [html]);

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
      frame.style.height = "0px"; // permite que también se achique
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
