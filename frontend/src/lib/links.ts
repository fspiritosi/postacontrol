// Enlaces permitidos en el redactor: http, https y mailto.
const ALLOWED_URI = /^(https?:|mailto:)/i;

export function isAllowedLink(url: string): boolean {
  return ALLOWED_URI.test(url);
}

// toLinkHref normaliza lo que escribió el usuario: sin esquema se asume
// https://. Devuelve null si el esquema no está permitido.
export function toLinkHref(input: string): string | null {
  const url = input.trim();
  const href = /^[a-z][a-z0-9+.-]*:/i.test(url) ? url : `https://${url}`;
  return isAllowedLink(href) ? href : null;
}
