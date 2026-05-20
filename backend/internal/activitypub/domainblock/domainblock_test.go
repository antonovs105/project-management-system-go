package domainblock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	assert.Equal(t, "example.test", Normalize(" HTTPS://Example.Test/users/alice "))
	assert.Equal(t, "example.test", Normalize("Example.Test:443"))
	assert.Empty(t, Normalize("bad/domain"))
}

func TestFromActorID(t *testing.T) {
	domain, err := FromActorID("https://team.example.test/users/alice")

	require.NoError(t, err)
	assert.Equal(t, "team.example.test", domain)
}

func TestCandidates(t *testing.T) {
	assert.Equal(t, []string{"team.example.test", "example.test", "test"}, Candidates("team.example.test"))
}

func TestContains(t *testing.T) {
	blocked := map[string]struct{}{"example.test": {}}

	assert.True(t, Contains(blocked, "team.example.test"))
	assert.False(t, Contains(blocked, "other.test"))
}
