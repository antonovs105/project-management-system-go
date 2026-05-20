package c2s

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasType(t *testing.T) {
	assert.True(t, hasType("Create", "Create"))
	assert.True(t, hasType([]any{"Activity", "forge:Ticket"}, "forge:Ticket"))
	assert.False(t, hasType([]any{"Update"}, "Create"))
}

func TestFirstString(t *testing.T) {
	assert.Equal(t, "https://example.test/projects/1", firstString(" https://example.test/projects/1 "))
	assert.Equal(t, "https://example.test/users/alice", firstString([]any{
		"https://example.test/users/alice",
		"https://example.test/users/bob",
	}))
	assert.Empty(t, firstString([]any{42, ""}))
}

func TestDecodeObject(t *testing.T) {
	object, err := decodeObject(json.RawMessage(`{"type":"Note","content":"Ready"}`))

	require.NoError(t, err)
	assert.True(t, hasType(object["type"], "Note"))
	assert.Equal(t, "Ready", object.optionalString("content"))
}

func TestDecodeObjectRejectsNonObject(t *testing.T) {
	_, err := decodeObject(json.RawMessage(`"https://example.test/notes/1"`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object")
}
