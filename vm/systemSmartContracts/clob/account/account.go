package account

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// AssetBalance represents available and locked balance for a single asset.
type AssetBalance struct {
	Available uint64 // Ready for new orders or withdraw
	Locked    uint64 // Funds in open orders
}

// Lock moves amount from available to locked. Returns error if insufficient.
func (ab *AssetBalance) Lock(amount uint64) error {
	if ab.Available < amount {
		return core.ErrInsufficientBalance
	}
	ab.Available -= amount
	ab.Locked += amount
	return nil
}

// Unlock moves amount from locked to available. Returns error if insufficient.
func (ab *AssetBalance) Unlock(amount uint64) error {
	if ab.Locked < amount {
		return core.ErrInsufficientLocked
	}
	ab.Locked -= amount
	ab.Available += amount
	return nil
}

// DeductLocked deducts from locked balance (e.g., when order fills).
func (ab *AssetBalance) DeductLocked(amount uint64) error {
	if ab.Locked < amount {
		return core.ErrInsufficientLocked
	}
	ab.Locked -= amount
	return nil
}

// Credit adds to available balance.
func (ab *AssetBalance) Credit(amount uint64) {
	ab.Available += amount
}

// ============================================================================
// Validation Methods (Check-Only, No State Mutation)
// These methods verify preconditions without modifying state, enabling
// two-phase commit patterns: validate ALL first, then mutate ALL.
// ============================================================================

// ValidateUnlock returns an error if Unlock(amount) would fail, without mutating state.
func (ab *AssetBalance) ValidateUnlock(amount uint64) error {
	if ab.Locked < amount {
		return core.ErrInsufficientLocked
	}
	return nil
}

// ValidateDeductLocked returns an error if DeductLocked(amount) would fail, without mutating state.
func (ab *AssetBalance) ValidateDeductLocked(amount uint64) error {
	if ab.Locked < amount {
		return core.ErrInsufficientLocked
	}
	return nil
}

// ValidateDeductAvailable returns an error if direct Available deduction would fail, without mutating state.
func (ab *AssetBalance) ValidateDeductAvailable(amount uint64) error {
	if ab.Available < amount {
		return core.ErrInsufficientBalance
	}
	return nil
}

// ============================================================================
// Storage Keys
// ============================================================================

// Storage key format constants (split keys only; legacy combined key removed)
const (
	keyPrefixAcc   = "acc/"    // acc/{UID}/...
	keySuffixAvail = "/avail/" // acc/{UID}/avail/{assetID}
	keySuffixLock  = "/lock/"  // acc/{UID}/lock/{assetID}
)

// AvailableKey returns the storage key for available balance only.
// Format: acc/{UID}/avail/{assetID}
func AvailableKey(uid uint64, assetID uint32) string {
	return fmt.Sprintf("%s%d%s%d", keyPrefixAcc, uid, keySuffixAvail, assetID)
}

// LockedKey returns the storage key for locked balance only.
// Format: acc/{UID}/lock/{assetID}
func LockedKey(uid uint64, assetID uint32) string {
	return fmt.Sprintf("%s%d%s%d", keyPrefixAcc, uid, keySuffixLock, assetID)
}

// AddressIndexKey returns the storage key for address-to-UID lookup.
// Format: addr_idx/{address_hex}
func AddressIndexKey(address []byte) string {
	return "addr_idx/" + hex.EncodeToString(address)
}

// ============================================================================
// Serialization
// ============================================================================

// MarshalUint64 serializes a single uint64 (8 bytes).
func MarshalUint64(v uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	return buf
}

// UnmarshalUint64 deserializes a single uint64.
func UnmarshalUint64(data []byte) (uint64, error) {
	if len(data) < 8 {
		if len(data) == 0 {
			return 0, nil // Empty = 0
		}
		return 0, fmt.Errorf("invalid uint64 data: too short (need 8, got %d)", len(data))
	}
	return binary.LittleEndian.Uint64(data), nil
}
