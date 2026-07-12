package netguard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzValidateRemoteURL checks that arbitrary federation targets are handled
// without panics and that accepted targets remain absolute HTTP(S) URLs.
func FuzzValidateRemoteURL(f *testing.F) {
	f.Add("https://example.com/users/alice")
	f.Add("http://127.0.0.1/latest/meta-data")
	f.Add(":// malformed")

	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64*1024 {
			t.Skip()
		}
		parsed, err := ValidateRemoteURL(raw)
		if err != nil {
			return
		}
		require.NotNil(t, parsed)
		require.Contains(t, []string{"http", "https"}, strings.ToLower(parsed.Scheme))
		require.NotEmpty(t, parsed.Host)
	})
}
