// Package mimex lee y arma mensajes MIME: decide qué partes son el cuerpo, las
// imágenes en línea y los adjuntos, y decodifica transfer-encodings y charsets.
package mimex

import (
	"strings"

	"github.com/emersion/go-imap/v2"

	"postacontrol/core/model"
)

// InlinePart es una parte con Content-ID referenciada desde el HTML como cid:.
type InlinePart struct {
	Part      []int
	ContentID string // sin los <>
	MIMEType  string
}

// PartPlan dice qué parte de un mensaje es cada cosa.
type PartPlan struct {
	// TextPart y HTMLPart son nil si no existen. Si el mensaje no es multipart,
	// la ruta es []int{1}.
	TextPart, HTMLPart []int
	Inline             []InlinePart
	Attachments        []model.Attachment
}

// Plan recorre la BODYSTRUCTURE y reparte las partes. En multipart/alternative
// se prefiere la última variante de cada tipo; en otros contenedores, la primera.
func Plan(bs imap.BodyStructure) PartPlan {
	p := PartPlan{Inline: []InlinePart{}, Attachments: []model.Attachment{}}
	switch b := bs.(type) {
	case *imap.BodyStructureSinglePart:
		p.visit(b, []int{1}, false, false)
	case *imap.BodyStructureMultiPart:
		p.visit(b, nil, false, false)
	}
	return p
}

func (p *PartPlan) visit(bs imap.BodyStructure, path []int, inAlternative, inRelated bool) {
	switch b := bs.(type) {
	case *imap.BodyStructureMultiPart:
		sub := strings.ToLower(b.Subtype)
		for i, child := range b.Children {
			childPath := append(append([]int{}, path...), i+1)
			p.visit(child, childPath, sub == "alternative", inRelated || sub == "related")
		}
	case *imap.BodyStructureSinglePart:
		p.visitSingle(b, path, inAlternative, inRelated)
	}
}

func (p *PartPlan) visitSingle(b *imap.BodyStructureSinglePart, path []int, inAlternative, inRelated bool) {
	mt := b.MediaType()
	disp := b.Disposition()
	isAttachment := disp != nil && strings.EqualFold(disp.Value, "attachment")
	name := partFilename(b)
	isText := strings.HasPrefix(mt, "text/")

	switch {
	case isAttachment:
		p.addAttachment(b, path, name)
	case isText && name == "" && (mt == "text/plain" || mt == "text/html"):
		dst := &p.TextPart
		if mt == "text/html" {
			dst = &p.HTMLPart
		}
		if *dst == nil || inAlternative {
			*dst = path
		}
	case !isText && inRelated && strings.Trim(b.ID, "<> ") != "":
		p.Inline = append(p.Inline, InlinePart{Part: path, ContentID: strings.Trim(b.ID, "<> "), MIMEType: mt})
	// Una parte text/* con nombre es un archivo adjunto, no el cuerpo.
	case !isText && name != "", isText && name != "":
		p.addAttachment(b, path, name)
	}
}

func (p *PartPlan) addAttachment(b *imap.BodyStructureSinglePart, path []int, name string) {
	p.Attachments = append(p.Attachments, model.Attachment{
		Index:    len(p.Attachments),
		Filename: name,
		MIMEType: b.MediaType(),
		Size:     int64(b.Size),
		Part:     path,
	})
}
