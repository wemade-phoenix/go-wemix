package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestGovMember(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(wsurl)
	require.NoError(t, err)

	blockNumber := big.NewInt(40)

	block, err := ec.BlockByNumber(ctx, blockNumber)
	require.NoError(t, err)

	// genesis coinbase
	var (
		gc                  = block.Coinbase()
		IMPLEMENTATION_SLOT = common.HexToHash("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc")
	)

	// search gov proxy contract
	gov := func() common.Address {
		govExpected := map[common.Address]bool{}
		for i := uint64(0); i < 100; i++ {
			ca := crypto.CreateAddress(gc, i)

			_, err := ec.CallContract(ctx, ethereum.CallMsg{To: &ca, Data: pack(t, "getMemberLength()")}, nil)
			if err != nil {
				if err.Error() == vm.ErrExecutionReverted.Error() {
					continue
				} else {
					require.NoError(t, err)
				}
			} else {
				govExpected[ca] = true
			}

			// check proxy
			res, err := ec.StorageAt(ctx, ca, IMPLEMENTATION_SLOT, nil)
			if err != nil {
				continue
			}
			impl := common.BytesToAddress(res)
			if govExpected[impl] {
				return ca
			}
		}
		return common.Address{}
	}()
	require.NotEqual(t, common.Address{}, gov)
	t.Logf("gov contract: %v", gov)

	// all members
	members := make([]common.Address, 0)
	{
		// call member length
		res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getMemberLength()")}, nil)
		require.NoError(t, err)

		memberLength := unpack(t, res, "uint256")[0].(*big.Int)
		t.Logf("gov members :%v", memberLength)

		// call all members
		for i := int64(1); i <= memberLength.Int64(); i++ {
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getMember(uint256)", big.NewInt(i))}, nil)
			require.NoError(t, err)
			members = append(members, unpack(t, data, "address")[0].(common.Address))
			t.Logf("%vth member is %v", i, members[i-1])
		}
	}

	// all nodes
	type nodeInfo struct {
		name  string
		enode string
		ip    string
		port  *big.Int
		node  common.Address
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
			nodes = append(nodes, &nodeInfo{
				name:  string(output[0].([]byte)),
				enode: common.Bytes2Hex(output[1].([]byte)),
				ip:    string(output[2].([]byte)),
				port:  output[3].(*big.Int),
				node: func(enode []byte) common.Address {
					if len(enode) > 0 {
						return common.BytesToAddress(crypto.Keccak256(output[1].([]byte))[12:])
					}
					return common.Address{}
				}(output[1].([]byte)),
			})
			t.Logf("%vth node is %v", i, nodes[i-1].node)
		}
	}

	require.Equal(t, len(nodes), len(members))

	// pair node and member
	nodeMemberMap := map[common.Address]common.Address{}
	for i, member := range members {
		nodeMemberMap[member] = nodes[i].node
	}

	// check block's coinbase and signer with pairs got from gov contract
	for i := int64(549898); i < 549998; i++ {
		block, err := ec.BlockByNumber(ctx, big.NewInt(i))
		require.NoError(t, err)

		// recover
		pubkey, err := crypto.Ecrecover(block.Root().Bytes(), block.Header().MinerNodeSig)
		require.NoError(t, err)
		got := common.BytesToAddress(crypto.Keccak256(pubkey[1:])[12:])
		require.Equal(t, common.BytesToAddress(crypto.Keccak256(block.MinerNodeId())[12:]), got)

		want := nodeMemberMap[block.Coinbase()]

		require.Equalf(t, want, got, "block: %v, coinbase: %v, want: %v, got: %v", i, block.Coinbase().Hex(), want, got)
	}

}
