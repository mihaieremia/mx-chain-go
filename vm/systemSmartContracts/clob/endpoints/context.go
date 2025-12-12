package endpoints

import (
	vmcommon "github.com/multiversx/mx-chain-vm-common-go"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/account"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/pair"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/storage"
)

// EEI is the subset of vm.SystemEI needed by endpoints.
type EEI interface {
	storage.StateStore
	Finish(value []byte)
	AddReturnMessage(msg string)
}

// Context holds all dependencies needed by endpoint handlers.
type Context struct {
	Eei        EEI
	AccountMgr *account.Manager
	PairMgr    *pair.Manager
	OwnerAddr  []byte
}

// NewContext creates a new endpoint context.
func NewContext(eei EEI, ownerAddr []byte) *Context {
	accountMgr := account.NewManager(eei)
	pairMgr := pair.NewManager(eei)
	return &Context{
		Eei:        eei,
		AccountMgr: accountMgr,
		PairMgr:    pairMgr,
		OwnerAddr:  ownerAddr,
	}
}

// Error returns a UserError with the given message.
func (ctx *Context) Error(msg string) vmcommon.ReturnCode {
	ctx.Eei.AddReturnMessage(msg)
	return vmcommon.UserError
}
