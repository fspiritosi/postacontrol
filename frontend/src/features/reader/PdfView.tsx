import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { PdfDoc } from "@/lib/pdf";

const ERROR_MESSAGE = "No pudimos abrir el PDF. Podés descargarlo y abrirlo con otro programa.";
const PAGE_ERROR_MESSAGE = "No pudimos mostrar esta página.";
// Igual a RENDER_CANCELLED de @/lib/pdf, que se carga aparte.
const RENDER_CANCELLED = "RenderCancelled";

// Margen a cada lado de las páginas (p-4). "Ajustar" usa el ancho que queda.
const GUTTER = 16;
// Espera tras un cambio de tamaño antes de volver a dibujar.
const RESIZE_DELAY = 150;
// Se dibujan las páginas a menos de un alto de pantalla de lo visible; las
// demás quedan como un recuadro del tamaño de la página, sin canvas.
const NEAR_MARGIN = "100% 0px";
// Tamaño de una página que no se pudo medir (carta, en puntos).
const FALLBACK_SIZE = { width: 612, height: 792 };

type Size = { width: number; height: number };
type State = { doc: PdfDoc; sizes: Size[] } | { failed: true } | null;
// Punto de lectura: página (índice), fracción de su alto que quedó arriba de la
// vista y centro horizontal de la vista como fracción del ancho del contenido.
type Anchor = { page: number; y: number; x: number };

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
  const [state, setState] = useState<State>(null);
  const [width, setWidth] = useState<number | null>(null);
  // Páginas cerca de lo visible. Sin IntersectionObserver se dibujan todas.
  const [near, setNear] = useState<ReadonlySet<number>>(() => new Set());
  const scroller = useRef<HTMLDivElement>(null);
  const pages = useRef<(HTMLDivElement | null)[]>([]);
  // Se anota al desplazarse; vale para cualquier zoom porque es relativo.
  const anchor = useRef<Anchor | null>(null);
  const notify = useRef(onPageChange);
  useLayoutEffect(() => {
    notify.current = onPageChange;
  });

  // Abre el documento, mide sus páginas y lo destruye al desmontar o al
  // cambiar `data`, también si llega cuando ya no se lo espera. pdf.js se
  // carga recién con el primer PDF, aparte del bundle principal.
  useEffect(() => {
    let live = true;
    let doc: PdfDoc | null = null;
    const open = async () => {
      const d = await (await import("@/lib/pdf")).loadPdf(data);
      if (!live) {
        d.destroy();
        return;
      }
      doc = d;
      const measured = await Promise.all(
        Array.from({ length: d.numPages }, (_, i) => d.pageSize(i + 1).catch(() => null)),
      );
      if (!live) return;
      const known = measured.find((s) => s !== null) ?? FALLBACK_SIZE;
      setState({ doc: d, sizes: measured.map((s) => s ?? known) });
    };
    open().catch(() => {
      if (live) setState({ failed: true });
    });
    return () => {
      live = false;
      doc?.destroy();
      setState(null);
      anchor.current = null;
    };
  }, [data]);

  const doc = state && "doc" in state ? state.doc : null;

  // Ancho disponible para "Ajustar": se mide enseguida y, al cambiar el
  // tamaño, después de RESIZE_DELAY para no redibujar en cada paso.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const measure = () => setWidth(Math.max(0, el.clientWidth - 2 * GUTTER));
    measure();
    if (typeof ResizeObserver === "undefined") return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const ro = new ResizeObserver(() => {
      clearTimeout(timer);
      timer = setTimeout(measure, RESIZE_DELAY);
    });
    ro.observe(el);
    return () => {
      clearTimeout(timer);
      ro.disconnect();
    };
  }, [doc]);

  // Página visible (la que más se ve) y páginas cercanas a lo visible.
  useEffect(() => {
    if (!doc) return;
    const total = doc.numPages;
    let current = 1;
    notify.current?.(current, total);
    const root = scroller.current;
    if (!root || typeof IntersectionObserver === "undefined") return;
    const els = pages.current.slice(0, total).filter((el) => el !== null);
    const pageOf = (e: IntersectionObserverEntry) => Number((e.target as HTMLElement).dataset.page);

    const ratios = new Map<number, number>();
    const visible = new IntersectionObserver(
      (entries) => {
        for (const e of entries) ratios.set(pageOf(e), e.intersectionRatio);
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

    const close = new Set<number>();
    const nearby = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) close.add(pageOf(e));
          else close.delete(pageOf(e));
        }
        setNear(new Set(close));
      },
      { root, rootMargin: NEAR_MARGIN },
    );

    for (const el of els) {
      visible.observe(el);
      nearby.observe(el);
    }
    return () => {
      visible.disconnect();
      nearby.disconnect();
    };
  }, [doc]);

  // Al cambiar el zoom (o el ancho, con "Ajustar") cambian los altos de las
  // páginas pero no scrollTop, que está en px: se vuelve al punto de lectura
  // anotado antes de que se pinte.
  const layoutKey = zoom === null ? `fit:${width}` : `zoom:${zoom}`;
  useLayoutEffect(() => {
    const el = scroller.current;
    const a = anchor.current;
    const p = a && pages.current[a.page];
    if (!el || !a || !p) return;
    el.scrollTop = p.offsetTop + a.y * p.offsetHeight;
    el.scrollLeft = Math.max(0, a.x * el.scrollWidth - el.clientWidth / 2);
  }, [layoutKey]);

  const onScroll = () => {
    const el = scroller.current;
    const n = state && "sizes" in state ? state.sizes.length : 0;
    if (!el || n === 0) return;
    // Primera página que no quedó entera arriba de la vista (búsqueda binaria:
    // las páginas van en orden).
    const top = el.scrollTop;
    let lo = 0;
    let hi = n - 1;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      const p = pages.current[mid];
      if (p && top >= p.offsetTop + p.offsetHeight) lo = mid + 1;
      else hi = mid;
    }
    const p = pages.current[lo];
    if (!p) return;
    const y = p.offsetHeight > 0 ? Math.min(1, Math.max(0, (top - p.offsetTop) / p.offsetHeight)) : 0;
    const x = el.scrollWidth > 0 ? (el.scrollLeft + el.clientWidth / 2) / el.scrollWidth : 0.5;
    anchor.current = { page: lo, y, x };
  };

  if (state && "failed" in state) {
    return (
      <div className="flex size-full items-center justify-center">
        <p className="max-w-md px-6 text-center text-sm text-zinc-300">{ERROR_MESSAGE}</p>
      </div>
    );
  }

  const everyPage = typeof IntersectionObserver === "undefined";

  return (
    <div ref={scroller} className="relative size-full overflow-auto" onScroll={onScroll}>
      {!state ? (
        <div className="flex size-full items-center justify-center">
          <p className="text-sm text-zinc-400">Cargando…</p>
        </div>
      ) : (
        <div className="flex w-max min-w-full flex-col items-center gap-4 p-4">
          {state.sizes.map((size, i) => {
            // Sin zoom ni ancho medido todavía no hay escala.
            const scale = zoom ?? (width === null ? null : width > 0 ? width / size.width : 1);
            return (
              <div
                key={i}
                data-page={i + 1}
                ref={(el) => {
                  pages.current[i] = el;
                }}
              >
                <PdfPage
                  doc={state.doc}
                  n={i + 1}
                  size={size}
                  scale={scale}
                  near={everyPage || near.has(i + 1)}
                />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// PdfPage es un recuadro del tamaño de la página. Cerca de lo visible dibuja
// la página, y la vuelve a dibujar si cambia la escala; lejos, libera la
// canvas.
function PdfPage({
  doc,
  n,
  size,
  scale,
  near,
}: {
  doc: PdfDoc;
  n: number;
  size: Size;
  scale: number | null;
  near: boolean;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  // Escala de lo que hay dibujado; null si no hay nada o no se sabe.
  const drawn = useRef<number | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const c = canvas.current;
    if (!c) return;
    if (!near) {
      doc.releaseCanvas(c);
      drawn.current = null;
      return;
    }
    if (scale === null || scale === drawn.current) return;
    let live = true;
    drawn.current = null;
    doc.renderPage(n, c, scale).then(
      () => {
        if (!live) return;
        drawn.current = scale;
        setFailed(false);
      },
      (e: unknown) => {
        // Cancelada por otro dibujo o al desmontar: no es un error.
        if (live && !(e instanceof Error && e.name === RENDER_CANCELLED)) setFailed(true);
      },
    );
    return () => {
      live = false;
    };
  }, [doc, n, scale, near]);

  const s = scale ?? 1;
  return (
    <div
      className="relative overflow-hidden bg-white shadow-md"
      style={{ width: Math.floor(size.width * s), height: Math.floor(size.height * s) }}
    >
      <canvas ref={canvas} className="block" />
      {failed && (
        <p className="absolute inset-0 flex items-center justify-center px-4 text-center text-xs text-zinc-500">
          {PAGE_ERROR_MESSAGE}
        </p>
      )}
    </div>
  );
}
