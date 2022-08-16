package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestEnvParameter(t *testing.T) {
	url := pnWsUrl

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ec, err := ethclient.Dial(url)
	require.NoError(t, err)

	// get registry address
	registry := testRegistryAddress(t, url)

	block := (*big.Int)(nil) //big.NewInt(100000)

	header, err := ec.HeaderByNumber(ctx, block)
	require.NoError(t, err)
	t.Log("block root", header.Root)
	block = new(big.Int).Sub(header.Number, big.NewInt(126))

	// get env address
	res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &registry, Data: pack(t, "getContractAddress(bytes32)", utf8ToHash("EnvStorage"))}, block)
	require.NoError(t, err)
	env := unpack(t, res, "address")[0].(common.Address)
	t.Log("envStorage address:", env)

	// get blockInterval
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getBlockCreationTime()")}, block)
	require.NoError(t, err)
	blockInterval := unpack(t, res, "uint256")[0].(*big.Int)
	t.Log("blockInterval:", blockInterval)

	// get blocksPer
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getBlocksPer()")}, block)
	require.NoError(t, err)
	blocksPer := unpack(t, res, "uint256")[0].(*big.Int)
	t.Log("blocksPer:", blocksPer)

	// get maxIdleBlockInterval
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getMaxIdleBlockInterval()")}, block)
	require.NoError(t, err)
	maxIdleBlockInterval := unpack(t, res, "uint256")[0].(*big.Int)
	t.Log("maxIdleBlockInterval:", maxIdleBlockInterval)

	// get blockRewardAmount
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getBlockRewardAmount()")}, block)
	require.NoError(t, err)
	blockRewardAmount := unpack(t, res, "uint256")[0].(*big.Int)
	t.Log("blockRewardAmount:", blockRewardAmount)

	// get maxPriorityFeePerGas
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getMaxPriorityFeePerGas()")}, block)
	require.NoError(t, err)
	maxPriorityFeePerGas := unpack(t, res, "uint256")[0].(*big.Int)
	t.Log("maxPriorityFeePerGas:", maxPriorityFeePerGas)

	// get gasLimitAndBaseFee
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getGasLimitAndBaseFee()")}, block)
	require.NoError(t, err)
	gasLimitAndBaseFee := unpack(t, res, "uint256", "uint256", "uint256")
	t.Log("gasLimit:", gasLimitAndBaseFee[0])
	t.Log("baseFeeMaxChangeRate:", gasLimitAndBaseFee[1])
	t.Log("gasTargetPercentage:", gasLimitAndBaseFee[2])

	// get maxBaseFee
	res, err = ec.CallContract(ctx, ethereum.CallMsg{To: &env, Data: pack(t, "getMaxBaseFee()")}, block)
	require.NoError(t, err)
	maxBaseFee := unpack(t, res, "uint256")[0].(*big.Int)
	t.Log("maxBaseFee:", maxBaseFee)
}
