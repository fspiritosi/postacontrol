import type { Address, MessageDetail } from "../../bindings/postacontrol/core/model/models";
import { fullDate } from "./format";
import { buildSrcDoc } from "./srcdoc";

type Printable = Pick<MessageDetail, "subject" | "date" | "html" | "text"> & {
  from: Address;
  to: Address[] | null;
  cc: Address[] | null;
};

const esc = (s: string) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

const addr = (a: Address) => (a.name ? `${a.name} <${a.email}>` : a.email);

// El documento que se imprime: encabezado (de, para, fecha, asunto) y el
// cuerpo, con la misma CSP del lector. Las imágenes remotas, solo si se
// permitieron.
export function printDocument(m: Printable, allowRemote: boolean): string {
  const rows: [string, string][] = [
    ["De", addr(m.from)],
    ["Para", (m.to ?? []).map(addr).join(", ")],
  ];
  if (m.cc?.length) rows.push(["CC", m.cc.map(addr).join(", ")]);
  rows.push(["Fecha", fullDate(m.date)]);
  const header =
    `<h1 style="font-size:18px;margin:0 0 8px">${esc(m.subject || "(sin asunto)")}</h1>` +
    `<table style="font-size:12px;color:#444;margin-bottom:12px;border-collapse:collapse">` +
    rows.map(([k, v]) => `<tr><td style="padding:1px 8px 1px 0;font-weight:600">${k}</td><td>${esc(v)}</td></tr>`).join("") +
    `</table><hr style="border:0;border-top:1px solid #ccc;margin:0 0 12px">`;
  const body = m.html ? m.html : `<pre style="white-space:pre-wrap;font:inherit">${esc(m.text)}</pre>`;
  return buildSrcDoc(header + body, { allowRemote });
}

// Imprime el correo desde un iframe aparte, para que salga solo el correo y
// no la app. El diálogo de impresión del sistema ofrece guardarlo como PDF.
// allow-same-origin deja llamar a print() desde acá; sin allow-scripts, el
// correo no ejecuta nada.
export function printMessage(m: Printable, allowRemote: boolean): void {
  const frame = document.createElement("iframe");
  frame.setAttribute("sandbox", "allow-same-origin allow-modals");
  frame.setAttribute("aria-hidden", "true");
  frame.style.cssText = "position:fixed;width:0;height:0;border:0;right:0;bottom:0";
  frame.srcdoc = printDocument(m, allowRemote);
  frame.onload = () => {
    frame.contentWindow?.focus();
    frame.contentWindow?.print();
    // print() espera al diálogo; después ya no hace falta el iframe.
    setTimeout(() => frame.remove(), 1000);
  };
  document.body.appendChild(frame);
}
