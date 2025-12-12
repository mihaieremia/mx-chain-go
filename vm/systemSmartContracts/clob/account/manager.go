package account

import (
	"encoding/binary"
	"fmt"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/storage"
)

// Manager handles account and balance operations with storage.
type Manager struct {
	store storage.StateStore
}

// NewManager creates a new account manager.
func NewManager(store storage.StateStore) *Manager {
	return &Manager{store: store}
}

// LoadBalance loads a balance from storage.
func (m *Manager) LoadBalance(uid uint64, assetID uint32) *AssetBalance {
	availData := m.store.GetStorage([]byte(AvailableKey(uid, assetID)))
	lockedData := m.store.GetStorage([]byte(LockedKey(uid, assetID)))

	available, _ := UnmarshalUint64(availData)
	locked, _ := UnmarshalUint64(lockedData)

	return &AssetBalance{
		Available: available,
		Locked:    locked,
	}
}

// SaveBalance saves a balance to storage.
func (m *Manager) SaveBalance(uid uint64, assetID uint32, bal *AssetBalance) {
	m.store.SetStorage([]byte(AvailableKey(uid, assetID)), MarshalUint64(bal.Available))
	m.store.SetStorage([]byte(LockedKey(uid, assetID)), MarshalUint64(bal.Locked))
}

// GetUIDByAddress looks up a UID by address.
// Returns 0 and error if not found.
func (m *Manager) GetUIDByAddress(address []byte) (uint64, error) {
	indexKey := AddressIndexKey(address)
	data := m.store.GetStorage([]byte(indexKey))
	if len(data) == 0 {
		return 0, fmt.Errorf("account not registered")
	}
	return binary.LittleEndian.Uint64(data), nil
}

// GetOrRegisterUID returns the UID for an address, auto-registering if needed.
func (m *Manager) GetOrRegisterUID(address []byte) (uint64, error) {
	uid, err := m.GetUIDByAddress(address)
	if err == nil {
		return uid, nil
	}

	// Auto-register
	metaData := m.store.GetStorage([]byte(storage.KeyAccountsMeta))
	var nextUID uint64 = 1
	if len(metaData) >= 8 {
		nextUID = binary.LittleEndian.Uint64(metaData)
	}
	uid = nextUID
	nextUID++

	metaBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(metaBytes, nextUID)
	m.store.SetStorage([]byte(storage.KeyAccountsMeta), metaBytes)

	uidBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(uidBytes, uid)
	m.store.SetStorage([]byte(AddressIndexKey(address)), uidBytes)

	return uid, nil
}

// Register registers a new account or returns existing UID.
// Returns the UID for the address.
func (m *Manager) Register(address []byte) uint64 {
	// Check if already registered
	indexKey := AddressIndexKey(address)
	existingUID := m.store.GetStorage([]byte(indexKey))
	if len(existingUID) > 0 {
		return binary.LittleEndian.Uint64(existingUID)
	}

	// Load and increment nextUID
	metaData := m.store.GetStorage([]byte(storage.KeyAccountsMeta))
	var nextUID uint64 = 1
	if len(metaData) >= 8 {
		nextUID = binary.LittleEndian.Uint64(metaData)
	}
	uid := nextUID
	nextUID++

	// Save updated nextUID
	metaBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(metaBytes, nextUID)
	m.store.SetStorage([]byte(storage.KeyAccountsMeta), metaBytes)

	// Save address index
	uidBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(uidBytes, uid)
	m.store.SetStorage([]byte(indexKey), uidBytes)

	return uid
}
