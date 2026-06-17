package lexorank

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

const (
	// alphabet is the sortable digit set used by this LexoRank variant.
	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// length keeps ranks fixed-width so lexical and numeric ordering match.
	length = 12
)

var (
	// base is the numeric radix derived from alphabet.
	base = big.NewInt(int64(len(alphabet)))
	// maxRank is the largest representable fixed-width rank value.
	maxRank = maxValue()

	// ErrNoSpace reports that two adjacent ranks need a rebalance.
	ErrNoSpace = errors.New("no lexorank space between ranks")
)

// Initial returns the middle rank for an empty ordered set.
func Initial() string {
	rank, _ := Between("", "")
	return rank
}

// Between returns a rank that sorts strictly after prev and before next.
// Empty prev and next values mean the beginning or end of the rank space.
func Between(prev, next string) (string, error) {
	low := big.NewInt(0)
	high := new(big.Int).Set(maxRank)

	if strings.TrimSpace(prev) != "" {
		value, err := parse(prev)
		if err != nil {
			return "", err
		}
		low = value
	}
	if strings.TrimSpace(next) != "" {
		value, err := parse(next)
		if err != nil {
			return "", err
		}
		high = value
	}
	if low.Cmp(high) >= 0 {
		return "", fmt.Errorf("prev rank must sort before next rank")
	}

	gap := new(big.Int).Sub(high, low)
	if gap.Cmp(big.NewInt(1)) <= 0 {
		return "", ErrNoSpace
	}
	mid := new(big.Int).Add(low, high)
	mid.Div(mid, big.NewInt(2))
	return format(mid), nil
}

// EvenlySpaced returns count ranks distributed across the rank space.
func EvenlySpaced(count int) []string {
	if count <= 0 {
		return nil
	}
	step := new(big.Int).Div(new(big.Int).Set(maxRank), big.NewInt(int64(count+1)))
	if step.Sign() == 0 {
		step.SetInt64(1)
	}
	ranks := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		value := new(big.Int).Mul(step, big.NewInt(int64(i)))
		ranks = append(ranks, format(value))
	}
	return ranks
}

// parse converts a fixed-width rank into its numeric value.
func parse(rank string) (*big.Int, error) {
	rank = strings.ToUpper(strings.TrimSpace(rank))
	if len(rank) != length {
		return nil, fmt.Errorf("rank must be %d characters", length)
	}
	value := big.NewInt(0)
	for _, char := range rank {
		digit := strings.IndexRune(alphabet, char)
		if digit < 0 {
			return nil, fmt.Errorf("rank contains invalid character %q", char)
		}
		value.Mul(value, base)
		value.Add(value, big.NewInt(int64(digit)))
	}
	return value, nil
}

// format converts a numeric value into a fixed-width rank.
func format(value *big.Int) string {
	if value.Sign() < 0 {
		value = big.NewInt(0)
	}
	if value.Cmp(maxRank) > 0 {
		value = new(big.Int).Set(maxRank)
	}

	current := new(big.Int).Set(value)
	digits := make([]byte, length)
	mod := new(big.Int)
	for i := length - 1; i >= 0; i-- {
		current.DivMod(current, base, mod)
		digits[i] = alphabet[mod.Int64()]
	}
	return string(digits)
}

// maxValue returns the largest numeric value encodable by the fixed rank length.
func maxValue() *big.Int {
	value := big.NewInt(1)
	for i := 0; i < length; i++ {
		value.Mul(value, base)
	}
	value.Sub(value, big.NewInt(1))
	return value
}
