package apperror

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPreservesMessageAndSupportsCategoryMatching(t *testing.T) {
	err := New(ErrForbidden, "insufficient permissions")

	require.EqualError(t, err, "insufficient permissions")
	require.ErrorIs(t, err, ErrForbidden)
	require.False(t, errors.Is(err, ErrNotFound))
}
