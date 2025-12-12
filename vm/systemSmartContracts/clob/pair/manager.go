package pair

import (
	"encoding/binary"
	"fmt"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/storage"
)

// Config holds metadata about a trading pair.
type Config struct {
	ID            uint32
	BaseAssetID   uint32
	QuoteAssetID  uint32
	BaseDecimals  uint8
	QuoteDecimals uint8
	PriceDecimals uint8
}

// Manager handles pair configuration with caching.
type Manager struct {
	store       storage.StateStore
	pairs       map[uint32]*Config
	pairLookup  map[string]uint32 // canonical key -> pairID
	lastPairID  uint32
	pairsLoaded bool
}

// NewManager creates a new pair manager.
func NewManager(store storage.StateStore) *Manager {
	return &Manager{
		store:      store,
		pairs:      make(map[uint32]*Config),
		pairLookup: make(map[string]uint32),
	}
}

// LoadMeta loads the last pair ID from storage.
func (m *Manager) LoadMeta() {
	data := m.store.GetStorage([]byte(storage.KeyMetaLastPairID))
	if len(data) >= 4 {
		m.lastPairID = binary.LittleEndian.Uint32(data)
	}
	m.pairsLoaded = true
}

// GetConfig returns the configuration for a pair, loading from storage if needed.
func (m *Manager) GetConfig(pairID uint32) (*Config, error) {
	if cfg, ok := m.pairs[pairID]; ok {
		return cfg, nil
	}

	// Load from storage
	pairKey := make([]byte, len(storage.KeyPairPrefix)+4)
	copy(pairKey, storage.KeyPairPrefix)
	binary.BigEndian.PutUint32(pairKey[len(storage.KeyPairPrefix):], pairID)

	data := m.store.GetStorage(pairKey)
	if len(data) < 12 {
		return nil, fmt.Errorf("pair not found: %d", pairID)
	}

	cfg := &Config{
		ID:            pairID,
		BaseAssetID:   binary.LittleEndian.Uint32(data[1:]),
		QuoteAssetID:  binary.LittleEndian.Uint32(data[5:]),
		BaseDecimals:  data[9],
		QuoteDecimals: data[10],
		PriceDecimals: data[11],
	}

	m.pairs[pairID] = cfg
	m.pairLookup[CanonicalKey(cfg.BaseAssetID, cfg.QuoteAssetID)] = pairID

	return cfg, nil
}

// CanonicalKey returns the canonical key for a pair of assets.
// The lower asset ID always comes first.
func CanonicalKey(a, b uint32) string {
	if a < b {
		return fmt.Sprintf("%d_%d", a, b)
	}
	return fmt.Sprintf("%d_%d", b, a)
}

// ScaleFactor returns the scale factor for a pair based on its price decimals.
func ScaleFactor(cfg *Config) uint64 {
	if cfg == nil || cfg.PriceDecimals == 0 {
		return core.ScaleFactor
	}
	factor := uint64(1)
	for i := uint8(0); i < cfg.PriceDecimals; i++ {
		factor *= 10
	}
	return factor
}

// GetLastPairID returns the last assigned pair ID.
func (m *Manager) GetLastPairID() uint32 {
	if !m.pairsLoaded {
		m.LoadMeta()
	}
	return m.lastPairID
}

// IsPairsLoaded returns whether pairs metadata has been loaded.
func (m *Manager) IsPairsLoaded() bool {
	return m.pairsLoaded
}

// CreatePair creates a new trading pair and returns the new pair ID.
func (m *Manager) CreatePair(baseAssetID, quoteAssetID uint32, baseDecimals, quoteDecimals, priceDecimals uint8) (uint32, error) {
	if !m.pairsLoaded {
		m.LoadMeta()
	}

	if baseAssetID == quoteAssetID {
		return 0, fmt.Errorf("base and quote assets must be different")
	}

	// Check uniqueness
	canonKey := CanonicalKey(baseAssetID, quoteAssetID)
	if _, exists := m.pairLookup[canonKey]; exists {
		return 0, fmt.Errorf("pair already exists for assets %d and %d", baseAssetID, quoteAssetID)
	}

	// Generate new ID
	m.lastPairID++
	newID := m.lastPairID

	// Register in memory
	m.pairs[newID] = &Config{
		ID:            newID,
		BaseAssetID:   baseAssetID,
		QuoteAssetID:  quoteAssetID,
		BaseDecimals:  baseDecimals,
		QuoteDecimals: quoteDecimals,
		PriceDecimals: priceDecimals,
	}
	m.pairLookup[canonKey] = newID

	// Save lastPairID
	metaBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(metaBytes, m.lastPairID)
	m.store.SetStorage([]byte(storage.KeyMetaLastPairID), metaBytes)

	// Save pair config
	// Format: [version:1][baseAssetID:4][quoteAssetID:4][baseDecimals:1][quoteDecimals:1][priceDecimals:1]
	pairData := make([]byte, 12)
	pairData[0] = 2 // Version 2
	binary.LittleEndian.PutUint32(pairData[1:], baseAssetID)
	binary.LittleEndian.PutUint32(pairData[5:], quoteAssetID)
	pairData[9] = baseDecimals
	pairData[10] = quoteDecimals
	pairData[11] = priceDecimals

	pairKey := make([]byte, len(storage.KeyPairPrefix)+4)
	copy(pairKey, storage.KeyPairPrefix)
	binary.BigEndian.PutUint32(pairKey[len(storage.KeyPairPrefix):], newID)
	m.store.SetStorage(pairKey, pairData)

	// Save uniqueness index
	idxKey := []byte(storage.KeyPairIdxPrefix + canonKey)
	idBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(idBytes, newID)
	m.store.SetStorage(idxKey, idBytes)

	return newID, nil
}

// GetPairs returns all cached pairs (for iteration).
func (m *Manager) GetPairs() map[uint32]*Config {
	return m.pairs
}

// GetPairLookup returns the pair lookup map (for checking existence).
func (m *Manager) GetPairLookup() map[string]uint32 {
	return m.pairLookup
}
