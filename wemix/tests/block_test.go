package test

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

func TestBlockSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	block, err := ec.BlockByNumber(ctx, nil)
	require.NoError(t, err)

	t.Logf("block: %v, size(KiB): %v, size(bytes): %v", block.Number(), block.Size(), block.Size()*1.024)

	// t.Logf("block: %v, size(KiB): %v, size(bytes): %v, actual: %v", block.Number(), block.Size(), len(bytes))

	// block.Header()

}

func TestBlockTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(wsurl)
	require.NoError(t, err)

	daysec := int64(86400)
	days := int64(19)

	for i := int64(0); i < days; i++ {
		var (
			from int64 = daysec * i
			to   int64 = daysec * (i + 1)
		)
		if from == 0 {
			from = 1
		}

		fromBlock, err := ec.HeaderByNumber(ctx, big.NewInt(from))
		require.NoErrorf(t, err, "block: %v", big.NewInt(from))

		toBlock, err := ec.HeaderByNumber(ctx, big.NewInt(to))
		require.NoErrorf(t, err, "block: %v", big.NewInt(to))

		t.Logf("block(%v ~ %v), time(%v ~ %v), since: %v", from, to, fromBlock.Time, toBlock.Time, toBlock.Time-fromBlock.Time)
		fmt.Println(fromBlock.UncleHash, toBlock.UncleHash, types.EmptyUncleHash)
	}
}

func TestScanBlock(t *testing.T) {
	params.ConsensusMethod = params.ConsensusETCD

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(privateURL)
	require.NoError(t, err)

	var start int64 = 1318514
	var parent common.Hash

	for i := start; ; i++ {
		header, err := ec.HeaderByNumber(ctx, big.NewInt(i))
		require.NoErrorf(t, err, "block: %v", i)

		if parent != (common.Hash{}) {
			require.Equalf(t, parent, header.ParentHash, "block: %v", header.Number)
		}
		parent = header.Hash()

	}
}

func TestRemovedEvent(t *testing.T) {
	const maxQueryRange = 10000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(wsurl)
	require.NoError(t, err)

	var (
		from, to = int64(0), int64(0)
	)

	wg := sync.WaitGroup{}

loop:
	for {
		if top, err := ec.BlockNumber(ctx); err != nil {
			t.Log("Failed to get BlockNumber", "error", err)
		} else if to = int64(top); to < from {
			break loop
		} else if to-from > (maxQueryRange - 1) {
			to = from + (maxQueryRange - 1)
		}

		wg.Add(1)
		go func(from, to int64) {
			defer wg.Done()
			if logs, err := ec.FilterLogs(ctx, ethereum.FilterQuery{FromBlock: big.NewInt(from), ToBlock: big.NewInt(to)}); err != nil {
				t.Log("Failed to filter logs", "error", err)
			} else {
				t.Log("logs", "from", from, "to", to, "num", len(logs))
				for _, l := range logs {
					if l.Removed {
						t.Log("Removed log", "block", l.BlockNumber, "hash", l.BlockHash, "tx", l.TxHash, "topic", l.Topics[0])
					}
				}
			}
		}(from, to)

		from = to + 1
	}
	wg.Wait()
}
