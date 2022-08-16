package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestPendingCall(t *testing.T) {
	path := "./openzeppelin-contracts/contracts/mocks/ERC20Mock.sol"
	compiled, ok := compile(t, path).Contracts[fmt.Sprintf("%v:ERC20Mock", path)]
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.DialContext(ctx, "wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)
	signer := types.NewLondonSigner(chainID)

	abibytes, _ := json.Marshal(compiled.Abi)
	abiparsed, err := abi.JSON(bytes.NewReader(abibytes))
	require.NoError(t, err)

	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)

	toPK, _ := crypto.GenerateKey()
	to := crypto.PubkeyToAddress(toPK.PublicKey)
	var ca common.Address
	initialSupply := decimal.NewFromBigInt(big.NewInt(1e9), 18).BigInt()

	// deploy
	{
		header, err := ec.HeaderByNumber(ctx, nil)
		require.NoError(t, err)
		gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))
		nonce, err := ec.NonceAt(ctx, holder, nil)
		require.NoError(t, err)

		constructorInputs, err := abiparsed.Constructor.Inputs.Pack("ERC20Mock", "test token", holder, initialSupply)
		require.NoError(t, err)

		tx, err := types.SignNewTx(holderPK, signer, &types.DynamicFeeTx{ChainID: signer.ChainID(), Nonce: nonce, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: 1000000,
			To:   nil,
			Data: append(common.Hex2Bytes(compiled.Bin), constructorInputs...),
		})
		require.NoError(t, err)

		require.NoError(t, ec.SendTransaction(ctx, tx))
		receipt, err := bind.WaitMined(ctx, ec, tx)
		require.NoError(t, err)
		require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
		t.Log("new contract", receipt.ContractAddress)
		ca = receipt.ContractAddress
	}

	// transfer
	{
		header, err := ec.HeaderByNumber(ctx, nil)
		require.NoError(t, err)
		gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))
		nonce, err := ec.NonceAt(ctx, holder, nil)
		require.NoError(t, err)

		transferInputs, err := abiparsed.Methods["transfer"].Inputs.Pack(to, initialSupply)
		require.NoError(t, err)

		tx, err := types.SignNewTx(holderPK, signer, &types.DynamicFeeTx{ChainID: signer.ChainID(), Nonce: nonce, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: 1000000,
			To:   &ca,
			Data: append(abiparsed.Methods["transfer"].ID, transferInputs...),
		})
		require.NoError(t, err)

		require.NoError(t, ec.SendTransaction(ctx, tx))
		// receipt, err := bind.WaitMined(ctx, ec, tx)
		// require.NoError(t, err)
		// require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	}

	// call balance
	{
		balanceOfInputs, err := abiparsed.Methods["balanceOf"].Inputs.Pack(holder)
		require.NoError(t, err)

		res, err := ec.PendingCallContract(ctx, ethereum.CallMsg{To: &ca, Data: append(abiparsed.Methods["balanceOf"].ID, balanceOfInputs...)})
		require.NoError(t, err)

		output, err := abiparsed.Methods["balanceOf"].Outputs.Unpack(res)
		require.NoError(t, err)

		require.Equal(t, big.NewInt(0).String(), output[0].(*big.Int).String())
	}

}

func TestTimerContract(t *testing.T) {
	compiledAll := compile(t, "./TestContracts.sol")
	compiled, ok := compiledAll.Contracts["./TestContracts.sol:Timer"]
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.DialContext(ctx, "wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)
	signer := types.NewLondonSigner(chainID)
	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)

	abibytes, _ := json.Marshal(compiled.Abi)
	abiparsed, err := abi.JSON(bytes.NewReader(abibytes))
	require.NoError(t, err)

	nonce, err := ec.NonceAt(ctx, holder, nil)
	require.NoError(t, err)

	ca := crypto.CreateAddress(holder, nonce)

	for i := uint64(0); i <= 5; i++ {
		header, err := ec.HeaderByNumber(ctx, nil)
		require.NoError(t, err)
		gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))

		var tx *types.Transaction
		if i == 0 {
			tx, err = types.SignNewTx(holderPK, signer, &types.DynamicFeeTx{ChainID: signer.ChainID(), Nonce: nonce + i, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: 100000,
				To:   nil,
				Data: hexutil.MustDecode("0x" + compiled.Bin),
			})
		} else {
			tx, err = types.SignNewTx(holderPK, signer, &types.DynamicFeeTx{ChainID: signer.ChainID(), Nonce: nonce + i, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: 100000,
				To:   &ca,
				Data: abiparsed.Methods["call"].ID,
			})
		}
		require.NoError(t, err)
		require.NoError(t, ec.SendTransaction(ctx, tx))
		receipt, err := bind.WaitMined(ctx, ec, tx)
		require.NoError(t, err)
		{
			header, err := ec.HeaderByNumber(ctx, receipt.BlockNumber)
			require.NoError(t, err)
			t.Log("exec", "receipt block", receipt.BlockNumber, "event block", new(big.Int).SetBytes(receipt.Logs[0].Topics[1][:]), "receipt time", header.Time, "event time", new(big.Int).SetBytes(receipt.Logs[0].Topics[2][:]))

			require.True(t, receipt.Bloom.Test(ca[:]) && receipt.Bloom.Test(abiparsed.Events["Time"].ID.Bytes()) && receipt.Bloom.Test(receipt.Logs[0].Topics[1][:]) && receipt.Bloom.Test(receipt.Logs[0].Topics[2][:]))
		}
	}
}

func TestStateContract(t *testing.T) {
	compiledAll := compile(t, "./TestContracts.sol")
	compiled, ok := compiledAll.Contracts["./TestContracts.sol:State"]
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.DialContext(ctx, "wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)
	signer := types.NewLondonSigner(chainID)
	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)

	abibytes, _ := json.Marshal(compiled.Abi)
	abiparsed, err := abi.JSON(bytes.NewReader(abibytes))
	require.NoError(t, err)

	nonce, err := ec.PendingNonceAt(ctx, holder)
	require.NoError(t, err)

	ca := crypto.CreateAddress(holder, nonce)

	for i := uint64(0); i <= 5; i++ {
		header, err := ec.HeaderByNumber(ctx, nil)
		require.NoError(t, err)
		gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))

		txdata := func() types.TxData {
			td := &types.DynamicFeeTx{ChainID: signer.ChainID(), Nonce: nonce + i, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: 100000}
			if i == 0 {
				td.To = nil
				td.Data = hexutil.MustDecode("0x" + compiled.Bin)
			} else {
				td.To = &ca
				td.Data = abiparsed.Methods["call"].ID
			}
			return td
		}()

		tx, err := types.SignNewTx(holderPK, signer, txdata)
		require.NoError(t, err)

		err = ec.SendTransaction(ctx, tx)
		require.NoError(t, err)

		receipt, err := bind.WaitMined(ctx, ec, tx)
		require.NoError(t, err)
		{
			header, err := ec.HeaderByNumber(ctx, receipt.BlockNumber)
			require.NoError(t, err)
			t.Log("exec", "receipt block", receipt.BlockNumber, "event block", new(big.Int).SetBytes(receipt.Logs[0].Topics[1][:]), "receipt time", header.Time, "event time", new(big.Int).SetBytes(receipt.Logs[0].Topics[2][:]))

			require.True(t, receipt.Bloom.Test(ca[:]) && receipt.Bloom.Test(abiparsed.Events["Time"].ID.Bytes()) && receipt.Bloom.Test(receipt.Logs[0].Topics[1][:]) && receipt.Bloom.Test(receipt.Logs[0].Topics[2][:]))
		}
	}
}
