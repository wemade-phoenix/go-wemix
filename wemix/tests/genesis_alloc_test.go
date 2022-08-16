package test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

var chainID = params.AllEthashProtocolChanges.ChainID
var signer = types.NewLondonSigner(chainID)

func parseABI(t *testing.T, abii interface{}) *abi.ABI {
	abiString, ok := abii.(string)
	if !ok {
		bytes, err := json.Marshal(abii)
		require.NoError(t, err)
		abiString = string(bytes)
	}
	abiparsed, err := abi.JSON(strings.NewReader(abiString))
	require.NoError(t, err)
	return &abiparsed
}

func packInput(t *testing.T, abiParsed *abi.ABI, method string, args ...interface{}) (data []byte) {
	var m abi.Method
	if method != "" {
		var ok bool
		m, ok = abiParsed.Methods[method]
		require.True(t, ok)
	} else {
		m = abiParsed.Constructor
	}

	input, err := m.Inputs.Pack(args...)
	require.NoError(t, err)

	data = append(m.ID, input...)
	require.NoError(t, err)
	return
}

func unpackOutput(t *testing.T, abiParsed *abi.ABI, method string, output []byte) (v []interface{}) {
	m, ok := abiParsed.Methods[method]
	require.True(t, ok)

	var err error
	v, err = m.Outputs.Unpack(output)
	require.NoError(t, err)
	return
}

func sendTx(ctx context.Context, backend *backends.SimulatedBackend, to *common.Address, data []byte, fromPK *ecdsa.PrivateKey) (tx *types.Transaction, receipt *types.Receipt, err error) {
	gas, err := backend.SuggestGasPrice(ctx)
	if err != nil {
		return
	}

	from := crypto.PubkeyToAddress(fromPK.PublicKey)

	nonce, err := backend.PendingNonceAt(ctx, from)
	if err != nil {
		return
	}

	if to != nil {
		tx = types.NewTransaction(nonce, *to, big.NewInt(0), 10000000, gas, data)
	} else {
		tx = types.NewContractCreation(nonce, big.NewInt(0), 10000000, gas, data)
	}

	tx, err = types.SignTx(tx, signer, fromPK)
	if err != nil {
		return
	}

	err = backend.SendTransaction(ctx, tx)
	if err != nil {
		return
	}

	backend.Commit()

	receipt, err = backend.TransactionReceipt(ctx, tx.Hash())
	return
}

func TestGenesisAllocRegistry(t *testing.T) {
	var (
		coinbasePK, _ = crypto.GenerateKey()
		coinbase      = crypto.PubkeyToAddress(coinbasePK.PublicKey)

		// compile
		solFiles = []string{"./governance-contract/contracts/Registry.sol", "./governance-contract/contracts/Staking.sol"}
		compiled = compile(t, solFiles...)

		// registry
		registryAddress = common.HexToAddress("0x99")
		registry        = compiled.Contracts[fmt.Sprintf("%v:%v", solFiles[0], "Registry")]
		registryRCode   = hexutil.MustDecode("0x" + registry.BinRuntime)
		registryABI     = parseABI(t, registry.Abi)

		// staking
		stakingAddress common.Address
		stakingName    = common.BytesToHash(common.RightPadBytes([]byte("Staking"), 32)[:])
		staking        = compiled.Contracts[fmt.Sprintf("%v:%v", solFiles[1], "Staking")]
		stakingABI     = parseABI(t, staking.Abi)

		genesisConfig = core.GenesisAlloc{
			coinbase: {
				Balance: new(big.Int).Mul(big.NewInt(1000000000), big.NewInt(1e18)),
			},
			registryAddress: {
				Balance: big.NewInt(0),
				Code:    registryRCode,
				Storage: map[common.Hash]common.Hash{
					{0}: common.BytesToHash(coinbase[:]), // assign owner address
				},
			},
		}
		// make simulated backend
		backend = backends.NewSimulatedBackend(genesisConfig, 10000000)
	)

	ctx := context.Background()

	t.Run("call and check owner", func(t *testing.T) {
		output, err := backend.CallContract(ctx, ethereum.CallMsg{
			To:   &registryAddress,
			Data: packInput(t, registryABI, "owner"),
		}, nil)
		require.NoError(t, err)

		v := unpackOutput(t, registryABI, "owner", output)
		require.Equal(t, coinbase, v[0])
	})

	t.Run("deploy Staking contract", func(t *testing.T) {
		// deploy
		input := append(
			hexutil.MustDecode("0x"+staking.Bin),
			packInput(t, stakingABI, "", registryAddress, []byte{})...,
		)
		tx, receipt, err := sendTx(ctx, backend, nil, input, coinbasePK)
		require.NoError(t, err)

		// check address
		stakingAddress = receipt.ContractAddress
		require.Equal(t, crypto.CreateAddress(coinbase, tx.Nonce()), stakingAddress)

		// check staking's reg
		output, err := backend.CallContract(ctx, ethereum.CallMsg{
			To:   &stakingAddress,
			Data: packInput(t, stakingABI, "reg"),
		}, nil)
		require.NoError(t, err)
		v := unpackOutput(t, stakingABI, "reg", output)
		require.Equal(t, registryAddress, v[0])

	})

	t.Run("setContractDomain", func(t *testing.T) {
		// send tx
		input := packInput(t, registryABI, "setContractDomain", stakingName, stakingAddress)
		_, receipt, err := sendTx(ctx, backend, &registryAddress, input, coinbasePK)
		require.NoError(t, err)
		require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)

		// check address
		output, err := backend.CallContract(ctx, ethereum.CallMsg{
			To:   &registryAddress,
			Data: packInput(t, registryABI, "getContractAddress", stakingName),
		}, nil)
		require.NoError(t, err)

		v := unpackOutput(t, registryABI, "getContractAddress", output)
		require.Equal(t, stakingAddress, v[0])
	})
}
