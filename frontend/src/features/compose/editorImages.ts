import { formatSize } from "@/lib/format";

export const IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];
export const MAX_INLINE_IMAGE_BYTES = 5 * 1024 * 1024;

// Mismos textos y límites que LoadImage en Go: la validación de acá es solo para avisar antes.
export function imageError(file: { type: string; size: number }): string | null {
  if (!IMAGE_TYPES.includes(file.type)) {
    return "Solo se pueden insertar imágenes PNG, JPG, GIF o WebP.";
  }
  if (file.size > MAX_INLINE_IMAGE_BYTES) {
    return `La imagen pesa ${formatSize(file.size)}; el máximo es ${formatSize(MAX_INLINE_IMAGE_BYTES)}.`;
  }
  return null;
}

export function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}
