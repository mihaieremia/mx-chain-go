package integration

import (
	"encoding/binary"
	"math/big"
	"testing"

	vmcommon "github.com/multiversx/mx-chain-vm-common-go"
	"github.com/stretchr/testify/require"

	"github.com/multiversx/mx-chain-go/vm"
	"github.com/multiversx/mx-chain-go/vm/mock"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
)

// TestEnv provides a complete test environment for CLOB integration tests.
type TestEnv struct {
	t           *testing.T
	storageData map[string][]byte
	returnData  [][]byte
	eei         *mock.SystemEIStub
	sc          vm.SystemSmartContract
	SelfAddr    []byte
	OwnerAddr   []byte
	Addresses   map[string][]byte
	UIDs        map[string]uint64
}

// NewTestEnv creates a new test environment.
func NewTestEnv(t *testing.T) *TestEnv {
	selfAddr := make([]byte, 32)
	copy(selfAddr, "clob_contract")
	ownerAddr := make([]byte, 32)
	copy(ownerAddr, "clob_owner")

	storageData := make(map[string][]byte)
	var returnData [][]byte

	eei := &mock.SystemEIStub{
		GetStorageCalled: func(key []byte) []byte {
			return storageData[string(key)]
		},
		SetStorageCalled: func(key, value []byte) {
			if len(value) == 0 {
				delete(storageData, string(key))
			} else {
				storageData[string(key)] = value
			}
		},
		FinishCalled: func(value []byte) {
			returnData = append(returnData, value)
		},
		BlockChainHookCalled: func() vm.BlockchainHook {
			return &mock.BlockChainHookStub{
				CurrentNonceCalled: func() uint64 {
					return 1000000
				},
			}
		},
	}

	args := clob.ArgsNewCLOBSmartContract{
		Eei:       eei,
		GasCost:   vm.GasCost{},
		SelfAddr:  selfAddr,
		OwnerAddr: ownerAddr,
	}

	sc, err := clob.NewCLOBSmartContract(args)
	require.NoError(t, err)

	env := &TestEnv{
		t:           t,
		storageData: storageData,
		eei:         eei,
		sc:          sc,
		SelfAddr:    selfAddr,
		OwnerAddr:   ownerAddr,
		Addresses:   make(map[string][]byte),
		UIDs:        make(map[string]uint64),
	}

	// Capture returnData by reference in the closure
	env.eei.FinishCalled = func(value []byte) {
		env.returnData = append(env.returnData, value)
	}

	return env
}

// execute calls the SC and returns output
func (env *TestEnv) execute(function string, caller []byte, args [][]byte) (vmcommon.ReturnCode, [][]byte, string) {
	// Clear return data before each call
	env.returnData = nil
	env.eei.ReturnMessage = ""

	input := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  caller,
			Arguments:   args,
			CallValue:   big.NewInt(0),
			GasProvided: 100_000_000,
		},
		RecipientAddr: env.SelfAddr,
		Function:      function,
	}

	retCode := env.sc.Execute(input)
	return retCode, env.returnData, env.eei.ReturnMessage
}

// CreatePair creates a trading pair with the given configuration.
// Must be called by the owner address.
func (env *TestEnv) CreatePair(baseAssetID, quoteAssetID uint32, decimals ...uint8) uint32 {
	args := [][]byte{
		big.NewInt(int64(baseAssetID)).Bytes(),
		big.NewInt(int64(quoteAssetID)).Bytes(),
	}

	// Add decimal args if provided
	for _, d := range decimals {
		args = append(args, []byte{d})
	}

	retCode, returnData, msg := env.execute("createPair", env.OwnerAddr, args)
	require.Equal(env.t, vmcommon.Ok, retCode, msg)
	require.NotEmpty(env.t, returnData)

	// Parse returned ID (big-endian from Finish)
	pairID := binary.BigEndian.Uint32(returnData[0])
	return pairID
}

// RegisterUser creates a new user with the given name.
func (env *TestEnv) RegisterUser(name string) uint64 {
	addr := make([]byte, 32)
	copy(addr, name)
	env.Addresses[name] = addr

	retCode, returnData, msg := env.execute("register", addr, nil)
	require.Equal(env.t, vmcommon.Ok, retCode, msg)
	require.NotEmpty(env.t, returnData)

	uid := big.NewInt(0).SetBytes(returnData[0]).Uint64()
	env.UIDs[name] = uid
	return uid
}

// Deposit adds funds to a user's account.
func (env *TestEnv) Deposit(name string, assetID uint32, amount uint64) {
	addr := env.Addresses[name]
	require.NotNil(env.t, addr, "user %s not registered", name)

	args := [][]byte{
		big.NewInt(int64(assetID)).Bytes(),
		big.NewInt(int64(amount)).Bytes(),
	}

	retCode, _, msg := env.execute("deposit", addr, args)
	require.Equal(env.t, vmcommon.Ok, retCode, msg)
}

// Withdraw removes funds from a user's account.
func (env *TestEnv) Withdraw(name string, assetID uint32, amount uint64) error {
	addr := env.Addresses[name]
	if addr == nil {
		env.t.Fatalf("user %s not registered", name)
	}

	args := [][]byte{
		big.NewInt(int64(assetID)).Bytes(),
		big.NewInt(int64(amount)).Bytes(),
	}

	retCode, _, msg := env.execute("withdraw", addr, args)
	if retCode != vmcommon.Ok {
		return &VMError{Code: retCode, Message: msg}
	}
	return nil
}

// PlaceOrder places an order and returns the result.
func (env *TestEnv) PlaceOrder(name string, pairID uint32, side core.Side, orderType core.OrderType,
	quantity, price, stop uint64, tif core.TIF, oco uint64) (*PlaceOrderResult, error) {

	addr := env.Addresses[name]
	if addr == nil {
		env.t.Fatalf("user %s not registered", name)
	}

	args := [][]byte{
		big.NewInt(int64(pairID)).Bytes(),
		{byte(side)},
		orderTypeToBytes(orderType),
		big.NewInt(int64(quantity)).Bytes(),
		big.NewInt(int64(price)).Bytes(),
		big.NewInt(int64(stop)).Bytes(),
		tifToBytes(tif),
		big.NewInt(int64(oco)).Bytes(),
	}

	retCode, returnData, msg := env.execute("processOrder", addr, args)
	if retCode != vmcommon.Ok {
		return nil, &VMError{Code: retCode, Message: msg}
	}

	result := &PlaceOrderResult{
		OrderID:      big.NewInt(0).SetBytes(returnData[0]).Uint64(),
		FilledQty:    big.NewInt(0).SetBytes(returnData[1]).Uint64(),
		RemainingQty: big.NewInt(0).SetBytes(returnData[2]).Uint64(),
		Status:       returnData[3][0],
	}
	return result, nil
}

// CancelOrder cancels an order.
func (env *TestEnv) CancelOrder(name string, pairID uint32, orderID uint64) (*CancelOrderResult, error) {
	addr := env.Addresses[name]
	if addr == nil {
		env.t.Fatalf("user %s not registered", name)
	}

	args := [][]byte{
		big.NewInt(int64(pairID)).Bytes(),
		big.NewInt(int64(orderID)).Bytes(),
	}

	retCode, returnData, msg := env.execute("cancelOrder", addr, args)
	if retCode != vmcommon.Ok {
		return nil, &VMError{Code: retCode, Message: msg}
	}

	result := &CancelOrderResult{
		OrderID:     big.NewInt(0).SetBytes(returnData[0]).Uint64(),
		UID:         big.NewInt(0).SetBytes(returnData[1]).Uint64(),
		Side:        core.Side(returnData[2][0]),
		UnlockedQty: big.NewInt(0).SetBytes(returnData[3]).Uint64(),
		Price:       big.NewInt(0).SetBytes(returnData[4]).Uint64(),
	}
	return result, nil
}

// GetBalance returns a user's balance for an asset.
func (env *TestEnv) GetBalance(name string, assetID uint32) (available, locked uint64) {
	uid := env.UIDs[name]
	if uid == 0 {
		env.t.Fatalf("user %s not registered", name)
	}

	args := [][]byte{
		big.NewInt(int64(uid)).Bytes(),
		big.NewInt(int64(assetID)).Bytes(),
	}

	retCode, returnData, msg := env.execute("getBalance", env.SelfAddr, args)
	require.Equal(env.t, vmcommon.Ok, retCode, msg)

	available = big.NewInt(0).SetBytes(returnData[0]).Uint64()
	locked = big.NewInt(0).SetBytes(returnData[1]).Uint64()
	return
}

// GetOrder returns order view data for a resting order.
func (env *TestEnv) GetOrder(pairID uint32, orderID uint64) *OrderView {
	args := [][]byte{
		big.NewInt(int64(pairID)).Bytes(),
		big.NewInt(int64(orderID)).Bytes(),
	}

	retCode, returnData, msg := env.execute("getOrder", env.SelfAddr, args)
	require.Equal(env.t, vmcommon.Ok, retCode, msg)
	require.Len(env.t, returnData, 8)

	return &OrderView{
		ID:       big.NewInt(0).SetBytes(returnData[0]).Uint64(),
		UID:      big.NewInt(0).SetBytes(returnData[1]).Uint64(),
		Side:     core.Side(returnData[2][0]),
		Type:     core.OrderType(returnData[3][0]),
		Quantity: big.NewInt(0).SetBytes(returnData[4]).Uint64(),
		Price:    big.NewInt(0).SetBytes(returnData[5]).Uint64(),
		Stop:     big.NewInt(0).SetBytes(returnData[6]).Uint64(),
		TIF:      core.TIF(returnData[7][0]),
	}
}

// GetDepth returns the order book depth.
func (env *TestEnv) GetDepth(pairID uint32) (bids, asks [][2]uint64) {
	args := [][]byte{big.NewInt(int64(pairID)).Bytes()}

	retCode, returnData, msg := env.execute("getDepth", env.SelfAddr, args)
	require.Equal(env.t, vmcommon.Ok, retCode, msg)

	if len(returnData) == 0 {
		return nil, nil
	}

	idx := 0
	numBids := int(big.NewInt(0).SetBytes(returnData[idx]).Int64())
	idx++

	for i := 0; i < numBids; i++ {
		price := big.NewInt(0).SetBytes(returnData[idx]).Uint64()
		idx++
		qty := big.NewInt(0).SetBytes(returnData[idx]).Uint64()
		idx++
		bids = append(bids, [2]uint64{price, qty})
	}

	numAsks := int(big.NewInt(0).SetBytes(returnData[idx]).Int64())
	idx++

	for i := 0; i < numAsks; i++ {
		price := big.NewInt(0).SetBytes(returnData[idx]).Uint64()
		idx++
		qty := big.NewInt(0).SetBytes(returnData[idx]).Uint64()
		idx++
		asks = append(asks, [2]uint64{price, qty})
	}

	return bids, asks
}

// AssertBalance checks that a user's balance matches expected values.
func (env *TestEnv) AssertBalance(name string, assetID uint32, expectedAvailable, expectedLocked uint64) {
	available, locked := env.GetBalance(name, assetID)
	require.Equal(env.t, expectedAvailable, available, "user %s asset %d: available mismatch", name, assetID)
	require.Equal(env.t, expectedLocked, locked, "user %s asset %d: locked mismatch", name, assetID)
}

// AssertDepth checks that the order book depth matches expected values.
func (env *TestEnv) AssertDepth(pairID uint32, expectedBids, expectedAsks [][2]uint64) {
	bids, asks := env.GetDepth(pairID)
	require.Equal(env.t, expectedBids, bids, "bids mismatch")
	require.Equal(env.t, expectedAsks, asks, "asks mismatch")
}

// MatchOrders triggers stop activation and matching for a pair and returns the number of matches.
func (env *TestEnv) MatchOrders(pairID uint32) int {
	retCode, returnData, msg := env.execute("matchOrders", env.SelfAddr, [][]byte{big.NewInt(int64(pairID)).Bytes()})
	require.Equal(env.t, vmcommon.Ok, retCode, msg)

	return int(big.NewInt(0).SetBytes(returnData[0]).Int64())
}

// makeInput creates a ContractCallInput (kept for setConfig helper)
func (env *TestEnv) makeInput(function string, caller []byte, args [][]byte) *vmcommon.ContractCallInput {
	return &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  caller,
			Arguments:   args,
			CallValue:   big.NewInt(0),
			GasProvided: 100_000_000,
		},
		RecipientAddr: env.SelfAddr,
		Function:      function,
	}
}

// SetConfig calls the setConfig admin endpoint directly.
func (env *TestEnv) SetConfig(pairID uint32, args [][]byte) error {
	// Clear return data before call
	env.returnData = nil
	env.eei.ReturnMessage = ""

	input := env.makeInput("setConfig", env.OwnerAddr, args)
	retCode := env.sc.Execute(input)
	if retCode != vmcommon.Ok {
		return &VMError{Code: retCode, Message: env.eei.ReturnMessage}
	}
	return nil
}

// Result types

// PlaceOrderResult holds the result of placing an order.
type PlaceOrderResult struct {
	OrderID      uint64
	FilledQty    uint64
	RemainingQty uint64
	Status       byte // 0=placed, 1=fully filled, 2=partial+cancelled
}

// CancelOrderResult holds the result of cancelling an order.
type CancelOrderResult struct {
	OrderID     uint64
	UID         uint64
	Side        core.Side
	UnlockedQty uint64
	Price       uint64
}

// OrderView represents getOrder output.
type OrderView struct {
	ID       uint64
	UID      uint64
	Side     core.Side
	Type     core.OrderType
	Quantity uint64
	Price    uint64
	Stop     uint64
	TIF      core.TIF
}

// VMError represents a VM execution error.
type VMError struct {
	Code    vmcommon.ReturnCode
	Message string
}

func (e *VMError) Error() string {
	return e.Message
}

// Helper functions

func orderTypeToBytes(ot core.OrderType) []byte {
	switch ot {
	case core.TypeMarket:
		return []byte("MARKET")
	case core.TypeLimit:
		return []byte("LIMIT")
	case core.TypeStopLimit:
		return []byte("STOP-LIMIT")
	default:
		return []byte("LIMIT")
	}
}

func tifToBytes(tif core.TIF) []byte {
	switch tif {
	case core.TIFGoodTillCancel:
		return []byte("GTC")
	case core.TIFFillOrKill:
		return []byte("FOK")
	case core.TIFImmediateOrCancel:
		return []byte("IOC")
	case core.TIFPostOnly:
		return []byte("POST_ONLY")
	default:
		return []byte("GTC")
	}
}

// Uint64ToBytes converts uint64 to little-endian bytes.
func Uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

// Uint32ToBytes converts uint32 to little-endian bytes.
func Uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}
