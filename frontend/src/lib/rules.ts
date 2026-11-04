import type {
  Address,
  Folder,
  Label,
  Rule,
  RuleAction,
  RuleCondition,
} from "../../bindings/postacontrol/core/model/models";
import type { RuleDraft } from "./events";
import { buildFolderTree, folderLabel, type FolderNode } from "./folders";

// Lo que se puede elegir en una regla, en el orden en que se ofrece. Los
// valores son los del núcleo (core/mail/rules.go).
export const RULE_FIELDS = [
  { value: "from", label: "De" },
  { value: "to", label: "Para" },
  { value: "cc", label: "Cc" },
  { value: "toOrCc", label: "Para o Cc" },
  { value: "subject", label: "Asunto" },
] as const;

export const RULE_OPS = [
  { value: "contains", label: "contiene" },
  { value: "notContains", label: "no contiene" },
  { value: "is", label: "es" },
  { value: "isNot", label: "no es" },
  { value: "beginsWith", label: "empieza con" },
  { value: "endsWith", label: "termina con" },
] as const;

export const RULE_ACTIONS = [
  { value: "move", label: "Mover a la carpeta" },
  { value: "markRead", label: "Marcar como leído" },
  { value: "flag", label: "Destacar" },
  { value: "label", label: "Poner la etiqueta" },
  { value: "delete", label: "Mandar a la Papelera" },
  { value: "stop", label: "No aplicar las reglas que siguen" },
] as const;

function labelOf(list: readonly { value: string; label: string }[], value: string): string {
  return list.find((x) => x.value === value)?.label ?? value;
}

// Ruta legible de una carpeta: «Entrada», «Clientes / Acme».
export function folderPath(f: Pick<Folder, "name" | "delimiter" | "role">): string {
  if (f.role) return folderLabel(f);
  return f.delimiter ? f.name.split(f.delimiter).join(" / ") : f.name;
}

export function describeCondition(c: RuleCondition): string {
  return `${labelOf(RULE_FIELDS, c.field)} ${labelOf(RULE_OPS, c.op)} «${c.value}»`;
}

export function describeAction(a: RuleAction, folders: Folder[], labels: Label[]): string {
  if (a.type === "move") {
    const f = folders.find((x) => x.id === a.folderId);
    return f ? `Mover a ${folderPath(f)}` : "Mover a una carpeta que ya no existe";
  }
  if (a.type === "label") {
    const l = labels.find((x) => x.id === a.labelId);
    return l ? `Etiquetar ${l.name}` : "Poner una etiqueta que ya no existe";
  }
  return labelOf(RULE_ACTIONS, a.type);
}

// Resumen de una línea para la lista: «Si De contiene «x» → Mover a Banco».
export function describeRule(r: Rule, folders: Folder[], labels: Label[]): string {
  const conds = (r.conditions ?? []).map(describeCondition).join(r.matchAll ? " y " : " o ");
  const acts = (r.actions ?? []).map((a) => describeAction(a, folders, labels)).join(", ");
  return `Si ${conds} → ${acts}`;
}

// Acción que apunta a una carpeta o etiqueta que ya no está: la regla la saltea.
export function brokenAction(a: RuleAction, folders: Folder[], labels: Label[]): boolean {
  if (a.type === "move") return !folders.some((f) => f.id === a.folderId);
  if (a.type === "label") return !labels.some((l) => l.id === a.labelId);
  return false;
}

// Carpetas a las que una regla puede mover, en el orden de la barra lateral y
// con su ruta completa.
export function moveTargets(folders: Folder[]): { id: number; path: string }[] {
  const out: { id: number; path: string }[] = [];
  const walk = (nodes: FolderNode[], prefix: string) => {
    for (const n of nodes) {
      const path = prefix ? `${prefix} / ${n.label}` : n.label;
      if (n.selectable && n.id > 0) out.push({ id: n.id, path });
      walk(n.children, path);
    }
  };
  walk(buildFolderTree(folders), "");
  return out;
}

// Regla nueva para lo que llega de un remitente («Crear regla…» en un correo).
export function ruleDraftFromSender(accountId: number, from: Address): RuleDraft {
  return {
    accountId,
    name: from.name || from.email,
    conditions: [{ field: "from", op: "is", value: from.email }],
  };
}
