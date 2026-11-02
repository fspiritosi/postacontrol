import type { Contact } from "../../bindings/postacontrol/core/model/models";

// Lo que se muestra de un contacto: su nombre, o si no tiene, su primer correo.
export const contactDisplayName = (c: Contact) => c.name || (c.emails ?? [])[0] || "";
