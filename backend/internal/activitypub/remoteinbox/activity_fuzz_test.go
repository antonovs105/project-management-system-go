package remoteinbox

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzParseActivity checks the public inbox parser against arbitrary JSON and
// preserves the identity invariants required before any database mutation.
func FuzzParseActivity(f *testing.F) {
	f.Add([]byte(`{"id":"https://remote.example/activities/1","type":"Create","actor":"https://remote.example/users/alice","object":{"id":"https://remote.example/notes/1","type":"Note","content":"hello"}}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"id":1,"type":[],"actor":false}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1024*1024 {
			t.Skip()
		}
		activity, err := parseActivity(body)
		if err != nil {
			return
		}
		require.NotNil(t, activity)
		require.True(t, absoluteURI(activity.ID))
		require.True(t, absoluteURI(activity.ActorAPID))
		require.True(t, bytes.Equal(body, activity.Document))
	})
}

func absoluteURI(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.IsAbs()
}
