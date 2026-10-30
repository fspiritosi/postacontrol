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
    const segs = segments(f);
    nodes.set(f.name, {
      ...f,
      label: ROLE_LABEL[f.role] ?? segs[segs.length - 1],
      children: [],
    });
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
        selectable: false,
        label: psegs[psegs.length - 1],
        children: [],
      };
      nodes.set(parentName, parent);
      attach(parent);
    }
    parent.children.push(node);
  };

  for (const f of folders) attach(nodes.get(f.name)!);

  const sortRec = (list: FolderNode[]) => {
    list.sort(byLabel);
    list.forEach((n) => sortRec(n.children));
  };
  sortRec(roots);
  const rank = (n: FolderNode) => {
    const i = ROLE_ORDER.indexOf(n.role);
    return i === -1 ? ROLE_ORDER.length : i;
  };
  // sort estable: primero por rango de rol, el resto conserva orden alfabético.
  return roots.sort((a, b) => rank(a) - rank(b));
}
