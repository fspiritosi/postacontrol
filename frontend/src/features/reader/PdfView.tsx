import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { PdfDoc } from "@/lib/pdf";

const ERROR_MESSAGE = "No pudimos abrir el PDF. Podés descargarlo y abrirlo con otro programa.";

// Margen a cada lado de las páginas (p-4). "Ajustar" usa el ancho que queda.
const GUTTER = 16;

type State = { doc: PdfDoc | null; failed: boolean };

// PdfView muestra las páginas del PDF una debajo de otra. `zoom` null es
// "Ajustar": cada página se escala al ancho del contenedor. `onPageChange`
// avisa la página que más se ve y el total.
export function PdfView({
  data,
  zoom,
  onPageChange,
}: {
  data: Uint8Array;
  zoom: number | null;
  onPageChange?: (page: number, total: number) => void;
}) {
  const [state, setState] = useState<State>({ doc: null, failed: false });
  const [width, setWidth] = useState<number | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const pages = useRef<(HTMLDivElement | null)[]>([]);
  const notify = useRef(onPageChange);
  useLayoutEffect(() => {
    notify.current = onPageChange;
  });

  // Abre el documento y lo destruye al desmontar o al cambiar `data`, también
  // si llega cuando ya no se lo espera. pdf.js se carga recién con el primer
  // PDF, aparte del bundle principal.
  useEffect(() => {
    let live = true;
    let doc: PdfDoc | null = null;
    import("@/lib/pdf")
      .then((m) => m.loadPdf(data))
      .then(
        (d) => {
          if (!live) {
            d.destroy();
            return;
          }
          doc = d;
          setState({ doc: d, failed: false });
        },
        () => {
          if (live) setState({ doc: null, failed: true });
        },
      );
    return () => {
      live = false;
      doc?.destroy();
      setState({ doc: null, failed: false });
    };
  }, [data]);

  // Ancho disponible para "Ajustar".
  const doc = state.doc;
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const measure = () => setWidth(Math.max(0, el.clientWidth - 2 * GUTTER));
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [doc]);

  // Página visible: la que más se ve dentro del contenedor.
  useEffect(() => {
    if (!doc) return;
    const total = doc.numPages;
    let current = 1;
    notify.current?.(current, total);
    const root = scroller.current;
    if (!root || typeof IntersectionObserver === "undefined") return;
    const ratios = new Map<number, number>();
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) ratios.set(Number((e.target as HTMLElement).dataset.page), e.intersectionRatio);
        let best = current;
        let bestRatio = -1;
        for (const [n, r] of ratios) {
          if (r > bestRatio || (r === bestRatio && n < best)) {
            best = n;
            bestRatio = r;
          }
        }
        if (bestRatio > 0 && best !== current) {
          current = best;
          notify.current?.(current, total);
        }
      },
      { root, threshold: [0, 0.25, 0.5, 0.75, 1] },
    );
    for (const el of pages.current.slice(0, total)) if (el) io.observe(el);
    return () => io.disconnect();
  }, [doc]);

  if (state.failed) {
    return (
      <div className="flex size-full items-center justify-center">
        <p className="max-w-md px-6 text-center text-sm text-zinc-300">{ERROR_MESSAGE}</p>
      </div>
    );
  }

  return (
    <div ref={scroller} className="size-full overflow-auto">
      {!doc ? (
        <div className="flex size-full items-center justify-center">
          <p className="text-sm text-zinc-400">Cargando…</p>
        </div>
      ) : (
        <div className="flex w-max min-w-full flex-col items-center gap-4 p-4">
          {Array.from({ length: doc.numPages }, (_, i) => (
            <div
              key={i}
              data-page={i + 1}
              ref={(el) => {
                pages.current[i] = el;
              }}
            >
              <PdfPage doc={doc} n={i + 1} zoom={zoom} width={width} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// PdfPage dibuja una página al montarse y otra vez cuando cambia el zoom o,
// con "Ajustar", el ancho disponible (null mientras no se midió).
function PdfPage({ doc, n, zoom, width }: { doc: PdfDoc; n: number; zoom: number | null; width: number | null }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  // Ancho de la página a escala 1, que se conoce después del primer dibujo.
  const natural = useRef<number | null>(null);
  // Escala de lo que hay dibujado; null si no se sabe.
  const drawn = useRef<number | null>(null);

  useEffect(() => {
    const c = canvas.current;
    if (!c || (zoom === null && width === null)) return;
    let live = true;
    let busy = false;
    const render = async (scale: number) => {
      busy = true;
      const size = await doc.renderPage(n, c, scale);
      busy = false;
      natural.current = size.width / scale;
      drawn.current = scale;
    };
    const draw = async () => {
      let scale = zoom;
      if (scale === null) {
        if (natural.current === null) {
          await render(1);
          if (!live) return;
        }
        const w = natural.current ?? 0;
        scale = width && w > 0 ? width / w : 1;
      }
      if (scale !== drawn.current) await render(scale);
    };
    // Una página cancelada (otro zoom, desmontaje) o rota queda en blanco.
    draw().catch(() => {});
    return () => {
      live = false;
      // Lo que estaba dibujando puede terminar con otra escala.
      if (busy) drawn.current = null;
    };
  }, [doc, n, zoom, width]);

  return <canvas ref={canvas} className="block bg-white shadow-md" />;
}
