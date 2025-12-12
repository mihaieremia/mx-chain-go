package factory

import (
	"github.com/multiversx/mx-chain-go/config"
	"github.com/multiversx/mx-chain-go/storage"
	"github.com/multiversx/mx-chain-go/storage/database"
	"github.com/multiversx/mx-chain-go/storage/storageunit"
	"github.com/multiversx/mx-chain-storage-go/factory"
)

const minNumShards = 2

// persisterCreator is the factory which will handle creating new persisters
type persisterCreator struct {
	conf config.DBConfig
}

func newPersisterCreator(config config.DBConfig) *persisterCreator {
	return &persisterCreator{
		conf: config,
	}
}

// Create will create the persister for the provided path
func (pc *persisterCreator) Create(path string) (storage.Persister, error) {
	if len(path) == 0 {
		return nil, storage.ErrInvalidFilePath
	}

	if pc.conf.NumShards < minNumShards {
		return pc.CreateBasePersister(path)
	}

	shardIDProvider, err := pc.createShardIDProvider()
	if err != nil {
		return nil, err
	}
	return database.NewShardedPersister(path, pc, shardIDProvider)
}

// CreateBasePersister will create base the persister for the provided path
func (pc *persisterCreator) CreateBasePersister(path string) (storage.Persister, error) {
	var dbType = storageunit.DBType(pc.conf.Type)

	// Calculate effective delay in seconds for storage-go library
	// BatchDelayMilliseconds takes precedence if set
	effectiveDelaySeconds := pc.conf.BatchDelaySeconds
	if pc.conf.BatchDelayMilliseconds > 0 {
		// Convert milliseconds to seconds, minimum 1 (library requires > 0)
		effectiveDelaySeconds = pc.conf.BatchDelayMilliseconds / 1000
		if effectiveDelaySeconds < 1 {
			effectiveDelaySeconds = 1
		}
	}

	argsDB := factory.ArgDB{
		DBType:            dbType,
		Path:              path,
		BatchDelaySeconds: effectiveDelaySeconds,
		MaxBatchSize:      pc.conf.MaxBatchSize,
		MaxOpenFiles:      pc.conf.MaxOpenFiles,
	}

	return storageunit.NewDB(argsDB)
}

func (pc *persisterCreator) createShardIDProvider() (storage.ShardIDProvider, error) {
	switch storageunit.ShardIDProviderType(pc.conf.ShardIDProviderType) {
	case storageunit.BinarySplit:
		return database.NewShardIDProvider(pc.conf.NumShards)
	default:
		return nil, storage.ErrNotSupportedShardIDProviderType
	}
}

// IsInterfaceNil returns true if there is no value under the interface
func (pc *persisterCreator) IsInterfaceNil() bool {
	return pc == nil
}
