package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestRegistryAddress(t *testing.T) {
	testRegistryAddress(t, "wss://ws-twemix.phnxops.in/")
}

func testRegistryAddress(t *testing.T, url string) common.Address {
	magic, _ := big.NewInt(0).SetString("0x57656d6978205265676973747279", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.DialContext(ctx, url)
	require.NoError(t, err)

	header, err := ec.HeaderByNumber(ctx, big.NewInt(0))
	require.NoError(t, err)

	coinbase := header.Coinbase
	var registry common.Address

	for i := uint64(0); i < 10; i++ {
		ca := crypto.CreateAddress(coinbase, i)

		res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &ca, Data: pack(t, "magic()")}, nil)
		require.NoError(t, err, i)

		if len(res) >= 0x20 {
			output := unpack(t, res, "uint256")[0].(*big.Int)
			if magic.Cmp(output) == 0 {
				registry = ca
				break
			}
		}
	}
	//assert.NotEqual(t, common.Address{}, registry)
	t.Logf("registry address: %v", registry)
	return registry
}
