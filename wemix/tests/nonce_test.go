package test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestNonceAny(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	account := common.HexToAddress("0x050D08D65BCA6208bF1eea75036666c39f5F3036")

	nonce, err := ec.NonceAt(ctx, account, nil)
	require.NoError(t, err)

	pendingNonce, err := ec.PendingNonceAt(ctx, account)
	require.NoError(t, err)

	t.Logf("account: %v, nonce: %v, pendingNonce: %v", account, nonce, pendingNonce)

}
