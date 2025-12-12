package endpoints

import (
	"fmt"
	"math/big"

	vmcommon "github.com/multiversx/mx-chain-vm-common-go"
)

// Register handles user registration endpoint.
func Register(ctx *Context, caller []byte) vmcommon.ReturnCode {
	uid := ctx.AccountMgr.Register(caller)
	ctx.Eei.Finish(big.NewInt(int64(uid)).Bytes())
	return vmcommon.Ok
}

// Deposit handles deposit endpoint.
func Deposit(ctx *Context, caller []byte, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 2 {
		return ctx.Error("deposit requires assetID, amount")
	}

	// Get or auto-register account
	uid, err := ctx.AccountMgr.GetOrRegisterUID(caller)
	if err != nil {
		return ctx.Error(err.Error())
	}

	assetID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())
	amount := big.NewInt(0).SetBytes(args[1]).Uint64()

	// Load balance, credit, save
	bal := ctx.AccountMgr.LoadBalance(uid, assetID)
	bal.Credit(amount)
	ctx.AccountMgr.SaveBalance(uid, assetID, bal)

	return vmcommon.Ok
}

// Withdraw handles withdraw endpoint.
func Withdraw(ctx *Context, caller []byte, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 2 {
		return ctx.Error("withdraw requires assetID, amount")
	}

	uid, err := ctx.AccountMgr.GetUIDByAddress(caller)
	if err != nil {
		return ctx.Error(err.Error())
	}

	assetID := uint32(big.NewInt(0).SetBytes(args[0]).Uint64())
	amount := big.NewInt(0).SetBytes(args[1]).Uint64()

	bal := ctx.AccountMgr.LoadBalance(uid, assetID)
	if bal.Available < amount {
		return ctx.Error(fmt.Sprintf("insufficient balance: have %d, need %d", bal.Available, amount))
	}
	bal.Available -= amount
	ctx.AccountMgr.SaveBalance(uid, assetID, bal)

	return vmcommon.Ok
}

// GetBalance handles getBalance endpoint.
func GetBalance(ctx *Context, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 2 {
		return ctx.Error("getBalance requires uid, assetID")
	}

	uid := big.NewInt(0).SetBytes(args[0]).Uint64()
	assetID := uint32(big.NewInt(0).SetBytes(args[1]).Uint64())

	bal := ctx.AccountMgr.LoadBalance(uid, assetID)

	ctx.Eei.Finish(big.NewInt(int64(bal.Available)).Bytes())
	ctx.Eei.Finish(big.NewInt(int64(bal.Locked)).Bytes())
	return vmcommon.Ok
}

// GetUID handles getUID endpoint.
func GetUID(ctx *Context, args [][]byte) vmcommon.ReturnCode {
	if len(args) < 1 {
		return ctx.Error("getUID requires address")
	}

	uid, err := ctx.AccountMgr.GetUIDByAddress(args[0])
	if err != nil {
		return ctx.Error(err.Error())
	}

	ctx.Eei.Finish(big.NewInt(int64(uid)).Bytes())
	return vmcommon.Ok
}
