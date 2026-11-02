import type { Folder } from "../../bindings/postacontrol/core/model/models";

export type FolderNode = Folder & { label: string; children: FolderNode[] };

const ROLE_ORDER = ["inbox", "drafts", "sent", "archive", "junk", "trash"];
const ROLE_LABEL: Record<string, string> = {
  inbox: "Entrada",
  drafts: "Borradores",
  sent: "Enviados",
  archive: "Archivo",
  junk: "Spam",
  trash: "Papelera",
};

// Etiqueta en español por rol; sin rol, el último segmento del nombre.
export function folderLabel(f: Pick<Folder, "name" | "delimiter" | "role">): string {
  const segs = f.delimiter ? f.name.split(f.delimiter) : [f.name];
  return ROLE_LABEL[f.role] ?? segs[segs.length - 1];
}

function byLabel(a: FolderNode, b: FolderNode): number {
  return a.label.localeCompare(b.label, "es", { sensitivity: "base" });
}

// Arma la jerarquía a partir de Delimiter. Las carpetas con rol van siempre
// en la raíz (aunque cuelguen de INBOX); los padres que el servidor no lista
// se sintetizan como nodos no seleccionables.
export function buildFolderTree(folders: Folder[]): FolderNode[] {
  const nodes = new Map<string, FolderNode>();
  const roots: FolderNode[] = [];

  const segments = (f: Pick<Folder, "name" | "delimiter">) =>
    f.delimiter ? f.name.split(f.delimiter) : [f.name];

  for (const f of folders) {
    nodes.set(f.name, { ...f, label: folderLabel(f), children: [] });
  }

  const attach = (node: FolderNode) => {
    if (ROLE_LABEL[node.role]) {
      roots.push(node);
      return;
    }
    const segs = segments(node);
    if (segs.length < 2) {
      roots.push(node);
      return;
    }
    const parentName = segs.slice(0, -1).join(node.delimiter);
    let parent = nodes.get(parentName);
    if (!parent) {
      const psegs = segs.slice(0, -1);
      parent = {
        id: 0,
        accountId: node.accountId,
        name: parentName,
        delimiter: node.delimiter,
        role: "",
        unread: 0,
        total: 0,
        selectable: false, position: 0,
        label: psegs[psegs.length - 1],
        children: [],
      };
      nodes.set(parentName, parent);
      attach(parent);
    }
    parent.children.push(node);
  };

  for (const f of folders) attach(nodes.get(f.name)!);

  const rank = (n: FolderNode) => {
    const i = ROLE_ORDER.indexOf(n.role);
    return i === -1 ? ROLE_ORDER.length : i;
  };
  // Primero los que tienen posición manual (por posición); después los de
  // posición 0 en el orden de siempre: en la raíz por rango de rol y alfabético.
  const cmp = (isRoot: boolean) => (a: FolderNode, b: FolderNode) => {
    if (a.position > 0 || b.position > 0) {
      if (a.position === 0) return 1;
      if (b.position === 0) return -1;
      return a.position - b.position;
    }
    return (isRoot ? rank(a) - rank(b) : 0) || byLabel(a, b);
  };
  const sortRec = (list: FolderNode[], isRoot: boolean) => {
    list.sort(cmp(isRoot));
    list.forEach((n) => sortRec(n.children, false));
  };
  sortRec(roots, true);
  return roots;
}

// El nodo o algún descendiente es una carpeta especial (tiene rol).
export function isSpecialTree(node: FolderNode): boolean {
  return node.role !== "" || node.children.some(isSpecialTree);
}

// Se puede renombrar, mover y eliminar: no es sintetizada ni contiene especiales.
export function canModify(node: FolderNode): boolean {
  return node.id !== 0 && !isSpecialTree(node);
}

// El árbol se reconstruye en cada render, así que los nodos se comparan por
// nombre (único dentro de la cuenta) y no por identidad.
const has = (list: FolderNode[], node: FolderNode) => list.some((n) => n.name === node.name);

function parentList(tree: FolderNode[], node: FolderNode): FolderNode[] | null {
  if (has(tree, node)) return tree;
  for (const n of tree) {
    const r = parentList(n.children, node);
    if (r) return r;
  }
  return null;
}

// Los hermanos del nodo (incluido él), en el orden en que se ven.
export function siblingsOf(tree: FolderNode[], node: FolderNode): FolderNode[] {
  return parentList(tree, node) ?? [];
}

function parentOf(tree: FolderNode[], node: FolderNode): FolderNode | null {
  for (const n of tree) {
    if (has(n.children, node)) return n;
    const r = parentOf(n.children, node);
    if (r) return r;
  }
  return null;
}

// Padres posibles para mover el nodo: de la misma cuenta, seleccionables o no
// pero reales (id != 0), sin él, sus descendientes ni su padre actual.
export function moveTargets(tree: FolderNode[], node: FolderNode): FolderNode[] {
  const parent = parentOf(tree, node);
  const out: FolderNode[] = [];
  const walk = (list: FolderNode[]) => {
    for (const n of list) {
      if (n.name === node.name) continue; // tampoco sus descendientes
      if (n.id !== 0 && n.accountId === node.accountId && n.name !== parent?.name) out.push(n);
      walk(n.children);
    }
  };
  walk(tree);
  return out;
}

// Está en el primer nivel del árbol.
export function isRootLevel(tree: FolderNode[], node: FolderNode): boolean {
  return has(tree, node);
}
