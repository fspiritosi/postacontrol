const BASE_CSS =
  "body{margin:16px;font:14px/1.5 system-ui;color:#111;background:#fff;word-wrap:break-word} img{max-width:100%;height:auto}";

function csp(allowRemote: boolean): string {
  return `default-src 'none'; style-src 'unsafe-inline'; img-src data:${allowRemote ? " https: http:" : ""}; font-src data:`;
}

// El HTML ya llega sanitizado por Go. Acá solo se arma el documento con su CSP
// y, a pedido, se activan las imágenes remotas (data-remote-src -> src).
export function buildSrcDoc(cleanHtml: string, opts: { allowRemote: boolean }): string {
  const parsed = new DOMParser().parseFromString(cleanHtml, "text/html");
  if (opts.allowRemote) {
    for (const el of parsed.querySelectorAll("[data-remote-src]")) {
      el.setAttribute("src", el.getAttribute("data-remote-src") ?? "");
      el.removeAttribute("data-remote-src");
    }
  }
  // DOMParser manda a <head> los <style> iniciales del fragmento: se conservan.
  const headStyles = Array.from(parsed.head.querySelectorAll("style"))
    .map((s) => s.outerHTML)
    .join("");
  return (
    `<!doctype html><html><head><meta charset="utf-8">` +
    `<meta http-equiv="Content-Security-Policy" content="${csp(opts.allowRemote)}">` +
    `<base target="_blank"><style>${BASE_CSS}</style>${headStyles}</head>` +
    `<body>${parsed.body.innerHTML}</body></html>`
  );
}
