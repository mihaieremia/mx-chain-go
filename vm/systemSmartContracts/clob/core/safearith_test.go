package core

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeMul(t *testing.T) {
	tests := []struct {
		name    string
		a, b    uint64
		want    uint64
		wantErr bool
	}{
		{"zero_a", 0, 100, 0, false},
		{"zero_b", 100, 0, 0, false},
		{"both_zero", 0, 0, 0, false},
		{"simple", 10, 20, 200, false},
		{"large_safe", 1000000, 1000000, 1000000000000, false},
		{"max_safe", math.MaxUint64 / 2, 2, math.MaxUint64 - 1, false},
		{"overflow", math.MaxUint64, 2, 0, true},
		{"overflow_large", 1 << 33, 1 << 33, 0, true},
		{"boundary", math.MaxUint64/2 + 1, 2, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeMul(tt.a, tt.b)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrOverflow, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSafeAdd(t *testing.T) {
	tests := []struct {
		name    string
		a, b    uint64
		want    uint64
		wantErr bool
	}{
		{"zero", 0, 0, 0, false},
		{"simple", 10, 20, 30, false},
		{"max_minus_one", math.MaxUint64 - 1, 1, math.MaxUint64, false},
		{"overflow", math.MaxUint64, 1, 0, true},
		{"overflow_large", math.MaxUint64 - 5, 10, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeAdd(tt.a, tt.b)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSafeSub(t *testing.T) {
	tests := []struct {
		name    string
		a, b    uint64
		want    uint64
		wantErr bool
	}{
		{"zero", 0, 0, 0, false},
		{"simple", 20, 10, 10, false},
		{"to_zero", 10, 10, 0, false},
		{"underflow", 5, 10, 0, true},
		{"underflow_zero", 0, 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeSub(tt.a, tt.b)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSafeDiv(t *testing.T) {
	tests := []struct {
		name    string
		a, b    uint64
		want    uint64
		wantErr bool
	}{
		{"simple", 100, 10, 10, false},
		{"zero_numerator", 0, 10, 0, false},
		{"div_by_zero", 100, 0, 0, true},
		{"truncation", 10, 3, 3, false},
		{"max_div_1", math.MaxUint64, 1, math.MaxUint64, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeDiv(tt.a, tt.b)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSafeMulDiv(t *testing.T) {
	tests := []struct {
		name    string
		a, b, c uint64
		want    uint64
		wantErr bool
	}{
		// Basic cases
		{"simple", 100, 50, 100, 50, false},
		{"zero_a", 0, 50, 100, 0, false},
		{"zero_b", 100, 0, 100, 0, false},
		{"div_by_zero", 100, 50, 0, 0, true},

		// CLOB realistic cases: quantity * price / scaleFactor
		{"clob_basic", 1000, 500, 100, 5000, false},                         // 1000 * 500 / 100 = 5000
		{"clob_decimals", 1000000, 12345, 10000, 1234500, false},             // With 4 decimal places
		{"clob_large_qty", 1000000000, 100, 100, 1000000000, false},          // 1B qty, price 1.00
		{"clob_large_price", 1000, 10000000000, 100, 100000000000, false},    // Large price

		// Edge cases that require 128-bit arithmetic
		{"needs_128bit", 1 << 40, 1 << 30, 1 << 20, 1 << 50, false},
		{"large_values", math.MaxUint64 / 2, 2, 2, math.MaxUint64 / 2, false},

		// Truncation cases
		{"truncation", 10, 3, 10, 3, false}, // 10 * 3 / 10 = 3

		// Cases that should overflow even with 128-bit
		{"result_overflow", math.MaxUint64, math.MaxUint64, 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeMulDiv(tt.a, tt.b, tt.c)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got, "SafeMulDiv(%d, %d, %d)", tt.a, tt.b, tt.c)
			}
		})
	}
}

func TestSafeMulDiv_CLOBScenarios(t *testing.T) {
	// Simulate real CLOB scenarios
	scaleFactor := uint64(100) // 2 decimal places

	t.Run("buy_order_lock_amount", func(t *testing.T) {
		// Buy 1000 tokens at price 50.00 (5000 in scaled)
		// Lock amount = 1000 * 5000 / 100 = 50000 quote
		quantity := uint64(1000)
		price := uint64(5000)
		expected := uint64(50000)

		result, err := SafeMulDiv(quantity, price, scaleFactor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("large_whale_order", func(t *testing.T) {
		// Buy 1,000,000,000 tokens at price 100.00 (10000 scaled)
		// Lock = 1B * 10000 / 100 = 100B quote
		quantity := uint64(1_000_000_000)
		price := uint64(10000)
		expected := uint64(100_000_000_000)

		result, err := SafeMulDiv(quantity, price, scaleFactor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("overflow_protection", func(t *testing.T) {
		// Attempt to buy max uint64 tokens at price max/2
		// This should safely return overflow error
		quantity := uint64(math.MaxUint64)
		price := uint64(math.MaxUint64 / 2)

		_, err := SafeMulDiv(quantity, price, scaleFactor)
		assert.Error(t, err)
	})
}

func TestMul128(t *testing.T) {
	tests := []struct {
		name     string
		a, b     uint64
		wantHi   uint64
		wantLo   uint64
	}{
		{"simple", 2, 3, 0, 6},
		{"large", 1 << 32, 1 << 32, 1, 0},
		{"max_times_2", math.MaxUint64, 2, 1, math.MaxUint64 - 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hi, lo := mul128(tt.a, tt.b)
			assert.Equal(t, tt.wantHi, hi, "hi mismatch")
			assert.Equal(t, tt.wantLo, lo, "lo mismatch")
		})
	}
}

func TestValidateNotional(t *testing.T) {
	tests := []struct {
		name        string
		qty, price  uint64
		maxNotional uint64
		wantErr     bool
	}{
		{"within_limit", 100, 50, 10000, false},
		{"at_limit", 100, 100, 10000, false},
		{"exceeds", 100, 101, 10000, true},
		{"zero_price", 100, 0, 10000, false},
		{"large_qty_small_price", 1000000, 1, 10000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotional(tt.qty, tt.price, tt.maxNotional)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmark tests
func BenchmarkSafeMulDiv_FastPath(b *testing.B) {
	// Values that don't overflow during multiplication
	a := uint64(1000000)
	c := uint64(100)
	d := uint64(100)

	for i := 0; i < b.N; i++ {
		_, _ = SafeMulDiv(a, c, d)
	}
}

func BenchmarkSafeMulDiv_SlowPath(b *testing.B) {
	// Values that require 128-bit arithmetic
	a := uint64(1 << 40)
	c := uint64(1 << 30)
	d := uint64(1 << 20)

	for i := 0; i < b.N; i++ {
		_, _ = SafeMulDiv(a, c, d)
	}
}

func BenchmarkUnsafeMulDiv(b *testing.B) {
	// Baseline: unsafe multiplication (may overflow silently)
	a := uint64(1000000)
	c := uint64(100)
	d := uint64(100)

	for i := 0; i < b.N; i++ {
		_ = (a * c) / d
	}
}
