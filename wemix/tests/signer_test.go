package test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestSigner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(apiurl)
	require.NoError(t, err)

	block, err := ec.BlockByNumber(ctx, nil)
	require.NoError(t, err)

	// recover
	pubkey, err := crypto.Ecrecover(block.Root().Bytes(), block.Header().MinerNodeSig)
	require.NoError(t, err)
	recovered := common.BytesToAddress(crypto.Keccak256(pubkey[1:])[12:])

	// compair recovered with MinerNodeId
	expected := common.BytesToAddress(crypto.Keccak256(block.MinerNodeId())[12:])

	require.Equal(t, expected, recovered)

	t.Log("blocknumber:", block.Number(), "coinbase:", hexutil.Encode(block.Coinbase().Bytes()), "miner-node-id:", hexutil.Encode(expected[:]), "recovered:", hexutil.Encode(recovered[:]))
}
