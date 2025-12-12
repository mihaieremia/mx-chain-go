package trie

import (
	"fmt"
	"testing"

	"github.com/multiversx/mx-chain-go/testscommon/enableEpochsHandlerMock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommitDeterminism_SinglePassVsTwoPass verifies that the new single-pass
// commit produces IDENTICAL root hashes to the legacy two-pass approach.
// This is critical for blockchain consensus - all nodes must compute the same state root.
func TestCommitDeterminism_SinglePassVsTwoPass(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		numKeys   int
		keySize   int
		valueSize int
	}{
		{"small_trie_10_keys", 10, 32, 100},
		{"medium_trie_100_keys", 100, 32, 100},
		{"large_trie_1000_keys", 1000, 32, 100},
		{"varied_key_sizes", 500, 64, 256},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create two identical tries
			tr1, _ := newEmptyTrie()
			tr2, _ := newEmptyTrie()

			// Generate deterministic test data
			keys, values := generateTestData(tc.numKeys, tc.keySize, tc.valueSize, 42)

			// Insert identical data into both tries
			for i := 0; i < tc.numKeys; i++ {
				err := tr1.Update(keys[i], values[i])
				require.NoError(t, err)
				err = tr2.Update(keys[i], values[i])
				require.NoError(t, err)
			}

			// Commit using new single-pass method
			err := tr1.Commit()
			require.NoError(t, err)

			// Commit using legacy two-pass method
			err = commitTwoPass(tr2)
			require.NoError(t, err)

			// Get root hashes
			rootHash1, err := tr1.RootHash()
			require.NoError(t, err)
			rootHash2, err := tr2.RootHash()
			require.NoError(t, err)

			// CRITICAL: Root hashes MUST be identical
			assert.Equal(t, rootHash1, rootHash2,
				"Single-pass and two-pass commits produced different root hashes! Blockchain consensus would break.")
		})
	}
}

// TestCommitDeterminism_MultipleCommits verifies determinism across multiple commit cycles
func TestCommitDeterminism_MultipleCommits(t *testing.T) {
	t.Parallel()

	tr1, _ := newEmptyTrie()
	tr2, _ := newEmptyTrie()

	// Perform multiple rounds of updates and commits
	for round := 0; round < 5; round++ {
		keys, values := generateTestData(100, 32, 100, int64(round))

		for i := 0; i < len(keys); i++ {
			_ = tr1.Update(keys[i], values[i])
			_ = tr2.Update(keys[i], values[i])
		}

		err := tr1.Commit()
		require.NoError(t, err)
		err = commitTwoPass(tr2)
		require.NoError(t, err)

		rootHash1, _ := tr1.RootHash()
		rootHash2, _ := tr2.RootHash()

		assert.Equal(t, rootHash1, rootHash2,
			"Round %d: Root hashes diverged after commit", round)
	}
}

// TestCommitDeterminism_WithDeletes verifies determinism when mixing inserts and deletes
func TestCommitDeterminism_WithDeletes(t *testing.T) {
	t.Parallel()

	tr1, _ := newEmptyTrie()
	tr2, _ := newEmptyTrie()

	// Insert data
	keys, values := generateTestData(200, 32, 100, 123)
	for i := 0; i < len(keys); i++ {
		_ = tr1.Update(keys[i], values[i])
		_ = tr2.Update(keys[i], values[i])
	}

	// Commit initial state
	_ = tr1.Commit()
	_ = commitTwoPass(tr2)

	// Delete half the keys
	for i := 0; i < len(keys)/2; i++ {
		_ = tr1.Delete(keys[i])
		_ = tr2.Delete(keys[i])
	}

	// Commit after deletes
	err := tr1.Commit()
	require.NoError(t, err)
	err = commitTwoPass(tr2)
	require.NoError(t, err)

	rootHash1, _ := tr1.RootHash()
	rootHash2, _ := tr2.RootHash()

	assert.Equal(t, rootHash1, rootHash2,
		"Root hashes diverged after delete operations")
}

// BenchmarkCommit_SinglePass benchmarks the new optimized single-pass commit
func BenchmarkCommit_SinglePass(b *testing.B) {
	benchmarkCommit(b, "single_pass", func(tr *patriciaMerkleTrie) error {
		return tr.Commit()
	})
}

// BenchmarkCommit_TwoPass benchmarks the legacy two-pass commit
func BenchmarkCommit_TwoPass(b *testing.B) {
	benchmarkCommit(b, "two_pass", func(tr *patriciaMerkleTrie) error {
		return commitTwoPass(tr)
	})
}

func benchmarkCommit(b *testing.B, name string, commitFn func(*patriciaMerkleTrie) error) {
	sizes := []int{100, 500, 1000, 5000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%s_%d_keys", name, size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tr := createBenchmarkTrie(b)
				keys, values := generateTestData(size, 32, 100, int64(i))

				for j := 0; j < size; j++ {
					_ = tr.Update(keys[j], values[j])
				}
				b.StartTimer()

				err := commitFn(tr)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCommit_Comparison directly compares both methods side by side
func BenchmarkCommit_Comparison(b *testing.B) {
	sizes := []int{100, 1000, 5000}

	for _, size := range sizes {
		// Benchmark single-pass
		b.Run(fmt.Sprintf("SinglePass_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tr := createBenchmarkTrie(b)
				populateTrie(tr, size, int64(i))
				b.StartTimer()

				_ = tr.Commit()
			}
		})

		// Benchmark two-pass
		b.Run(fmt.Sprintf("TwoPass_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tr := createBenchmarkTrie(b)
				populateTrie(tr, size, int64(i))
				b.StartTimer()

				_ = commitTwoPass(tr)
			}
		})
	}
}

// commitTwoPass implements the legacy two-pass commit for comparison
func commitTwoPass(tr *patriciaMerkleTrie) error {
	tr.mutOperation.Lock()
	defer tr.mutOperation.Unlock()

	if tr.root == nil {
		return nil
	}
	if !tr.root.isDirty() {
		return nil
	}

	// Pass 1: Compute all hashes
	err := tr.root.setRootHash()
	if err != nil {
		return err
	}

	tr.oldRoot = make([]byte, 0)
	tr.oldHashes = make([][]byte, 0)

	// Pass 2: Write to storage
	err = tr.root.commitDirty(0, tr.maxTrieLevelInMemory, tr.trieStorage, tr.trieStorage)
	if err != nil {
		return err
	}

	return nil
}

// Helper functions

func createBenchmarkTrie(b *testing.B) *patriciaMerkleTrie {
	args := GetDefaultTrieStorageManagerParameters()
	trieStorage, err := NewTrieStorageManager(args)
	if err != nil {
		b.Fatal(err)
	}

	tr := &patriciaMerkleTrie{
		trieStorage:          trieStorage,
		marshalizer:          args.Marshalizer,
		hasher:               args.Hasher,
		oldHashes:            make([][]byte, 0),
		oldRoot:              make([]byte, 0),
		maxTrieLevelInMemory: 5,
		chanClose:            make(chan struct{}),
		enableEpochsHandler:  &enableEpochsHandlerMock.EnableEpochsHandlerStub{},
	}

	return tr
}

func generateTestData(numKeys, keySize, valueSize int, seed int64) ([][]byte, [][]byte) {
	keys := make([][]byte, numKeys)
	values := make([][]byte, numKeys)

	// Use deterministic "random" data based on seed
	for i := 0; i < numKeys; i++ {
		keys[i] = make([]byte, keySize)
		values[i] = make([]byte, valueSize)

		// Simple deterministic generation
		for j := 0; j < keySize; j++ {
			keys[i][j] = byte((seed + int64(i) + int64(j)) % 256)
		}
		for j := 0; j < valueSize; j++ {
			values[i][j] = byte((seed + int64(i) + int64(j) + 128) % 256)
		}
	}

	return keys, values
}

func populateTrie(tr *patriciaMerkleTrie, size int, seed int64) {
	keys, values := generateTestData(size, 32, 100, seed)
	for i := 0; i < size; i++ {
		_ = tr.Update(keys[i], values[i])
	}
}
