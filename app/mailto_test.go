package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestParseMailto_Full(t *testing.T) {
	f, err := parseMailto("MAILTO:a@x.com,Bob%20%3Cb@x.com%3E,malo?to=c@x.com&cc=d@x.com,e@x.com&cc=f@x.com&bcc=g@x.com&subject=Hola%20mundo&body=l1%0D%0Al2%20%3Cb%3E")
	require.NoError(t, err)
	assert.Equal(t, []model.Address{{Email: "a@x.com"}, {Name: "Bob", Email: "b@x.com"}, {Email: "c@x.com"}}, f.To)
	assert.Equal(t, []model.Address{{Email: "d@x.com"}, {Email: "e@x.com"}, {Email: "f@x.com"}}, f.Cc)
	assert.Equal(t, []model.Address{{Email: "g@x.com"}}, f.Bcc)
	assert.Equal(t, "Hola mundo", f.Subject)
	assert.Equal(t, "l1\r\nl2 <b>", f.Body)
}

func TestParseMailto_IgnoresAttach(t *testing.T) {
	f, err := parseMailto("mailto:a@x.com?attach=/etc/passwd&attachment=x&in-reply-to=y")
	require.NoError(t, err)
	assert.Equal(t, []model.Address{{Email: "a@x.com"}}, f.To)
	assert.Empty(t, f.Cc)
	assert.Empty(t, f.Bcc)
	assert.Empty(t, f.Subject)
	assert.Empty(t, f.Body)
}

func TestParseMailto_Rejects(t *testing.T) {
	for _, bad := range []string{"https://x.com", "javascript:alert(1)", "mailto:" + strings.Repeat("a", 9000)} {
		_, err := parseMailto(bad)
		assert.Error(t, err, bad[:min(len(bad), 30)])
	}
}

func TestParseMailto_EmptyIsOK(t *testing.T) {
	f, err := parseMailto("mailto:")
	require.NoError(t, err)
	assert.Empty(t, f.To)
	assert.Empty(t, f.Subject)
}

func TestMailtoInitJSON_Keys(t *testing.T) {
	js, err := mailtoInitJSON(mailtoFields{Body: "a<b\r\nc\nd"}, model.Account{ID: 7, DefaultSignatureID: 3})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(js), &m))
	keys := []string{"accountId", "to", "cc", "bcc", "subject", "bodyHtml", "quotedHtml", "reply", "attachments", "originalAttachments", "skipOriginalAttachments", "signatureId"}
	assert.Len(t, m, len(keys))
	for _, k := range keys {
		assert.Contains(t, m, k)
	}
	for _, k := range []string{"to", "cc", "bcc", "attachments", "originalAttachments", "skipOriginalAttachments"} {
		assert.IsType(t, []any{}, m[k], k)
	}
	assert.Nil(t, m["reply"])
	assert.EqualValues(t, 3, m["signatureId"])
	assert.EqualValues(t, 7, m["accountId"])
	assert.Equal(t, "a&lt;b<br>c<br>d", m["bodyHtml"])
	assert.Equal(t, "", m["quotedHtml"])
}

func TestPickMailtoAccount(t *testing.T) {
	accs := []model.Account{{ID: 4}, {ID: 9}}
	a, ok := pickMailtoAccount(accs, 9)
	assert.True(t, ok)
	assert.EqualValues(t, 9, a.ID)
	for _, id := range []int64{0, 55} {
		a, ok = pickMailtoAccount(accs, id)
		assert.True(t, ok)
		assert.EqualValues(t, 4, a.ID)
	}
	_, ok = pickMailtoAccount(nil, 1)
	assert.False(t, ok)
}
