// Stub: la Task 6 lo reemplaza por el render con pdf.js y usa `data` y `zoom`.
export function PdfView(props: { data: Uint8Array; zoom: number | null }) {
  return (
    <p className="text-sm text-zinc-400" data-bytes={props.data.length}>
      Vista previa de PDF próximamente
    </p>
  );
}
