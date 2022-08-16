package test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestPendingTx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := rpc.DialContext(ctx, "wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	// txpool_status
	{
		var status map[string]hexutil.Uint
		err = cli.CallContext(ctx, &status, "txpool_status")
		require.NoError(t, err)
		t.Logf("txpool_status: { pending %v, queued %v }", uint(status["pending"]), uint(status["queued"]))
	}

	// txpool_inspect
	// {
	// 	var inspect interface{}
	// 	err = cli.CallContext(ctx, &inspect, "txpool_inspect")
	// 	require.NoError(t, err)
	// 	bytes, err := json.MarshalIndent(inspect, "", "    ")
	// 	require.NoError(t, err)
	// 	t.Log("txpool_inspect:", string(bytes))
	// }

	// txpool_content
	// {
	// 	var content interface{}
	// 	err = cli.CallContext(ctx, &content, "txpool_content")
	// 	require.NoError(t, err)
	// 	bytes, err := json.MarshalIndent(content, "", "    ")
	// 	require.NoError(t, err)
	// 	t.Log("txpool_content:", string(bytes))
	// }

	// check balance and cost
	// {
	// 	cost := new(big.Int).Mul(big.NewInt(1621000), big.NewInt(100000000100))
	// 	fmt.Println(decimal.NewFromBigInt(cost, -18))
	// 	balance, err := ethclient.NewClient(cli).BalanceAt(ctx, common.HexToAddress("0x050D08D65BCA6208bF1eea75036666c39f5F3036"), nil)
	// 	require.NoError(t, err)
	// 	fmt.Println(decimal.NewFromBigInt(balance, -18))
	// }

}
