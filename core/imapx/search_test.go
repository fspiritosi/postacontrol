package imapx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchHeader_FindsByMessageID(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	s.Deliver(t, "INBOX", plainMsg("dos", ""))
	s.Deliver(t, "INBOX", plainMsg("uno", "")) // otra versión del mismo Message-ID
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)

	uids, err := c.SearchHeader("Message-ID", "<uno@ej.com>")
	require.NoError(t, err)
	assert.Equal(t, []uint32{1, 3}, uids)

	uids, err = c.SearchHeader("Message-ID", "<inexistente@ej.com>")
	require.NoError(t, err)
	assert.Empty(t, uids)
}
