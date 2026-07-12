package githubintegration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzNormalizeRepositoryInput checks GitHub identifiers at the external input
// boundary and preserves the same grammar after whitespace normalization.
func FuzzNormalizeRepositoryInput(f *testing.F) {
	f.Add("octo-org", "progo")
	f.Add(" owner ", " repository ")
	f.Add("", "../private")

	f.Fuzz(func(t *testing.T, owner, name string) {
		if len(owner)+len(name) > 64*1024 {
			t.Skip()
		}
		normalizedOwner, normalizedName, err := normalizeRepositoryInput(owner, name)
		if err != nil {
			return
		}
		require.Equal(t, strings.TrimSpace(owner), normalizedOwner)
		require.Equal(t, strings.TrimSpace(name), normalizedName)
		require.True(t, githubNamePattern.MatchString(normalizedOwner))
		require.True(t, githubNamePattern.MatchString(normalizedName))
	})
}
