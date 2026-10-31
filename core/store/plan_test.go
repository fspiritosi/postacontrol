package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// queryPlan devuelve las líneas de EXPLAIN QUERY PLAN de la consulta.
func queryPlan(t *testing.T, s *Store, query string, args ...any) []string {
	t.Helper()
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN `+query, args...)
	require.NoError(t, err)
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		out = append(out, detail)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestListThreads_PlanUsesFolderThreadIndex(t *testing.T) {
	s := newStore(t)
	for _, scope := range []int{1, 2} {
		args := []any{}
		for range 2 * scope {
			args = append(args, int64(1))
		}
		args = append(args, int64(1), 50, 0)
		plan := queryPlan(t, s, threadsSQL(scope, false), args...)
		t.Logf("alcance %d:\n%s", scope, strings.Join(plan, "\n"))
		// El "último del hilo" (subconsulta correlacionada sobre y) tiene que ir por
		// (folder_id, thread_root, date), no recorrer la carpeta entera.
		found := false
		for _, l := range plan {
			if strings.Contains(l, "SEARCH y USING") {
				require.Contains(t, l, "idx_messages_folder_thread", "plan:\n%s", strings.Join(plan, "\n"))
				require.Contains(t, l, "thread_root=?", "plan:\n%s", strings.Join(plan, "\n"))
				found = true
			}
			require.NotContains(t, l, "SCAN y", "plan:\n%s", strings.Join(plan, "\n"))
		}
		require.True(t, found, "plan:\n%s", strings.Join(plan, "\n"))
	}
}
