package test

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func TestBlackSign(t *testing.T) {
	var (
		url        string = wsurl
		startBlock int64  = 1
		endBlock   int64  = 0
	)

	// search gov proxy contract
	govCA := governanceCA(t, url)
	require.NotEqual(t, common.Address{}, govCA)
	t.Logf("gov contract: %v", govCA)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(url)
	require.NoError(t, err)

	// collect all members
	members := func() (ms []common.Address) {
		// call member length
		res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &govCA, Data: pack(t, "getMemberLength()")}, nil)
		require.NoError(t, err)

		memberLength := unpack(t, res, "uint256")[0].(*big.Int)
		t.Logf("gov members :%v", memberLength)

		// call all members
		for i := int64(1); i <= memberLength.Int64(); i++ {
			// member
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &govCA, Data: pack(t, "getMember(uint256)", big.NewInt(i))}, nil)
			require.NoError(t, err)
			m := unpack(t, data, "address")[0].(common.Address)
			ms = append(ms, m)
			t.Logf("member: %v", m)
		}
		return
	}()

	// collect all nodes
	nodes := func() (ns []common.Address) {
		// call node length
		res, err := ec.CallContract(ctx, ethereum.CallMsg{To: &govCA, Data: pack(t, "getNodeLength()")}, nil)
		require.NoError(t, err)

		nodesLength := unpack(t, res, "uint256")[0].(*big.Int).Int64()
		t.Logf("gov nodes :%v", nodesLength)

		// call all nodes
		for i := int64(1); i <= nodesLength; i++ {
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &govCA, Data: pack(t, "getNode(uint256)", big.NewInt(i))}, nil)
			require.NoError(t, err)
			output := unpack(t, data, "bytes", "bytes", "bytes", "uint256")

			name := string(output[0].([]byte))
			enode := common.Bytes2Hex(output[1].([]byte))
			ip := string(output[2].([]byte))
			port := output[3].(*big.Int)
			node := common.BytesToAddress(crypto.Keccak256(output[1].([]byte))[12:])
			t.Log(i, name, enode, ip, port, node)

			ns = append(ns, node)
		}
		require.Equal(t, len(ns), len(members))
		return
	}()

	// compair reward with member
	func() {
		// getReward
		for i := range nodes {
			data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &govCA, Data: pack(t, "getReward(uint256)", big.NewInt(int64(i)+1))}, nil)
			require.NoError(t, err)

			reward := unpack(t, data, "address")[0].(common.Address)
			require.Equal(t, members[i], reward, i)
		}
	}()

	// pair node and member
	nodesByMember := map[common.Address]common.Address{}
	for i, member := range members {
		nodesByMember[member] = nodes[i]
	}
	membersBynode := map[common.Address]common.Address{}
	for i, member := range members {
		membersBynode[nodes[i]] = member
	}

	// check events to update member
	func() {
		topics := map[common.Hash]string{
			crypto.Keccak256Hash([]byte("MemberAdded(address,address)")):           "MemberAdded",
			crypto.Keccak256Hash([]byte("MemberChanged(address,address,address)")): "MemberChanged",
			crypto.Keccak256Hash([]byte("MemberUpdated(address,address)")):         "MemberUpdated",
			crypto.Keccak256Hash([]byte("MemberRemoved(address,address)")):         "MemberRemoved",
		}

		logs, err := ec.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: nil,
			ToBlock:   nil,
			Addresses: []common.Address{govCA},
			Topics: [][]common.Hash{
				func() (slice []common.Hash) {
					for hash := range topics {
						slice = append(slice, hash)
					}
					return
				}(),
			},
		})
		require.NoError(t, err)

		for _, log := range logs {
			t.Logf("event emited: %v, block: %v", topics[log.Topics[0]], log.BlockNumber)
		}
		t.Logf("total events: %v", len(logs))
	}()

	// get all header from startblock to endblock
	chHeader := func() (ch chan *types.Header) {
		ch = make(chan *types.Header, 100000)
		worknum := int64(100)
		wg := sync.WaitGroup{}
		go func() {
			for i := int64(0); i < worknum; i++ {
				wg.Add(1)
				go func(index int64) {
					defer wg.Done()
					for i := startBlock + index; ; i += worknum {
						if endBlock > 0 && i > endBlock { // end가 0보다 크게 설정되어있으면 마지막 블록
							return
						}
						if header, err := ec.HeaderByNumber(ctx, big.NewInt(i)); err != nil {
							if err.Error() != "not found" {
								t.Logf("read header %v, error: %v", i, err)
							}
							return
						} else {
							ch <- header
						}
					}
				}(i)
			}
			wg.Wait()
			close(ch)
		}()
		return
	}()

	// check header's sig
	total, notEqual := func(ch chan *types.Header) (total, notEqual int64) {
		wg := sync.WaitGroup{}
		for header := range ch {
			wg.Add(1)
			go func(header *types.Header) {
				defer wg.Done()
				// recover
				pubkey, err := crypto.Ecrecover(header.Root.Bytes(), header.MinerNodeSig)
				require.NoError(t, err)
				got := common.BytesToAddress(crypto.Keccak256(pubkey[1:])[12:])
				require.Equal(t, common.BytesToAddress(crypto.Keccak256(header.MinerNodeId)[12:]), got)

				want, ok := nodesByMember[header.Coinbase]
				require.True(t, ok)

				if want != got {
					t.Logf("block: %v, coinbase: %v, want: %v, got: %v, got's coinbase: %v", header.Number, hexutil.Encode(header.Coinbase[:]), hexutil.Encode(want[:]), hexutil.Encode(got[:]), membersBynode[got])
					atomic.AddInt64(&notEqual, 1)
				}

				// progress
				if c := atomic.AddInt64(&total, 1); c%1000 == 0 {
					t.Logf("process block up to %v, not equal %v", c, atomic.LoadInt64(&notEqual))
				}
			}(header)
		}
		wg.Wait()
		return
	}(chHeader)

	t.Logf("all scaned blocks, total: %v, not equal: %v", total, notEqual)
}
