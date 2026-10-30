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
