package storage

import (
	"testing"

	"github.com/multiversx/mx-chain-go/vm/systemSmartContracts/clob/core"
	"github.com/stretchr/testify/require"
)

func TestMarshalOrder(t *testing.T) {
	// Create a typical order with small IDs and prices
	order := &core.Order{
		ID:        1000,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  5000,
		Price:     10000,
		Stop:      0,
		TIF:       core.TIFGoodTillCancel,
		OCO:       0,
		UID:       50,
	}

	// Marshal order
	data, err := MarshalOrder(order)
	require.NoError(t, err)

	t.Logf("Order size: %d bytes", len(data))

	// Unmarshal and verify
	decoded, err := UnmarshalOrder(data)
	require.NoError(t, err)
	require.Equal(t, order.ID, decoded.ID)
	require.Equal(t, order.Side, decoded.Side)
	require.Equal(t, order.OrderType, decoded.OrderType)
	require.Equal(t, order.Quantity, decoded.Quantity)
	require.Equal(t, order.Price, decoded.Price)
	require.Equal(t, order.Stop, decoded.Stop)
	require.Equal(t, order.TIF, decoded.TIF)
	require.Equal(t, order.OCO, decoded.OCO)
	require.Equal(t, order.UID, decoded.UID)
}

func TestMarshalOrder_EdgeCases(t *testing.T) {
	t.Run("nil order", func(t *testing.T) {
		data, err := MarshalOrder(nil)
		require.NoError(t, err)
		require.Nil(t, data)

		decoded, err := UnmarshalOrder(nil)
		require.NoError(t, err)
		require.Nil(t, decoded)
	})

	t.Run("zero values", func(t *testing.T) {
		order := &core.Order{}
		data, err := MarshalOrder(order)
		require.NoError(t, err)

		decoded, err := UnmarshalOrder(data)
		require.NoError(t, err)
		require.NotNil(t, decoded)
	})

	t.Run("large IDs", func(t *testing.T) {
		order := &core.Order{
			ID:        9999999999,
			Side:      core.SideSell,
			OrderType: core.TypeStopLimit,
			Quantity:  999999999999,
			Price:     888888888888,
			Stop:      777777777777,
			TIF:       core.TIFImmediateOrCancel,
			OCO:       666666666666,
			UID:       555555555555,
		}

		data, err := MarshalOrder(order)
		require.NoError(t, err)

		decoded, err := UnmarshalOrder(data)
		require.NoError(t, err)
		require.Equal(t, order.ID, decoded.ID)
		require.Equal(t, order.Side, decoded.Side)
		require.Equal(t, order.OrderType, decoded.OrderType)
		require.Equal(t, order.Quantity, decoded.Quantity)
		require.Equal(t, order.Price, decoded.Price)
		require.Equal(t, order.Stop, decoded.Stop)
		require.Equal(t, order.TIF, decoded.TIF)
		require.Equal(t, order.OCO, decoded.OCO)
		require.Equal(t, order.UID, decoded.UID)
	})
}

func TestMarshalEngineState(t *testing.T) {
	orders := []*core.Order{
		{
			ID:        1,
			Side:      core.SideBuy,
			OrderType: core.TypeLimit,
			Quantity:  100,
			Price:     1000,
			TIF:       core.TIFGoodTillCancel,
			UID:       1,
		},
		{
			ID:        2,
			Side:      core.SideSell,
			OrderType: core.TypeLimit,
			Quantity:  200,
			Price:     2000,
			TIF:       core.TIFGoodTillCancel,
			UID:       2,
		},
	}

	lastPrice := uint64(1500)

	// Marshal engine state
	data, err := MarshalEngineState(3, &lastPrice, orders)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.True(t, len(data) > 0)

	t.Logf("Engine state with 2 orders: %d bytes", len(data))

	// Unmarshal and verify
	nextOrderID, decodedLastPrice, decodedOrders, err := UnmarshalEngineState(data)
	require.NoError(t, err)
	require.Equal(t, uint64(3), nextOrderID)
	require.NotNil(t, decodedLastPrice)
	require.Equal(t, lastPrice, *decodedLastPrice)
	require.Len(t, decodedOrders, 2)

	// Verify orders
	for i, order := range orders {
		decoded := decodedOrders[i]
		require.Equal(t, order.ID, decoded.ID)
		require.Equal(t, order.Side, decoded.Side)
		require.Equal(t, order.OrderType, decoded.OrderType)
		require.Equal(t, order.Quantity, decoded.Quantity)
		require.Equal(t, order.Price, decoded.Price)
		require.Equal(t, order.TIF, decoded.TIF)
		require.Equal(t, order.UID, decoded.UID)
	}
}

func BenchmarkMarshalOrder(b *testing.B) {
	order := &core.Order{
		ID:        1000,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  5000,
		Price:     10000,
		Stop:      0,
		TIF:       core.TIFGoodTillCancel,
		OCO:       0,
		UID:       50,
	}

	for i := 0; i < b.N; i++ {
		_, _ = MarshalOrder(order)
	}
}

func BenchmarkUnmarshalOrder(b *testing.B) {
	order := &core.Order{
		ID:        1000,
		Side:      core.SideBuy,
		OrderType: core.TypeLimit,
		Quantity:  5000,
		Price:     10000,
		Stop:      0,
		TIF:       core.TIFGoodTillCancel,
		OCO:       0,
		UID:       50,
	}

	data, _ := MarshalOrder(order)

	for i := 0; i < b.N; i++ {
		_, _ = UnmarshalOrder(data)
	}
}
