package endpoints

import (
	"bytes"
	"encoding/binary"
	"math/big"

	vmcommon "github.com/multiversx/mx-chain-vm-common-go"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/storage"
)

// CreatePair handles the createPair admin endpoint.
func CreatePair(ctx *Context, caller []byte, args [][]byte) vmcommon.ReturnCode {
	if !bytes.Equal(caller, ctx.OwnerAddr) {
		return ctx.Error("createPair can only be called by owner")
	}

	if len(args) < 2 {
		return ctx.Error("createPair requires baseAssetID, quoteAssetID [, baseDecimals, quoteDecimals, priceDecimals]")
	}

	baseAssetID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())
	quoteAssetID := uint32(big.NewInt(0).SetBytes(args[1]).Uint64())

	// Parse decimals
	baseDecimals := uint8(2)
	quoteDecimals := uint8(2)
	priceDecimals := uint8(2)

	if len(args) > 2 && len(args[2]) > 0 {
		baseDecimals = args[2][0]
	}
	if len(args) > 3 && len(args[3]) > 0 {
		quoteDecimals = args[3][0]
	}
	if len(args) > 4 && len(args[4]) > 0 {
		priceDecimals = args[4][0]
	}

	// Create pair using pair manager
	newID, err := ctx.PairMgr.CreatePair(baseAssetID, quoteAssetID, baseDecimals, quoteDecimals, priceDecimals)
	if err != nil {
		return ctx.Error(err.Error())
	}

	idBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(idBytes, newID)
	ctx.Eei.Finish(idBytes)
	return vmcommon.Ok
}

// SetConfig handles the setConfig admin endpoint.
func SetConfig(ctx *Context, caller []byte, args [][]byte) vmcommon.ReturnCode {
	if !bytes.Equal(caller, ctx.OwnerAddr) {
		return ctx.Error("setConfig can only be called by owner")
	}

	if len(args) < 1 {
		return ctx.Error("setConfig requires at least pairID")
	}

	pairID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())
	_, err := ctx.PairMgr.GetConfig(pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	eng, loader, err := storage.LoadOrderBookLazy(ctx.Eei, pairID)
	if err != nil {
		return ctx.Error(err.Error())
	}

	// Ensure existing state is present before saving new config
	if err := loader.LoadStops(); err != nil {
		return ctx.Error(err.Error())
	}
	if err := loader.LoadAllLevels(); err != nil {
		return ctx.Error(err.Error())
	}

	cfg := *eng.GetConfig()

	readU64 := func(idx int) (uint64, bool) {
		if len(args) > idx && len(args[idx]) > 0 {
			return big.NewInt(0).SetBytes(args[idx]).Uint64(), true
		}
		return 0, false
	}

	if v, ok := readU64(1); ok {
		cfg.MaxOrders = int(v)
	}
	if v, ok := readU64(2); ok {
		cfg.MaxPriceLevelsPerSide = int(v)
	}
	if v, ok := readU64(3); ok {
		cfg.MinOrderQty = v
	}
	if v, ok := readU64(4); ok {
		cfg.MaxOrderQty = v
	}
	if v, ok := readU64(5); ok {
		cfg.MinPrice = v
	}
	if v, ok := readU64(6); ok {
		cfg.MaxPrice = v
	}
	if v, ok := readU64(7); ok {
		cfg.MinNotional = v
	}
	if v, ok := readU64(8); ok {
		cfg.QtyStep = v
	}
	if v, ok := readU64(9); ok {
		cfg.PriceTick = v
	}
	if v, ok := readU64(10); ok {
		cfg.MaxOrdersPerAccount = int(v)
	}

	eng.SetConfig(&cfg)
	storage.SaveOrderBook(ctx.Eei, pairID, eng)

	return vmcommon.Ok
}
