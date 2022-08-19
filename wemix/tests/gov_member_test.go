package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestGovMember(t *testing.T) {
	url := privateURL

	// search gov proxy contract
	gov := governanceCA(t, url)
	require.NotEqual(t, common.Address{}, gov)
	t.Logf("gov contract: %v", gov)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(url)
	require.NoError(t, err)

	// coinbase
	coinbases := make([]common.Address, 0)
	{
		// call member length
		res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getMemberLength()")}, nil)
		require.NoError(t, err)

		memberLength := unpack(t, res, "uint256")[0].(*big.Int)
		t.Logf("gov members :%v", memberLength)

		// call all members
		for i := int64(1); i <= memberLength.Int64(); i++ {
			// member
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getMember(uint256)", big.NewInt(i))}, nil)
			require.NoError(t, err)
			coinbase := unpack(t, data, "address")[0].(common.Address)
			coinbases = append(coinbases, coinbase)
			t.Logf("coinbase: %v", coinbase)
		}
	}

	// all nodes
	type nodeInfo struct {
		name  string
		enode string
		ip    string
		port  *big.Int
		miner common.Address
	}
	nodes := make([]*nodeInfo, 0)
	{
		// call node length
		res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getNodeLength()")}, nil)
		require.NoError(t, err)

		nodesLength := unpack(t, res, "uint256")[0].(*big.Int).Int64()
		t.Logf("gov nodes :%v", nodesLength)

		// call all nodes
		for i := int64(1); i <= nodesLength; i++ {
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getNode(uint256)", big.NewInt(i))}, nil)
			require.NoError(t, err)

			//(bytes memory name, bytes memory enode, bytes memory ip, uint port)
			output := unpack(t, data, "bytes", "bytes", "bytes", "uint256")

			node := &nodeInfo{
				name:  string(output[0].([]byte)),
				enode: common.Bytes2Hex(output[1].([]byte)),
				ip:    string(output[2].([]byte)),
				port:  output[3].(*big.Int),
				miner: func(enode []byte) common.Address {
					if len(enode) > 0 {
						return common.BytesToAddress(crypto.Keccak256(output[1].([]byte))[12:])
					}
					return common.Address{}
				}(output[1].([]byte)),
			}
			t.Log(i, node.name, node.enode, node.ip, node.port, node.miner)

			nodes = append(nodes, node)
		}

		// getReward
		for i := int64(1); i <= nodesLength; i++ {
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getReward(uint256)", big.NewInt(i))}, nil)
			require.NoError(t, err)

			reward := unpack(t, data, "address")[0].(common.Address)
			require.Equal(t, coinbases[i-1], reward, i)
		}
	}

	require.Equal(t, len(nodes), len(coinbases))

	// pair node and member
	coinbaseMemberMap := map[common.Address]*nodeInfo{}
	for i, coinbase := range coinbases {
		coinbaseMemberMap[coinbase] = nodes[i]
	}

	// check block's coinbase and signer with pairs got from gov contract
	for i := int64(1); i < 100; i++ {
		header, err := ec.HeaderByNumber(ctx, big.NewInt(i))
		require.NoError(t, err)

		// recover
		pubkey, err := crypto.Ecrecover(header.Root.Bytes(), header.MinerNodeSig)
		require.NoError(t, err)
		got := common.BytesToAddress(crypto.Keccak256(pubkey[1:])[12:])
		require.Equal(t, common.BytesToAddress(crypto.Keccak256(header.MinerNodeId)[12:]), got)

		want, ok := coinbaseMemberMap[header.Coinbase]
		require.True(t, ok)

		if want.miner != got {
			t.Logf("block: %v, coinbase: %v, want: %v, got: %v", i, hexutil.Encode(header.Coinbase[:]), hexutil.Encode(want.miner[:]), hexutil.Encode(got[:]))
		}

		//require.Equalf(t, want.miner, got, "block: %v, coinbase: %v, want: %v, got: %v", i, hexutil.Encode(header.Coinbase[:]), hexutil.Encode(want.miner[:]), hexutil.Encode(got[:]))

		if i%1000 == 0 {
			t.Logf("read up to %v block, ", i)
		}
	}

}
