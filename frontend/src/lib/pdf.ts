// Único archivo que importa pdfjs-dist. El PDF es un adjunto que no
// controlamos: se abre sin XFA y se dibuja solo en canvas, sin capa
// de texto ni de anotaciones (sus enlaces no se pueden cliquear). Nada sale a
// la red: el worker y los decodificadores wasm van en el bundle; las fuentes
// estándar caen en las del sistema y los CMaps predefinidos no se cargan.
import { getDocument, GlobalWorkerOptions, type RenderTask } from "pdfjs-dist";
import PdfWorker from "pdfjs-dist/build/pdf.worker.min.mjs?worker";
import jbig2Wasm from "pdfjs-dist/wasm/jbig2.wasm?url";
import openjpegWasm from "pdfjs-dist/wasm/openjpeg.wasm?url";
import qcmsWasm from "pdfjs-dist/wasm/qcms_bg.wasm?url";

export type PdfDoc = {
  numPages: number;
  // Dibuja la página n (desde 1) en la canvas y devuelve su tamaño en px CSS.
  renderPage(n: number, canvas: HTMLCanvasElement, scale: number): Promise<{ width: number; height: number }>;
  destroy(): void;
};

let workerStarted = false;

// El worker se crea una sola vez, cuando se abre el primer PDF. Con `?worker`
// Vite lo emite como .js y no dependemos del MIME de .mjs.
function startWorker() {
  if (workerStarted) return;
  GlobalWorkerOptions.workerPort = new PdfWorker();
  workerStarted = true;
}

const WASM: Record<string, string> = {
  "jbig2.wasm": jbig2Wasm,
  "openjpeg.wasm": openjpegWasm,
  "qcms_bg.wasm": qcmsWasm,
};

// LocalData le da a pdf.js los wasm del bundle (imágenes JBIG2 y JPEG 2000,
// perfiles de color). Para lo demás falla y pdf.js usa su alternativa.
class LocalData {
  async fetch({ kind, filename }: { kind: string; filename: string }): Promise<Uint8Array> {
    const url = kind === "wasmUrl" ? WASM[filename] : undefined;
    if (!url) throw new Error(`Sin datos locales para ${kind}/${filename}`);
    const res = await fetch(url);
    if (!res.ok) throw new Error(`No se pudo leer ${filename}`);
    return new Uint8Array(await res.arrayBuffer());
  }
}

class Cancelled extends Error {}

// loadPdf abre el PDF. pdf.js transfiere el buffer al worker, así que le
// pasamos una copia y `data` sigue sirviendo.
export async function loadPdf(data: Uint8Array): Promise<PdfDoc> {
  startWorker();
  const task = getDocument({
    data: data.slice(),
    // pdf.js 6 ya no tiene `isEvalSupported`: no usa eval ni new Function.
    enableXfa: false,
    useWorkerFetch: false,
    BinaryDataFactory: LocalData,
  });
  let doc;
  try {
    doc = await task.promise;
  } catch (e) {
    void task.destroy();
    throw e;
  }

  // Una canvas no admite dos render a la vez: el último pedido cancela al
  // anterior y espera a que termine.
  const gen = new WeakMap<HTMLCanvasElement, number>();
  const running = new WeakMap<HTMLCanvasElement, RenderTask>();

  return {
    numPages: doc.numPages,
    async renderPage(n, canvas, scale) {
      const mine = (gen.get(canvas) ?? 0) + 1;
      gen.set(canvas, mine);
      running.get(canvas)?.cancel();
      const page = await doc.getPage(n);
      await running.get(canvas)?.promise.catch(() => {});
      if (gen.get(canvas) !== mine) throw new Cancelled();

      const viewport = page.getViewport({ scale });
      const ratio = window.devicePixelRatio || 1;
      canvas.width = Math.floor(viewport.width * ratio);
      canvas.height = Math.floor(viewport.height * ratio);
      canvas.style.width = `${Math.floor(viewport.width)}px`;
      canvas.style.height = `${Math.floor(viewport.height)}px`;
      const render = page.render({
        canvas,
        viewport,
        transform: ratio !== 1 ? [ratio, 0, 0, ratio, 0, 0] : undefined,
      });
      running.set(canvas, render);
      try {
        await render.promise;
      } finally {
        if (running.get(canvas) === render) running.delete(canvas);
      }
      return { width: viewport.width, height: viewport.height };
    },
    destroy() {
      void task.destroy();
    },
  };
}
