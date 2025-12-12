package clob

import (
	"fmt"
	"sync"

	"github.com/multiversx/mx-chain-core-go/core"
	"github.com/multiversx/mx-chain-core-go/core/check"
	vmcommon "github.com/multiversx/mx-chain-vm-common-go"

	"github.com/multiversx/mx-chain-go/vm"
	clobcore "github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/endpoints"
)

// Endpoint constants
const (
	RegisterEndpoint   = "register"
	DepositEndpoint    = "deposit"
	WithdrawEndpoint   = "withdraw"
	CreatePairEndpoint = "createPair"
	SetConfigEndpoint  = "setConfig"
	GetBalanceEndpoint = "getBalance"
	GetUIDEndpoint     = "getUID"
)

// ArgsNewCLOBSmartContract holds arguments for creating a CLOB system smart contract
type ArgsNewCLOBSmartContract struct {
	Eei       vm.SystemEI
	GasCost   vm.GasCost
	SelfAddr  []byte
	OwnerAddr []byte
}

// clobSystemSC implements vm.SystemSmartContract for the CLOB.
type clobSystemSC struct {
	eei          vm.SystemEI
	gasCost      vm.GasCost
	selfAddr     []byte
	ownerAddr    []byte
	mutExecution sync.RWMutex
}

// NewCLOBSmartContract creates a new CLOB system smart contract instance
func NewCLOBSmartContract(args ArgsNewCLOBSmartContract) (*clobSystemSC, error) {
	if check.IfNil(args.Eei) {
		return nil, vm.ErrNilSystemEnvironmentInterface
	}
	if len(args.SelfAddr) == 0 {
		return nil, fmt.Errorf("%w for CLOB SC address", vm.ErrInvalidAddress)
	}
	if len(args.OwnerAddr) == 0 {
		return nil, vm.ErrInvalidAddress
	}

	return &clobSystemSC{
		eei:       args.Eei,
		gasCost:   args.GasCost,
		selfAddr:  args.SelfAddr,
		ownerAddr: args.OwnerAddr,
	}, nil
}

// Execute implements vm.SystemSmartContract
func (c *clobSystemSC) Execute(args *vmcommon.ContractCallInput) vmcommon.ReturnCode {
	c.mutExecution.Lock()
	defer c.mutExecution.Unlock()

	if checkIfNil(args) != nil {
		return vmcommon.UserError
	}

	fn := args.Function
	caller := args.VMInput.CallerAddr
	arguments := args.VMInput.Arguments

	// Create endpoint context for delegation
	ctx := endpoints.NewContext(c.eei, c.ownerAddr)

	switch fn {
	case core.SCDeployInitFunctionName:
		return c.init()

	// Account endpoints
	case RegisterEndpoint:
		return endpoints.Register(ctx, caller)
	case DepositEndpoint:
		return endpoints.Deposit(ctx, caller, arguments)
	case WithdrawEndpoint:
		return endpoints.Withdraw(ctx, caller, arguments)
	case GetBalanceEndpoint:
		return endpoints.GetBalance(ctx, arguments)
	case GetUIDEndpoint:
		return endpoints.GetUID(ctx, arguments)

	// Admin endpoints
	case CreatePairEndpoint:
		return endpoints.CreatePair(ctx, caller, arguments)
	case SetConfigEndpoint:
		return endpoints.SetConfig(ctx, caller, arguments)

	// Order endpoints
	case clobcore.ProcessOrderEndpoint:
		return endpoints.ProcessOrder(ctx, caller, arguments)
	case clobcore.CancelOrderEndpoint:
		return endpoints.CancelOrder(ctx, caller, arguments)
	case clobcore.GetOrderEndpoint:
		return endpoints.GetOrder(ctx, arguments)
	case clobcore.GetDepthEndpoint:
		return endpoints.GetDepth(ctx, arguments)
	case clobcore.MatchOrdersEndpoint:
		return endpoints.MatchOrders(ctx, arguments)

	default:
		c.eei.AddReturnMessage("invalid function: " + fn)
		return vmcommon.UserError
	}
}

// CanUseContract implements vm.SystemSmartContract
func (c *clobSystemSC) CanUseContract() bool {
	return true
}

// SetNewGasCost implements vm.SystemSmartContract
func (c *clobSystemSC) SetNewGasCost(gasCost vm.GasCost) {
	c.mutExecution.Lock()
	defer c.mutExecution.Unlock()
	c.gasCost = gasCost
}

// IsInterfaceNil implements vm.SystemSmartContract
func (c *clobSystemSC) IsInterfaceNil() bool {
	return c == nil
}

// init handles contract initialization
func (c *clobSystemSC) init() vmcommon.ReturnCode {
	return vmcommon.Ok
}

// checkIfNil validates contract call input
func checkIfNil(args *vmcommon.ContractCallInput) error {
	if args == nil {
		return vm.ErrInputArgsIsNil
	}
	if args.CallValue == nil {
		return vm.ErrInputCallValueIsNil
	}
	if args.Function == "" {
		return vm.ErrInputFunctionIsNil
	}
	if args.CallerAddr == nil {
		return vm.ErrInputCallerAddrIsNil
	}
	if args.RecipientAddr == nil {
		return vm.ErrInputRecipientAddrIsNil
	}
	return nil
}

var _ vm.SystemSmartContract = (*clobSystemSC)(nil)
