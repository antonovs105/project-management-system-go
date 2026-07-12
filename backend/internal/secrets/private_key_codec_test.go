package secrets

import (
	"encoding/base64"
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
	require.True(t, strings.HasPrefix(encrypted, encryptedPrivateKeyV2Prefix))
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
	require.Contains(t, err.Error(), "at least 32 characters")
}

func TestPrivateKeyCodecKeepsPreviousKeysForRotation(t *testing.T) {
	oldSecret := strings.Repeat("o", 32)
	newSecret := strings.Repeat("n", 32)
	oldCodec, err := NewPrivateKeyCodec(oldSecret)
	require.NoError(t, err)

	oldCiphertext, err := oldCodec.EncryptPrivateKey("private-key")
	require.NoError(t, err)

	rotatedCodec, err := NewPrivateKeyCodec(newSecret, oldSecret)
	require.NoError(t, err)
	decrypted, err := rotatedCodec.DecryptPrivateKey(oldCiphertext)
	require.NoError(t, err)
	require.Equal(t, "private-key", decrypted)

	newCiphertext, err := rotatedCodec.EncryptPrivateKey("new-private-key")
	require.NoError(t, err)
	_, err = oldCodec.DecryptPrivateKey(newCiphertext)
	require.Error(t, err)
}

func TestPrivateKeyCodecDecryptsLegacyV1WithPreviousKey(t *testing.T) {
	oldSecret := strings.Repeat("o", 32)
	newSecret := strings.Repeat("n", 32)
	oldValue, err := NewPrivateKeyCodec(oldSecret)
	require.NoError(t, err)
	oldCodec := oldValue.(AESGCMPrivateKeyCodec)
	gcm, err := oldCodec.gcm(oldCodec.keys[oldCodec.primaryKeyID])
	require.NoError(t, err)
	nonce := make([]byte, gcm.NonceSize())
	legacy := encryptedPrivateKeyV1Prefix + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte("legacy"), nil))

	rotated, err := NewPrivateKeyCodec(newSecret, oldSecret)
	require.NoError(t, err)
	plaintext, err := rotated.DecryptPrivateKey(legacy)
	require.NoError(t, err)
	require.Equal(t, "legacy", plaintext)
}
