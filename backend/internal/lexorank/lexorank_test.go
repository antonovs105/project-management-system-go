package lexorank

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBetweenCreatesSortableRank(t *testing.T) {
	first := Initial()
	second, err := Between(first, "")
	require.NoError(t, err)
	middle, err := Between(first, second)
	require.NoError(t, err)

	ranks := []string{second, middle, first}
	sort.Strings(ranks)

	assert.Equal(t, []string{first, middle, second}, ranks)
}

func TestEvenlySpacedRanksSortInReturnedOrder(t *testing.T) {
	ranks := EvenlySpaced(4)
	require.Len(t, ranks, 4)

	sorted := append([]string(nil), ranks...)
	sort.Strings(sorted)

	assert.Equal(t, ranks, sorted)
}

func TestBetweenRejectsInvalidBoundaries(t *testing.T) {
	_, err := Between("ZZZZZZZZZZZZ", "000000000001")

	assert.Error(t, err)
}
