package secrets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrivateKeyCodecEncryptsAndDecrypts(t *testing.T) {
	codec, err := NewPrivateKeyCodec(strings.Repeat("k", 32))
	require.NoError(t, err)

	encrypted, err := codec.EncryptPrivateKey("private-key")
	require.NoError(t, err)
	require.NotEqual(t, "private-key", encrypted)
	require.True(t, strings.HasPrefix(encrypted, encryptedPrivateKeyPrefix))
	require.True(t, IsEncryptedPrivateKey(encrypted))

	decrypted, err := codec.DecryptPrivateKey(encrypted)
	require.NoError(t, err)
	require.Equal(t, "private-key", decrypted)
}

func TestPrivateKeyCodecAllowsLegacyPlaintext(t *testing.T) {
	codec, err := NewPrivateKeyCodec(strings.Repeat("k", 32))
	require.NoError(t, err)

	decrypted, err := codec.DecryptPrivateKey("legacy-private-key")
	require.NoError(t, err)
	require.Equal(t, "legacy-private-key", decrypted)
	require.False(t, IsEncryptedPrivateKey("legacy-private-key"))
}

func TestPrivateKeyCodecRejectsWeakConfiguredKey(t *testing.T) {
	_, err := NewPrivateKeyCodec("short")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ACTOR_PRIVATE_KEY_ENCRYPTION_KEY")
}
