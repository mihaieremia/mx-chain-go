package core

import "math"

// SafeMul multiplies two uint64 values and returns an error if overflow occurs.
// Uses the standard overflow check: a*b overflows if a > MaxUint64/b
func SafeMul(a, b uint64) (uint64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxUint64/b {
		return 0, ErrOverflow
	}
	return a * b, nil
}

// SafeAdd adds two uint64 values and returns an error if overflow occurs.
func SafeAdd(a, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// SafeSub subtracts b from a and returns an error if underflow occurs.
func SafeSub(a, b uint64) (uint64, error) {
	if b > a {
		return 0, ErrOverflow
	}
	return a - b, nil
}

// SafeDiv divides a by b and returns an error if b is zero.
func SafeDiv(a, b uint64) (uint64, error) {
	if b == 0 {
		return 0, ErrOverflow
	}
	return a / b, nil
}

// SafeMulDiv computes (a * b) / c with overflow protection.
// This is the most common pattern in CLOB: quantity * price / scaleFactor
//
// Algorithm:
// 1. Check if a*b would overflow
// 2. If no overflow, compute directly
// 3. If overflow possible, use uint128 arithmetic via decomposition
//
// For the overflow case, we use the identity:
// (a * b) / c = (a / c) * b + (a % c) * b / c
// This avoids overflow when a/c is small enough.
func SafeMulDiv(a, b, c uint64) (uint64, error) {
	if c == 0 {
		return 0, ErrOverflow
	}
	if a == 0 || b == 0 {
		return 0, nil
	}

	// Fast path: check if a*b fits in uint64
	if a <= math.MaxUint64/b {
		return (a * b) / c, nil
	}

	// Slow path: use 128-bit arithmetic via decomposition
	// (a * b) / c = (a / c) * b + ((a % c) * b) / c
	quotient := a / c
	remainder := a % c

	// Check if quotient * b overflows
	term1, err := SafeMul(quotient, b)
	if err != nil {
		return 0, err
	}

	// remainder < c, so remainder * b might still overflow, check it
	if remainder > math.MaxUint64/b {
		// Use recursive decomposition on the remainder term
		// ((a % c) * b) / c where (a % c) < c
		// Since remainder < c, we can try: (remainder * b) / c
		// But this still overflows. Use: remainder * (b / c) + remainder * (b % c) / c
		bQuot := b / c
		bRem := b % c

		// remainder * bQuot
		t1, err := SafeMul(remainder, bQuot)
		if err != nil {
			return 0, err
		}

		// remainder * bRem / c - both remainder and bRem are < c, so their product < c^2
		// If c^2 fits in uint64, we're safe. Otherwise need full 128-bit.
		if remainder > math.MaxUint64/bRem {
			// Need true 128-bit: compute using mul128
			hi, lo := mul128(remainder, bRem)
			t2 := div128by64(hi, lo, c)
			result, err := SafeAdd(term1, t1)
			if err != nil {
				return 0, err
			}
			return SafeAdd(result, t2)
		}

		t2 := (remainder * bRem) / c
		result, err := SafeAdd(term1, t1)
		if err != nil {
			return 0, err
		}
		return SafeAdd(result, t2)
	}

	term2 := (remainder * b) / c
	return SafeAdd(term1, term2)
}

// mul128 multiplies two 64-bit integers and returns a 128-bit result as (hi, lo)
func mul128(a, b uint64) (hi, lo uint64) {
	const mask32 = (1 << 32) - 1
	a0 := a & mask32
	a1 := a >> 32
	b0 := b & mask32
	b1 := b >> 32

	// a*b = (a1*2^32 + a0) * (b1*2^32 + b0)
	//     = a1*b1*2^64 + (a1*b0 + a0*b1)*2^32 + a0*b0
	p0 := a0 * b0
	p1 := a1 * b0
	p2 := a0 * b1
	p3 := a1 * b1

	// Combine with carry handling
	lo = p0
	mid := p1 + p2
	carry := uint64(0)
	if mid < p1 {
		carry = 1 << 32 // overflow in mid
	}

	lo += mid << 32
	if lo < mid<<32 {
		carry++
	}

	hi = p3 + (mid >> 32) + carry
	return
}

// div128by64 divides a 128-bit number (hi, lo) by a 64-bit divisor d
// Returns the 64-bit quotient (assumes result fits in 64 bits)
func div128by64(hi, lo, d uint64) uint64 {
	if hi == 0 {
		return lo / d
	}

	// Use long division algorithm
	// This is a simplified version that works when hi < d
	if hi >= d {
		// Result would overflow uint64, return max as approximation
		// In practice, this shouldn't happen in our CLOB use case
		return math.MaxUint64
	}

	// Binary long division
	var quotient uint64
	remainder := hi

	for i := 63; i >= 0; i-- {
		// Shift in next bit from lo
		remainder = (remainder << 1) | ((lo >> i) & 1)
		if remainder >= d {
			remainder -= d
			quotient |= 1 << i
		}
	}

	return quotient
}

// ValidateNotional checks if quantity * price would exceed max notional value
// without performing the multiplication (avoids overflow)
func ValidateNotional(quantity, price, maxNotional uint64) error {
	if price == 0 {
		return nil // Zero price means zero notional
	}
	if quantity > maxNotional/price {
		return ErrOverflow
	}
	return nil
}
