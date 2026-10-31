package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	gomail "github.com/emersion/go-message/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, data, 0o600))
	return p
}

func sparseFile(t *testing.T, name string, size int64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))
	require.NoError(t, f.Close())
	return p
}

func userErr(t *testing.T, err error) *UserError {
	t.Helper()
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	return ue
}

// attachmentsOf devuelve nombre → contenido de los adjuntos del correo recibido.
func attachmentsOf(t *testing.T, e testenv.Envelope) map[string][]byte {
	t.Helper()
	r := parseEnvelope(t, e)
	out := map[string][]byte{}
	for {
		p, err := r.NextPart()
		if err != nil {
			return out
		}
		if ah, ok := p.Header.(*gomail.AttachmentHeader); ok {
			name, _ := ah.Filename()
			b, _ := io.ReadAll(p.Body)
			out[name] = b
		}
	}
}

func TestStageFiles_StoresAndReturnsInfo(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	p1 := writeTemp(t, "nota.txt", []byte("hola"))
	p2 := writeTemp(t, "datos.pdf", pdfBytes)
	got, err := c.StageFiles(context.Background(), []string{p1, p2})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "nota.txt", got[0].Filename)
	assert.Contains(t, got[0].MIMEType, "text/plain")
	assert.EqualValues(t, 4, got[0].Size)
	assert.Equal(t, "datos.pdf", got[1].Filename)
	assert.Equal(t, "application/pdf", got[1].MIMEType)
	assert.EqualValues(t, len(pdfBytes), got[1].Size)

	info, err := c.StagedInfo(context.Background(), []int64{got[0].ID, got[1].ID})
	require.NoError(t, err)
	assert.Equal(t, got, info)
}

func TestStageFiles_KeepsNonASCIIName(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	name := "presupuesto año 2026 — versión final.pdf"
	got, err := c.StageFiles(context.Background(), []string{writeTemp(t, name, pdfBytes)})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, name, got[0].Filename)
}

func TestStageFiles_RejectsDirectoryAndTooLarge(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	ok := writeTemp(t, "ok.txt", []byte("ok"))

	dir := filepath.Join(t.TempDir(), "carpeta")
	require.NoError(t, os.Mkdir(dir, 0o700))
	_, err := c.StageFiles(ctx, []string{ok, dir})
	ue := userErr(t, err)
	assert.Equal(t, "attach", ue.Code)
	assert.Equal(t, "«carpeta» es una carpeta; solo se pueden adjuntar archivos.", ue.Msg)

	big := sparseFile(t, "enorme.iso", 25<<20+1)
	_, err = c.StageFiles(ctx, []string{ok, big})
	ue = userErr(t, err)
	assert.Equal(t, "attach", ue.Code)
	assert.Equal(t, "«enorme.iso» pesa 25.0 MB; el máximo por correo es 25.0 MB.", ue.Msg)

	_, err = c.StageFiles(ctx, []string{ok, filepath.Join(t.TempDir(), "fantasma.txt")})
	assert.Equal(t, "No pudimos leer «fantasma.txt».", userErr(t, err).Msg)

	// Ninguno quedó guardado.
	info, err := c.store.StagedInfo(ctx, []int64{1, 2, 3})
	require.NoError(t, err)
	assert.Empty(t, info)
}

func TestLoadImage_TypesAndLimit(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	img, err := c.LoadImage(ctx, writeTemp(t, "foto.png", buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, "foto.png", img.Filename)
	assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(buf.Bytes()), img.DataURL)

	_, err = c.LoadImage(ctx, writeTemp(t, "falsa.png", []byte("esto es texto")))
	ue := userErr(t, err)
	assert.Equal(t, "image", ue.Code)
	assert.Equal(t, "Solo se pueden insertar imágenes PNG, JPG, GIF o WebP.", ue.Msg)

	big := filepath.Join(t.TempDir(), "grande.png")
	require.NoError(t, os.WriteFile(big, append(buf.Bytes(), make([]byte, 5<<20)...), 0o600))
	_, err = c.LoadImage(ctx, big)
	ue = userErr(t, err)
	assert.Equal(t, "image", ue.Code)
	assert.Equal(t, "La imagen pesa 5.0 MB; el máximo es 5.0 MB.", ue.Msg)
}

func TestFormatSize(t *testing.T) {
	assert.Equal(t, "500 B", formatSize(500))
	assert.Equal(t, "2 KB", formatSize(2048))
	assert.Equal(t, "1.5 MB", formatSize(3<<19))
	assert.Equal(t, "25.0 MB", formatSize(25<<20))
}

func TestSend_WithStagedAttachments(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	staged, err := c.StageFiles(ctx, []string{writeTemp(t, "a.pdf", pdfBytes), writeTemp(t, "b.txt", []byte("bee"))})
	require.NoError(t, err)

	_, err = c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "adj", BodyHTML: "<p>x</p>",
		Attachments: []int64{staged[0].ID, staged[1].ID},
	})
	require.NoError(t, err)
	got := attachmentsOf(t, smtp.Received()[0])
	assert.Equal(t, pdfBytes, got["a.pdf"])
	assert.Equal(t, []byte("bee"), got["b.txt"])

	info, err := c.StagedInfo(ctx, []int64{staged[0].ID, staged[1].ID})
	require.NoError(t, err)
	assert.Empty(t, info)
}

func TestSend_TooLargeIsRejectedBeforeQueue(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	// Se prepara por el store: StageFiles ya rechaza cada archivo por separado.
	var ids []int64
	for _, n := range []string{"a", "b"} {
		s, err := c.store.StageAttachment(ctx, n, "application/octet-stream", make([]byte, 13<<20))
		require.NoError(t, err)
		ids = append(ids, s.ID)
	}
	_, err := c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s", BodyHTML: "<p>x</p>", Attachments: ids,
	})
	ue := userErr(t, err)
	assert.Equal(t, "too_large", ue.Code)
	assert.Equal(t, "El correo pesa 26.0 MB y el máximo es 25.0 MB. Sacá algún adjunto.", ue.Msg)
	ds, err := c.ListLocalDrafts(ctx)
	require.NoError(t, err)
	assert.Empty(t, ds)

	// También cuentan las imágenes pegadas en el cuerpo.
	body := `<img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(make([]byte, 13<<20)) + `">`
	_, err = c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s", BodyHTML: body, Attachments: ids[:1],
	})
	assert.Equal(t, "too_large", userErr(t, err).Code)

	// Un id inexistente tampoco guarda nada.
	_, err = c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s", BodyHTML: "<p>x</p>", Attachments: []int64{9999},
	})
	ue = userErr(t, err)
	assert.Equal(t, "attach", ue.Code)
	assert.Equal(t, "Uno de los adjuntos ya no está disponible. Volvé a adjuntarlo.", ue.Msg)
	ds, err = c.ListLocalDrafts(ctx)
	require.NoError(t, err)
	assert.Empty(t, ds)
}

func TestSend_ForwardSkipsOriginalAttachments(t *testing.T) {
	body := "--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nadjuntos\r\n" +
		"--b\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\nContent-Disposition: attachment; filename=\"uno.pdf\"\r\n\r\n" +
		wrap76(base64.StdEncoding.EncodeToString(pdfBytes)) +
		"--b\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\nContent-Disposition: attachment; filename=\"dos.txt\"\r\n\r\n" +
		wrap76(base64.StdEncoding.EncodeToString([]byte("segundo"))) +
		"--b--\r\n"
	c, _, smtp, acc, _, m := prepared(t, mailWith("adj2", "multipart/mixed; boundary=b", body))
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Fwd: adj2", BodyHTML: "<p>mirá</p>",
		Reply:                   &model.ReplyContext{Mode: "forward", OriginalMessageID: m.ID},
		SkipOriginalAttachments: []int{0},
	})
	require.NoError(t, err)
	got := attachmentsOf(t, smtp.Received()[0])
	assert.Len(t, got, 1)
	assert.Equal(t, []byte("segundo"), got["dos.txt"])
}

func TestSend_FailureKeepsStagedForRetry(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	staged, err := c.StageFiles(ctx, []string{writeTemp(t, "a.pdf", pdfBytes)})
	require.NoError(t, err)
	_, err = c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "s", BodyHTML: "<p>x</p>",
		Attachments: []int64{staged[0].ID},
	})
	assert.Equal(t, "rejected", userErr(t, err).Code)
	ds, err := c.ListLocalDrafts(ctx)
	require.NoError(t, err)
	require.Len(t, ds, 1)
	assert.Equal(t, []int64{staged[0].ID}, ds[0].Request.Attachments)
	info, err := c.StagedInfo(ctx, []int64{staged[0].ID})
	require.NoError(t, err)
	assert.Equal(t, staged, info)
}

func TestPruneStaged_KeepsReferencedByOutbox(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	staged, err := c.StageFiles(ctx, []string{writeTemp(t, "a.pdf", pdfBytes)})
	require.NoError(t, err)
	_, err = c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "s", BodyHTML: "<p>x</p>",
		Attachments: []int64{staged[0].ID},
	})
	require.Error(t, err)
	// Un tope en el futuro vuelve "viejo" a todo: solo sobrevive lo de la bandeja.
	require.NoError(t, c.store.PruneStagedAttachments(ctx, time.Now().Add(time.Hour)))
	info, err := c.StagedInfo(ctx, []int64{staged[0].ID})
	require.NoError(t, err)
	assert.Len(t, info, 1)
}

func TestDeleteLocalDraft_DeletesItsStaged(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	staged, err := c.StageFiles(ctx, []string{writeTemp(t, "a.pdf", pdfBytes)})
	require.NoError(t, err)
	_, err = c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "s", BodyHTML: "<p>x</p>",
		Attachments: []int64{staged[0].ID},
	})
	require.Error(t, err)
	ds, err := c.ListLocalDrafts(ctx)
	require.NoError(t, err)
	require.Len(t, ds, 1)
	require.NoError(t, c.DeleteLocalDraft(ctx, ds[0].ID))
	info, err := c.StagedInfo(ctx, []int64{staged[0].ID})
	require.NoError(t, err)
	assert.Empty(t, info)
}

func TestPrepareCompose_ForwardListsOriginalAttachments(t *testing.T) {
	c, _, _, _, _, m := prepared(t, attachmentMail("factura.pdf"))
	ctx := context.Background()
	d, err := c.PrepareCompose(ctx, m.ID, "forward")
	require.NoError(t, err)
	require.Len(t, d.OriginalAttachments, 1)
	assert.Equal(t, "factura.pdf", d.OriginalAttachments[0].Filename)
	assert.Equal(t, 0, d.OriginalAttachments[0].Index)

	d, err = c.PrepareCompose(ctx, m.ID, "reply")
	require.NoError(t, err)
	assert.NotNil(t, d.OriginalAttachments)
	assert.Empty(t, d.OriginalAttachments)
}
