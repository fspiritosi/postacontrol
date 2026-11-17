package mimex

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/emersion/go-message"
)

const stripMixed = "From: Ana <ana@ej.com>\r\n" +
	"To: bob@ej.com\r\n" +
	"Subject: Informe\r\n" +
	"Message-ID: <abc@ej.com>\r\n" +
	"Date: Mon, 16 Nov 2026 10:00:00 +0000\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=\"MIX\"\r\n\r\n" +
	"--MIX\r\n" +
	"Content-Type: multipart/alternative; boundary=\"ALT\"\r\n\r\n" +
	"--ALT\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHola texto\r\n" +
	"--ALT\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Hola html</p>\r\n" +
	"--ALT--\r\n\r\n" +
	"--MIX\r\n" +
	"Content-Type: application/pdf; name=\"=?UTF-8?Q?informe.pdf?=\"\r\n" +
	"Content-Disposition: attachment\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n" +
	"JVBERi0xLjQK\r\n" +
	"--MIX--\r\n"

func TestStrip_KeepsBodies(t *testing.T) {
	out, dropped, err := StripAttachments([]byte(stripMixed))
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 1 || dropped[0] != (DroppedPart{"informe.pdf", 9}) {
		t.Fatalf("dropped = %+v", dropped)
	}
	s := string(out)
	for _, want := range []string{"Hola texto", "<p>Hola html</p>", "multipart/alternative"} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en:\n%s", want, s)
		}
	}
	if strings.Contains(s, "JVBER") || strings.Contains(s, "application/pdf") {
		t.Errorf("el adjunto sigue en:\n%s", s)
	}
	e, err := message.Read(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("no parsea: %v", err)
	}
	mr := e.MultipartReader()
	if mr == nil {
		t.Fatal("se esperaba multipart")
	}
	n := 0
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n++
		_ = p
	}
	if n != 1 {
		t.Errorf("partes de primer nivel = %d, quería 1", n)
	}
}

func TestStrip_NoAttachmentsUnchanged(t *testing.T) {
	raw := []byte("From: a@ej.com\r\nSubject: hola\r\nContent-Type: text/plain\r\n\r\ncuerpo\r\n")
	out, dropped, err := StripAttachments(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) || len(dropped) != 0 {
		t.Errorf("out=%q dropped=%v", out, dropped)
	}
}

func TestStrip_KeepsHeaders(t *testing.T) {
	out, _, err := StripAttachments([]byte(stripMixed))
	if err != nil {
		t.Fatal(err)
	}
	e, err := message.Read(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"From": "Ana <ana@ej.com>", "Subject": "Informe",
		"Message-Id": "<abc@ej.com>", "Date": "Mon, 16 Nov 2026 10:00:00 +0000",
	} {
		if got := e.Header.Get(k); got != v {
			t.Errorf("%s = %q, quería %q", k, got, v)
		}
	}
}

func TestStrip_SingleLeafAttachment(t *testing.T) {
	raw := "From: a@ej.com\r\nSubject: solo pdf\r\nContent-Type: application/pdf; name=\"x.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQK\r\n"
	out, dropped, err := StripAttachments([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 1 || dropped[0] != (DroppedPart{"x.pdf", 9}) {
		t.Fatalf("dropped = %+v", dropped)
	}
	e, err := message.Read(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if e.Header.Get("Subject") != "solo pdf" {
		t.Errorf("subject = %q", e.Header.Get("Subject"))
	}
	mt, _, _ := e.Header.ContentType()
	if mt != "text/plain" {
		t.Errorf("content-type = %q", mt)
	}
	if b, _ := io.ReadAll(e.Body); len(bytes.TrimSpace(b)) != 0 {
		t.Errorf("cuerpo no vacío: %q", b)
	}
}

func TestStrip_EmptyMultipartDropped(t *testing.T) {
	raw := "From: a@ej.com\r\nContent-Type: multipart/mixed; boundary=\"M\"\r\n\r\n" +
		"--M\r\nContent-Type: text/plain\r\n\r\nhola\r\n" +
		"--M\r\nContent-Type: multipart/mixed; boundary=\"N\"\r\n\r\n" +
		"--N\r\nContent-Type: image/png\r\nContent-ID: <i1>\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw0K\r\n--N--\r\n" +
		"--M--\r\n"
	out, dropped, err := StripAttachments([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 1 || dropped[0].Filename != "" || dropped[0].Size != 6 {
		t.Fatalf("dropped = %+v", dropped)
	}
	if strings.Contains(string(out), "boundary=\"N\"") || strings.Contains(string(out), "iVBOR") {
		t.Errorf("multipart vacío sigue:\n%s", out)
	}
	if !strings.Contains(string(out), "hola") {
		t.Errorf("falta el texto:\n%s", out)
	}
}

func TestStrip_Malformed(t *testing.T) {
	raw := "From: a@ej.com\r\nContent-Type: multipart/mixed\r\n\r\nhola\r\n"
	if _, _, err := StripAttachments([]byte(raw)); err == nil {
		t.Error("se esperaba error por falta de boundary")
	}
}
