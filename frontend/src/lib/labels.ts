import type { CSSProperties } from "react";
import type { Label } from "../../bindings/postacontrol/core/model/models";

// Paleta de etiquetas, en este orden.
export const LABEL_COLORS: readonly string[] = [
  "#dc2626",
  "#ea580c",
  "#d97706",
  "#65a30d",
  "#16a34a",
  "#0d9488",
  "#0891b2",
  "#2563eb",
  "#4f46e5",
  "#7c3aed",
  "#db2777",
  "#475569",
];

// Color de una etiqueta nueva: el primero que no use otra, o el primero si están todos usados.
export function nextColor(used: string[]): string {
  return LABEL_COLORS.find((c) => !used.includes(c)) ?? LABEL_COLORS[0];
}

// Estilo de la pastilla: legible en tema claro y oscuro.
export function labelChipStyle(color: string): CSSProperties {
  return {
    color: `color-mix(in oklab, ${color} 78%, var(--foreground))`,
    backgroundColor: `color-mix(in oklab, ${color} 12%, transparent)`,
    borderColor: `color-mix(in oklab, ${color} 40%, transparent)`,
  };
}

// Las etiquetas de `labels` cuyo id está en `ids`, en el orden de `labels`.
export function orderedLabels(ids: number[], labels: Label[]): Label[] {
  return labels.filter((l) => ids.includes(l.id));
}
