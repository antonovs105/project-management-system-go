package attachment

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLocalStoreRoundTripAndTraversalDefense(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	require.NoError(t, err)
	key := uuid.NewString()
	size, checksum, err := store.Put(context.Background(), key, strings.NewReader("hello attachment"))
	require.NoError(t, err)
	require.EqualValues(t, 16, size)
	require.Len(t, checksum, 64)
	reader, err := store.Open(context.Background(), key)
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "hello attachment", string(content))
	objects, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.Equal(t, key, objects[0].Key)
	require.NoError(t, store.Delete(context.Background(), key))
	_, err = store.Open(context.Background(), "../escape")
	require.Error(t, err)
}

func TestValidID(t *testing.T) {
	require.True(t, validID(uuid.NewString()))
	require.False(t, validID("not-a-uuid"))
}

func TestAttachmentFilenameAndContentValidation(t *testing.T) {
	require.Equal(t, "evidence.png", cleanFilename(`..\private\evidence.png`))
	require.Empty(t, cleanFilename(".."))
	require.True(t, allowedContentType("application/pdf"))
	require.True(t, allowedContentType("image/png"))
	require.False(t, allowedContentType("image/svg+xml"))
	require.False(t, allowedContentType("application/x-msdownload"))
}

func TestLocalStoreEnforcesSizeLimit(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	require.NoError(t, err)
	_, _, err = store.Put(context.Background(), uuid.NewString(), io.LimitReader(&repeatingReader{}, MaxSizeBytes+1))
	require.ErrorIs(t, err, ErrInvalidFile)
}

// repeatingReader produces deterministic non-empty bytes without allocating the limit.
type repeatingReader struct{}

// Read fills every requested buffer.
func (*repeatingReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}
