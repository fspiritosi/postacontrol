package taskapp

import "strings"

// publicDomains son dominios de correo de uso general: el dominio no dice
// nada sobre la empresa del remitente, así que nunca se usa para sugerir.
var publicDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "hotmail.com": true, "hotmail.com.ar": true,
	"outlook.com": true, "outlook.com.ar": true, "live.com": true, "live.com.ar": true,
	"yahoo.com": true, "yahoo.com.ar": true, "icloud.com": true, "me.com": true,
	"msn.com": true, "aol.com": true, "proton.me": true, "protonmail.com": true,
	"gmx.com": true, "fibertel.com.ar": true, "speedy.com.ar": true, "arnet.com.ar": true,
}

// prefijosAsunto son los prefijos de respuesta y reenvío que CleanSubject saca.
var prefijosAsunto = map[string]bool{
	"re": true, "rv": true, "fw": true, "fwd": true, "reenviar": true, "tr": true,
}

// IsPublicDomain dice si el dominio es de correo público (sin distinguir
// mayúsculas).
func IsPublicDomain(domain string) bool {
	return publicDomains[strings.ToLower(strings.TrimSpace(domain))]
}

// SuggestProject devuelve el ID del proyecto activo que corresponde al
// remitente, o 0 si no hay sugerencia. Primero busca la dirección exacta
// entre los aprobadores; si no aparece, busca el dominio exacto (sin
// subdominios) salvo que sea público. Si en un paso coinciden varios
// proyectos distintos, devuelve 0 sin seguir al paso siguiente.
func SuggestProject(sender string, projects []Project) int64 {
	sender = strings.ToLower(strings.TrimSpace(sender))
	arroba := strings.LastIndex(sender, "@")
	if arroba < 0 {
		return 0
	}
	dominio := sender[arroba+1:]

	if id, ok := unicoProyecto(projects, func(aprobador string) bool {
		return aprobador == sender
	}); ok {
		return id
	}
	if IsPublicDomain(dominio) {
		return 0
	}
	if id, ok := unicoProyecto(projects, func(aprobador string) bool {
		return dominioDe(aprobador) == dominio
	}); ok {
		return id
	}
	return 0
}

// unicoProyecto devuelve el ID del único proyecto activo con algún aprobador
// para el que coincide(aprobador) sea true. ok es false si ninguno o más de
// uno cumplen.
func unicoProyecto(projects []Project, coincide func(aprobador string) bool) (int64, bool) {
	var encontrado int64
	cantidad := 0
	for _, p := range projects {
		if p.Status != "active" {
			continue
		}
		for _, a := range p.ApproverEmails {
			if coincide(strings.ToLower(strings.TrimSpace(a))) {
				cantidad++
				encontrado = p.ID
				break
			}
		}
	}
	return encontrado, cantidad == 1
}

// dominioDe devuelve la parte después de la última arroba, o "" si no hay.
func dominioDe(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	return email[i+1:]
}

// CleanSubject saca, en bucle, los prefijos de respuesta y reenvío (Re:, RV:,
// Fwd:, etc.) del asunto. Conserva las mayúsculas del resto y recorta espacios.
func CleanSubject(subject string) string {
	s := strings.TrimSpace(subject)
	for {
		i := strings.Index(s, ":")
		if i < 0 {
			break
		}
		if !prefijosAsunto[strings.ToLower(strings.TrimSpace(s[:i]))] {
			break
		}
		s = strings.TrimSpace(s[i+1:])
	}
	return s
}
