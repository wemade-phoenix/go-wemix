package test

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	bip39 "github.com/tyler-smith/go-bip39"
)

func TestGenTesters(t *testing.T) {
	entropy, err := bip39.NewEntropy(128)
	require.NoError(t, err)
	mnemonic, err := bip39.NewMnemonic(entropy)
	require.NoError(t, err)

	t.Logf("mnemonic: %v", mnemonic)
}

func TestDistributeValue(t *testing.T) {
	var (
		url                      = privateURL
		transferAmountETH int64  = 1000
		pendingTxs               = sync.Map{}
		pendingWg                = sync.WaitGroup{}
		totalTx           uint32 = 0
		loopCount                = 0
		maxLooping               = 1
		loopInterval             = time.Duration(5e9)
	)

start:

	loopCount++
	if loopCount > maxLooping {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.DialContext(ctx, url)
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)

	balance, err := ec.BalanceAt(ctx, holder, nil)
	require.NoError(t, err)

	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)

	distribute := decimal.NewFromBigInt(big.NewInt(transferAmountETH), 18).BigInt()
	t.Logf("chainID: %v, url: %v, holder's balance: %v gasTipCap: %v, distribute: %v", chainID, url, decimal.NewFromBigInt(balance, -18), gasTipCap, decimal.NewFromBigInt(distribute, -18))

	nonce, err := ec.NonceAt(ctx, holder, nil)
	require.NoError(t, err)

	gasFeeCap := func() *big.Int {
		head, err := ec.HeaderByNumber(ctx, nil)
		require.NoError(t, err)

		return new(big.Int).Add(
			gasTipCap,
			new(big.Int).Mul(head.BaseFee, big.NewInt(200)),
		)
	}()

	for i, to := range getTestersAddress(t) {
		pendingWg.Add(1)
		go func(i int, to common.Address) {
			tx, err := types.SignNewTx(holderPK, types.NewLondonSigner(chainID), &types.DynamicFeeTx{
				ChainID:   chainID,
				Nonce:     nonce + uint64(i),
				To:        &to,
				GasTipCap: gasTipCap,
				GasFeeCap: gasFeeCap,
				Gas:       21000,
				Value:     distribute,
				Data:      nil,
			})
			require.NoError(t, err)

			err = ec.SendTransaction(ctx, tx)
			require.NoError(t, err)

			pendingTxs.Store(tx.Hash(), func(tx *types.Transaction) {
				atomic.AddUint32(&totalTx, 1)
				t.Log(tx.To(), tx.Nonce(), tx.Hash(), "done")
				pendingWg.Done()
			})
		}(i, to)
	}

	go func() {
		headerCh := make(chan *types.Header, 100)
		sub, err := ec.SubscribeNewHead(ctx, headerCh)
		require.NoError(t, err)

		defer sub.Unsubscribe()

		for {
			select {
			case head := <-headerCh:
				block, err := ec.BlockByNumber(ctx, head.Number)
				require.NoError(t, err)

				for _, tx := range block.Transactions() {
					if fn, ok := pendingTxs.LoadAndDelete(tx.Hash()); ok {
						go fn.(func(tx *types.Transaction))(tx)
					}
				}
			case err := <-sub.Err():
				require.NoError(t, err)
				return
			}
		}
	}()
	pendingWg.Wait()

	t.Log("all done", totalTx, "loop count", loopCount)
	totalTx = 0

	<-time.After(loopInterval)
	goto start
}

func TestGatherValue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)

	for i, fromPK := range getTesters(t) {
		from := crypto.PubkeyToAddress(fromPK.PublicKey)

		nonce, err := ec.NonceAt(ctx, from, nil)
		require.NoError(t, err)

		gasprice, err := ec.SuggestGasPrice(ctx)
		require.NoError(t, err)
		balance, err := ec.BalanceAt(ctx, from, nil)
		require.NoError(t, err)

		amount := balance.Sub(balance, new(big.Int).Mul(gasprice, big.NewInt(21000)))
		if amount.Sign() <= 0 {
			continue
		}

		tx, err := types.SignNewTx(fromPK, types.NewLondonSigner(chainID), &types.LegacyTx{Nonce: nonce, To: &holder, GasPrice: gasprice, Gas: 21000, Value: amount, Data: nil})
		require.NoError(t, err)

		start := time.Now()
		err = ec.SendTransaction(ctx, tx)
		require.NoError(t, err)
		receipt, err := bind.WaitMined(ctx, ec, tx)
		require.NoError(t, err)
		t.Log("receipt", receipt.Status, "to", tx.To(), "mining", time.Since(start), "index", i, "amount", amount)
	}
}
