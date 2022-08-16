package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestFee(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	block, err := ec.BlockByNumber(ctx, nil)
	require.NoError(t, err)
	suggestGasPrice, err := ec.SuggestGasPrice(ctx)
	require.NoError(t, err)
	suggestGasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)

	t.Log("block", block.Number(), "txs", len(block.Transactions()), "feeBase", block.BaseFee(), "suggestGasPrice", suggestGasPrice, "suggestGasTipCap", suggestGasTipCap)

	lastFeeBase := big.NewInt(0)
	num := int64(0)
	for {
		block, err := ec.BlockByNumber(ctx, big.NewInt(num))
		if err != nil {
			t.Log(err)
			break
		}
		txs := len(block.Transactions())
		if lastFeeBase.Cmp(block.BaseFee()) != 0 {
			suggestGasPrice, err := ec.SuggestGasPrice(ctx)
			require.NoError(t, err)
			t.Logf("block: %v, txs: %v, fees: %v, feebase: %v, suggestGasPrice: %v, gasUsed: %v, gaslimit: %v", num, txs, block.Fees(), block.BaseFee(), suggestGasPrice, block.GasUsed(), block.GasLimit())
			lastFeeBase = block.BaseFee()
		}
		num++
	}
	latest, err := ec.BlockNumber(ctx)
	require.NoError(t, err)

	t.Logf("read blocknumber: %v, latest: %v", num, latest)
}

func TestHeaderFees(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	block, err := ec.BlockByNumber(ctx, big.NewInt(834054))
	require.NoError(t, err)

	totalCost := big.NewInt(0)
	for _, tx := range block.Transactions() {
		cost := new(big.Int).Mul(tx.GasPrice(), new(big.Int).SetUint64(tx.Gas()))
		totalCost.Add(totalCost, cost)
	}

	t.Logf("blocknum: %v, fees: %v, totalcost: %v", block.Number(), block.Header().Fees, totalCost)

}
