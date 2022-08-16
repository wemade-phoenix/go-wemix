package test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestAdminAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := rpc.DialContext(ctx, wsurl)
	require.NoError(t, err)

	var result map[string]interface{}
	err = cli.CallContext(ctx, &result, "admin_wemixInfo")
	require.NoError(t, err)

	//nodes := result["nodes"]

	bytes, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)

	t.Logf("admin_wemixInfo: %v", string(bytes))

}
