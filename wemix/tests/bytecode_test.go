package test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

type contractSource struct {
	Name        string
	Path        string
	Alias       string
	RuntimeCode []byte
	Code        []byte
	ABI         *abi.ABI
	Address     common.Address
	TX          common.Hash
	ProxIndex   int
}

func (c *contractSource) String() string {
	return fmt.Sprintf("%s:%s", c.Path, c.Name)
}

func TestGovContractByteCode(t *testing.T) {
	registryAddress := testRegistryAddress(t, apiurl)

	registredIndex := 5
	contracts := []*contractSource{
		{Name: "Registry", Path: "./governance-contract/contracts/Registry.sol", Alias: "Registry", Address: registryAddress},
		{Name: "Staking", Path: "./governance-contract/contracts/Staking.sol", Alias: "Staking"},
		{Name: "BallotStorage", Path: "./governance-contract/contracts/storage/BallotStorage.sol", Alias: "BallotStorage"},
		{Name: "EnvStorage", Path: "./governance-contract/contracts/storage/EnvStorage.sol", Alias: "EnvStorage"},
		{Name: "Gov", Path: "./governance-contract/contracts/Gov.sol", Alias: "GovernanceContract"},
		{Name: "EnvStorageImp", Path: "./governance-contract/contracts/storage/EnvStorageImp.sol", ProxIndex: 3, TX: common.BytesToHash(hexutil.MustDecode("0xa92bd0055a04c7f321e839dac6bf14445bf6ed77aaccf09b43e5c762e548b1fb"))},
		{Name: "GovImp", Path: "./governance-contract/contracts/GovImp.sol", ProxIndex: 4, TX: common.BytesToHash(hexutil.MustDecode("0x1453d2094064f04967bc5c2f69fab3a53da53c374c78fc89126da2055c3bf72e"))},
	}

	solFiles := make([]string, len(contracts))
	for i, c := range contracts {
		solFiles[i] = c.Path
	}

	compiledAll := compile(t, solFiles...)
	for _, c := range contracts {
		compiled := compiledAll.Contracts[c.String()]
		c.ABI = parseABI(t, compiled.Abi)
		c.RuntimeCode = hexutil.MustDecode("0x" + compiled.BinRuntime)
		c.Code = hexutil.MustDecode("0x" + compiled.Bin)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(apiurl)
	require.NoError(t, err)

	// collect all addresses
	for _, c := range contracts[1:registredIndex] {
		alias := common.RightPadBytes([]byte(c.Alias), 32)

		output, err := ec.CallContract(ctx, ethereum.CallMsg{
			To:   &registryAddress,
			Data: packInput(t, contracts[0].ABI, "getContractAddress", common.BytesToHash(alias)),
		}, nil)
		require.NoError(t, err)

		v := unpackOutput(t, contracts[0].ABI, "getContractAddress", output)
		c.Address = v[0].(common.Address)
		t.Log(c.Name, c.Address)
	}

	IMPLEMENTATION_SLOT := hexutil.MustDecode("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc")

	for _, c := range contracts[registredIndex:] {
		output, err := ec.StorageAt(ctx, contracts[c.ProxIndex].Address, common.BytesToHash(IMPLEMENTATION_SLOT), nil)
		require.NoError(t, err)
		c.Address = common.BytesToAddress(output)
		t.Log(c.Name, c.Address)
	}

	metaDelimiter := hexutil.MustDecode("0x" + "a26469706673")

	// check bytecode
	for _, c := range contracts {
		var actual, expected []byte
		if c.TX == (common.Hash{}) {
			actual, err = ec.CodeAt(ctx, c.Address, nil)
			require.NoError(t, err)
			expected = c.RuntimeCode
		} else {
			tx, _, err := ec.TransactionByHash(ctx, c.TX)
			require.NoError(t, err)
			actual = tx.Data()
			expected = c.Code
		}

		splited := bytes.Split(actual, metaDelimiter)
		require.Equal(t, 2, len(splited))
		actual = splited[0]

		splited = bytes.Split(expected, metaDelimiter)
		require.Equal(t, 2, len(splited))
		expected = splited[0]

		require.Equal(t, expected, actual, c.Name)
	}

}
