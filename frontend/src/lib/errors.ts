// Un error de una llamada enlazada rechaza con un RuntimeError (Error con
// `.message` = Msg en español del UserError del núcleo y `.cause` opcional).
// Se aceptan también strings y objetos con `message`.
export function errorMessage(e: unknown): string {
  if (typeof e === "string" && e) return e;
  if (e && typeof e === "object" && "message" in e) {
    const m = (e as { message: unknown }).message;
    if (typeof m === "string" && m) return m;
  }
  return "Ocurrió un error inesperado.";
}

// Contrato con Go: core/mail/read.go (código "stale"); lo cubre un test allá.
export const STALE_MESSAGE_FRAGMENT = "cambió en el servidor";

export function isStaleError(e: unknown): boolean {
  return errorMessage(e).includes(STALE_MESSAGE_FRAGMENT);
}

// Contrato con Go: msgNotFound de core/mail/errors.go (código "not_found").
export const NOT_FOUND_MESSAGE = "No encontramos lo que buscabas.";

export function isNotFoundError(e: unknown): boolean {
  return errorMessage(e) === NOT_FOUND_MESSAGE;
}
