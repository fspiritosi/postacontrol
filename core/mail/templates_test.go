package mail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestTemplates_CRUD(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()

	var ue *UserError
	_, err := c.SaveTemplate(ctx, model.Template{Name: "  ", HTML: "<p>x</p>"})
	require.ErrorAs(t, err, &ue)

	tpl, err := c.SaveTemplate(ctx, model.Template{Name: " Presupuesto ", Subject: " Presupuesto ", HTML: `<p>Hola</p><script>x()</script>`})
	require.NoError(t, err)
	assert.Equal(t, "Presupuesto", tpl.Name)
	assert.Equal(t, "Presupuesto", tpl.Subject)
	assert.NotContains(t, tpl.HTML, "script", "el HTML se sanea")
	assert.NotEmpty(t, n.matching(EventTemplatesChanged))

	tpl.Name = "Presupuesto anual"
	_, err = c.SaveTemplate(ctx, tpl)
	require.NoError(t, err)
	list, err := c.ListTemplates(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Presupuesto anual", list[0].Name)

	require.NoError(t, c.DeleteTemplate(ctx, tpl.ID))
	list, err = c.ListTemplates(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
	_, err = c.SaveTemplate(ctx, tpl)
	require.ErrorAs(t, err, &ue)
}

func TestBackup_IncludesTemplates(t *testing.T) {
	src, _, _, _ := newTestCore(t)
	ctx := context.Background()
	_, err := src.SaveTemplate(ctx, model.Template{Name: "Gracias", Subject: "Gracias", HTML: "<p>¡Gracias!</p>"})
	require.NoError(t, err)
	_, err = src.SaveTemplate(ctx, model.Template{Name: "Turno", HTML: "<p>Te confirmo el turno</p>"})
	require.NoError(t, err)
	path := exportTo(t, src)

	dst, _, _, _ := newTestCore(t)
	_, err = dst.SaveTemplate(ctx, model.Template{Name: "gracias", HTML: "<p>mía</p>"})
	require.NoError(t, err)
	got, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 1, got.TemplatesAdded)
	assert.Equal(t, 1, got.TemplatesSkipped)
	list, err := dst.ListTemplates(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "<p>mía</p>", list[0].HTML, "la que ya estaba no se pisa")
}
