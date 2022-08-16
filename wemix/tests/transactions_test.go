package test

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestTransactions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(apiurl)
	require.NoError(t, err)

	totalTx := 0
	totalPayload := 0
	num := int64(0)
	for {
		block, err := ec.BlockByNumber(ctx, big.NewInt(num))
		if err != nil {
			t.Log(err)
			break
		}
		txs := len(block.Transactions())
		payload := 0
		for _, tx := range block.Transactions() {
			if len(tx.Data()) > 0 {
				payload++
			}
		}
		if txs > 0 {
			t.Logf("block: %v, txs: %v, payload: %v", num, txs, payload)
		}
		totalTx += txs
		totalPayload += payload
		num++
	}
	latest, err := ec.BlockNumber(ctx)
	require.NoError(t, err)

	t.Logf("blocknumber: %v, totalTx: %v, latest: %v", num, totalTx, latest)
}

func TestTransactionsBeferAdminDeployed(t *testing.T) {
	registryAddress := testRegistryAddress(t, "http://3.38.224.208:8588")
	require.Equal(t, common.Address{}, registryAddress)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("http://3.38.224.208:8588")
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)
	signer := types.NewLondonSigner(chainID)

	val, ok := new(big.Int).SetString("1", 10)
	require.True(t, ok)

	balance, err := ec.BalanceAt(ctx, holder, nil)
	require.NoError(t, err)
	t.Logf("holder's balance before: %v", balance)

	for _, toPK := range getTesters(t)[:10] {
		gasPrice, err := ec.SuggestGasPrice(ctx)
		require.NoError(t, err)

		to := crypto.PubkeyToAddress(toPK.PublicKey)

		nonce, err := ec.NonceAt(ctx, holder, nil)
		require.NoError(t, err)

		tx := types.NewTransaction(nonce, to, val, 21000, gasPrice, nil)
		sig, err := crypto.Sign(signer.Hash(tx).Bytes(), holderPK)
		require.NoError(t, err)
		tx, err = tx.WithSignature(signer, sig)
		require.NoError(t, err)

		err = ec.SendTransaction(ctx, tx)
		require.NoError(t, err)

		start := time.Now()
		receipt, err := bind.WaitMined(ctx, ec, tx)
		require.NoError(t, err)

		fmt.Println("receipt", receipt.Status, "to", tx.To(), "since", time.Since(start))
	}

	balance, err = ec.BalanceAt(ctx, holder, nil)
	require.NoError(t, err)
	t.Logf("holder's balance after: %v", balance)

}
