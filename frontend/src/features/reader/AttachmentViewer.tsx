import { useEffect, useRef, useState, type PointerEvent } from "react";
import { ChevronLeftIcon, ChevronRightIcon, FileTextIcon, ImageIcon, PaperclipIcon, XIcon } from "lucide-react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Attachment, AttachmentData } from "../../../bindings/postacontrol/core/model/models";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { base64ToBytes, viewKind, viewMime } from "@/lib/attachments";
import { errorMessage } from "@/lib/errors";
import { cn } from "@/lib/utils";
import { PdfView } from "./PdfView";
import { saveAttachment } from "./saveAttachment";

// null es "Ajustar".
type Zoom = number | null;
const ZOOMS = [0.5, 0.75, 1, 1.25, 1.5, 2, 3];

function zoomIn(z: Zoom): Zoom {
  if (z === null) return 1;
  return ZOOMS.find((l) => l > z) ?? ZOOMS[ZOOMS.length - 1];
}

function zoomOut(z: Zoom): Zoom {
  if (z === null) return null;
  return [...ZOOMS].reverse().find((l) => l < z) ?? null;
}

function zoomLabel(z: Zoom): string {
  return z === null ? "Ajustar" : `${Math.round(z * 100)}%`;
}

// Resultado de leer un adjunto. `key` dice de qué lectura es: si no coincide
// con la actual, todavía se está cargando.
type Result =
  | { key: string; ok: true; url: string | null; bytes: Uint8Array | null }
  | { key: string; ok: false; message: string };

const EMPTY_MESSAGE = "El adjunto está vacío.";
const NOT_VIEWABLE_MESSAGE = "Este adjunto no se puede ver acá.";

const barButton =
  "rounded-md border border-zinc-600 px-2 py-0.5 text-xs transition-colors hover:bg-zinc-700 disabled:pointer-events-none disabled:opacity-40";

// AttachmentViewer muestra PDF e imágenes adjuntas sin descargarlas. `index`
// es el `Attachment.index` con el que abre; la tira de abajo y ←/→ pasan a
// los demás adjuntos visibles.
export function AttachmentViewer({
  messageId,
  attachments,
  index,
  onClose,
}: {
  messageId: number;
  attachments: Attachment[];
  index: number;
  onClose: () => void;
}) {
  const [current, setCurrent] = useState(index);
  const [zoom, setZoom] = useState<Zoom>(null);
  const [result, setResult] = useState<Result | null>(null);
  // Página visible del PDF, de la lectura `key` como en Result.
  const [page, setPage] = useState<{ key: string; n: number; total: number } | null>(null);

  const att = attachments.find((a) => a.index === current);
  const kind = att ? viewKind(att) : null;
  const mime = att ? viewMime(att) : null;
  const key = `${messageId}:${current}`;

  // Lee los bytes del adjunto. El blob URL de una imagen nace acá y se revoca
  // en la limpieza: al pasar a otro adjunto o al cerrar. La limpieza también
  // cancela la lectura si sigue en curso (la llamada de Wails es cancelable),
  // así no se siguen bajando hasta 50 MB que ya nadie va a ver.
  useEffect(() => {
    if (!kind || !mime) return;
    let live = true;
    let url: string | null = null;
    const call: Promise<AttachmentData> & { cancel?: () => unknown } = svc.AttachmentData(messageId, current);
    call
      .then((d) => {
        if (!live) return;
        const bytes = base64ToBytes(d.data ?? "");
        if (bytes.length === 0) {
          setResult({ key, ok: false, message: EMPTY_MESSAGE });
          return;
        }
        if (kind === "image") {
          url = URL.createObjectURL(new Blob([bytes as Uint8Array<ArrayBuffer>], { type: mime }));
          setResult({ key, ok: true, url, bytes: null });
        } else {
          setResult({ key, ok: true, url: null, bytes });
        }
      })
      .catch((e: unknown) => {
        if (live) setResult({ key, ok: false, message: errorMessage(e) });
      });
    return () => {
      live = false;
      void call.cancel?.();
      if (url) URL.revokeObjectURL(url);
    };
  }, [messageId, current, kind, mime, key]);

  const pos = attachments.findIndex((a) => a.index === current);
  const neighbor = (dir: 1 | -1): number | null => {
    for (let i = pos + dir; i >= 0 && i < attachments.length; i += dir) {
      if (viewKind(attachments[i])) return attachments[i].index;
    }
    return null;
  };
  const prev = neighbor(-1);
  const next = neighbor(1);

  // ←/→ mientras el visor está abierto. Esc lo maneja el Dialog.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
      const to = e.key === "ArrowLeft" ? prev : e.key === "ArrowRight" ? next : undefined;
      if (to === undefined) return;
      e.preventDefault();
      if (to === null) return;
      setCurrent(to);
      setZoom(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [prev, next]);

  const go = (i: number) => {
    setCurrent(i);
    setZoom(null);
  };

  const shown = !kind ? ({ key, ok: false, message: NOT_VIEWABLE_MESSAGE } as const) : result?.key === key ? result : null;
  const ready = shown?.ok === true;
  const name = att?.filename || "adjunto";

  let stage;
  if (!shown) stage = <p className="text-sm text-zinc-400">Cargando…</p>;
  else if (!shown.ok) stage = <p className="max-w-md px-6 text-center text-sm text-zinc-300">{shown.message}</p>;
  else if (shown.url) stage = <ImageView key={shown.url} url={shown.url} alt={name} zoom={zoom} />;
  else if (shown.bytes)
    stage = (
      <PdfView key={key} data={shown.bytes} zoom={zoom} onPageChange={(n, total) => setPage({ key, n, total })} />
    );
  const pageShown = ready && kind === "pdf" && page?.key === key ? page : null;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[calc(100vh-3rem)] w-[calc(100vw-3rem)] max-w-none flex-col gap-0 overflow-hidden bg-zinc-800 p-0 text-zinc-50 ring-zinc-700 sm:max-w-none"
      >
        <div className="flex items-center gap-2.5 border-b border-zinc-700 px-3 py-2">
          <DialogTitle className="min-w-0 flex-1 truncate text-sm font-semibold">{name}</DialogTitle>
          <DialogDescription className="sr-only">
            Visor de adjuntos. Usá las flechas izquierda y derecha para pasar de uno a otro.
          </DialogDescription>
          {pageShown && (
            <span className="shrink-0 text-xs text-zinc-400 tabular-nums">
              pág. {pageShown.n} de {pageShown.total}
            </span>
          )}
          <button
            type="button"
            aria-label="Alejar"
            className={barButton}
            disabled={!ready || zoom === null}
            onClick={() => setZoom(zoomOut)}
          >
            −
          </button>
          <span className="w-14 text-center text-xs tabular-nums">{zoomLabel(zoom)}</span>
          <button
            type="button"
            aria-label="Acercar"
            className={barButton}
            disabled={!ready || zoom === ZOOMS[ZOOMS.length - 1]}
            onClick={() => setZoom(zoomIn)}
          >
            +
          </button>
          <button
            type="button"
            className={cn(barButton, "border-zinc-50 bg-zinc-50 text-zinc-900 hover:bg-zinc-200")}
            onClick={() => void saveAttachment(messageId, current)}
          >
            Descargar
          </button>
          <DialogClose asChild>
            <button type="button" aria-label="Cerrar" className={cn(barButton, "px-1.5")}>
              <XIcon className="size-3.5" />
            </button>
          </DialogClose>
        </div>

        <div className="relative min-h-0 flex-1 bg-zinc-900">
          <div className="absolute inset-0 flex items-center justify-center">{stage}</div>
          {prev !== null && (
            <button
              type="button"
              aria-label="Anterior"
              onClick={() => go(prev)}
              className="absolute top-1/2 left-3 flex size-8 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 transition-colors hover:bg-white/20"
            >
              <ChevronLeftIcon className="size-4" />
            </button>
          )}
          {next !== null && (
            <button
              type="button"
              aria-label="Siguiente"
              onClick={() => go(next)}
              className="absolute top-1/2 right-3 flex size-8 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 transition-colors hover:bg-white/20"
            >
              <ChevronRightIcon className="size-4" />
            </button>
          )}
        </div>

        <div className="flex items-center gap-2 overflow-x-auto border-t border-zinc-700 px-3 py-2 text-xs">
          <span className="shrink-0 text-zinc-400">Adjuntos:</span>
          {attachments.map((a) => {
            const k = viewKind(a);
            const on = a.index === current;
            const Icon = k === "pdf" ? FileTextIcon : k === "image" ? ImageIcon : PaperclipIcon;
            return (
              <button
                key={a.index}
                type="button"
                data-viewable={k ? "true" : "false"}
                aria-current={on ? "true" : undefined}
                title={k ? undefined : "Descargar"}
                onClick={() => {
                  // Lo que no se previsualiza se descarga, sin cambiar lo que se ve.
                  if (!k) void saveAttachment(messageId, a.index);
                  else if (!on) go(a.index);
                }}
                className={cn(
                  "flex shrink-0 items-center gap-1.5 rounded-md border border-zinc-600 px-2 py-0.5 transition-opacity hover:opacity-100",
                  on ? "border-[#3598CC] bg-[#3598CC]/20 opacity-100" : k ? "opacity-70" : "opacity-45",
                )}
              >
                <Icon className="size-3.5" />
                <span className="max-w-48 truncate">{a.filename || "adjunto"}</span>
              </button>
            );
          })}
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ImageView muestra la imagen ajustada a la ventana o, con zoom, a
// naturalWidth × zoom dentro de un contenedor que se arrastra para moverla.
function ImageView({ url, alt, zoom }: { url: string; alt: string; zoom: Zoom }) {
  const [natural, setNatural] = useState(0);
  const drag = useRef<{ x: number; y: number; left: number; top: number } | null>(null);

  const img = (
    <img
      src={url}
      alt={alt}
      draggable={false}
      onLoad={(e) => setNatural(e.currentTarget.naturalWidth)}
      className={zoom === null ? "max-h-full max-w-full object-contain" : "max-w-none"}
      style={zoom !== null && natural > 0 ? { width: natural * zoom } : undefined}
    />
  );

  if (zoom === null) return <div className="flex size-full items-center justify-center p-4">{img}</div>;

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    const el = e.currentTarget;
    drag.current = { x: e.clientX, y: e.clientY, left: el.scrollLeft, top: el.scrollTop };
    el.setPointerCapture?.(e.pointerId);
  };
  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d) return;
    e.currentTarget.scrollLeft = d.left - (e.clientX - d.x);
    e.currentTarget.scrollTop = d.top - (e.clientY - d.y);
  };
  const stop = () => {
    drag.current = null;
  };

  return (
    <div
      className="size-full cursor-grab overflow-auto active:cursor-grabbing"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={stop}
      onPointerCancel={stop}
    >
      <div className="grid min-h-full w-max min-w-full place-items-center p-4">{img}</div>
    </div>
  );
}
