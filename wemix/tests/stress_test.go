package test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/stretchr/testify/require"
)

type pending struct {
	tx       *types.Transaction
	sender   common.Address
	sendTime time.Time
	gas      uint64
	callback func(errMsg string, ctx ...interface{})
}

func TestWaitMined(t *testing.T) {
	var (
		url                  = privateURL
		testerStart          = 0
		testerEnd            = testerNum
		transferValue        = big.NewInt(0)
		gasLimit      uint64 = 21000 * 500
		multiFeebase         = big.NewInt(200)
		dataSize             = 0 // max : 262144
	)

	log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// client
	ec, err := ethclient.DialContext(ctx, url)
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)

	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)
	log.Debug("Chain info", "chainID", chainID, "url", url, "gasTipCap", gasTipCap)

	updateGasFeeCap := func(head *types.Header) *big.Int {
		if head == nil {
			var err error
			head, err = ec.HeaderByNumber(ctx, nil)
			require.NoError(t, err)
		}
		return new(big.Int).Add(gasTipCap, new(big.Int).Mul(head.BaseFee, multiFeebase)) //new(big.Int).Add(gasTipCap, big.NewInt(50000000000000)) //
	}

	gasStation := atomic.Value{}
	gasStation.Store(updateGasFeeCap(nil))

	// metrics
	metrics.Enabled = true
	sendMeter := metrics.NewRegisteredMeter("tx/send", nil)
	recvMeter := metrics.NewRegisteredMeter("tx/recv", nil)
	minedTime := metrics.NewRegisteredTimer("tx/mined", nil)

	// sync
	sendStopC := make(chan struct{})
	sendStopWg := sync.WaitGroup{}
	pendingTxs := sync.Map{}
	pendingCount := int64(0)

	// sendtx
	for i, fromPK := range getTesters(t)[testerStart:testerEnd] {
		sendStopWg.Add(1)
		go func(i int, fromPK *ecdsa.PrivateKey) {
			defer sendStopWg.Done()

			var (
				from   = crypto.PubkeyToAddress(fromPK.PublicKey)
				to     = from
				data   = bytes.Repeat([]byte{0xFF}, dataSize)
				execCh = make(chan interface{})
			)
			if gasLimit == 0 {
				gasLimit, err = ec.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &to, Value: transferValue, Data: data})
				require.NoError(t, err)
			}

			logger := log.New("index", i, "from", from, "to", to, "gas", gasLimit, "datasize", len(data))

			retry := func(errMsg string, ctx ...interface{}) {
				go func() {
					<-time.After(time.Duration(rand.Int63n(1e9)))
					execCh <- struct{}{}
				}()
				if errMsg != "" {
					logger.Error(errMsg, ctx...)
				}
			}

			for {
				gasFeeCap := gasStation.Load().(*big.Int)

				if nonce, err := ec.NonceAt(ctx, from, nil); err != nil {
					retry("Call NonceAt error", "error", err)
				} else if tx, err := types.SignNewTx(fromPK, types.NewLondonSigner(chainID), &types.DynamicFeeTx{ChainID: chainID, Nonce: nonce, To: &to, Value: transferValue, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: gasLimit, Data: data}); err != nil {
					retry("Sign new tx error", "error", err)
				} else if err = ec.SendTransaction(ctx, tx); err != nil {
					retry("Send tx error", "gasFeeCap", gasFeeCap, "nonce", nonce, "dataSize", len(data), "error", err)
				} else {
					sendMeter.Mark(1)
					atomic.AddInt64(&pendingCount, 1)
					pendingTxs.Store(tx.Hash(), &pending{tx: tx, sender: from, gas: gasLimit, sendTime: time.Now(), callback: retry})
				}

				select {
				case <-sendStopC:
					return
				case <-execCh:
				}
			}
		}(i, fromPK)
	}

	go func() {
		headerCh := make(chan *types.Header, 100)
		sub, err := ec.SubscribeNewHead(ctx, headerCh)
		require.NoError(t, err)

		defer sub.Unsubscribe()

		for {
			select {
			case head := <-headerCh:
				trycount := 0
			retry:
				block, err := ec.BlockByNumber(ctx, head.Number)
				if err != nil {
					log.Error("Failed to call block", "block", head.Number, "trycount", trycount, "error", err)
					trycount++
					goto retry
				}

				// save gas info
				gasStation.Store(updateGasFeeCap(head))

				// collect receipts
				txSent := 0
				for _, tx := range block.Transactions() {
					if wri, ok := pendingTxs.LoadAndDelete(tx.Hash()); ok {
						wr := wri.(*pending)

						wr.callback("")
						recvMeter.Mark(1)
						atomic.AddInt64(&pendingCount, -1)
						minedTime.UpdateSince(wr.sendTime)
						txSent++
						//log.Warn("TX confirmed", "tx", tx.Hash().Hex(), "sender", sender, "nonce", tx.Nonce(), "gasFeeCap", tx.GasFeeCap(), "gasTipCap", tx.GasTipCap())
					}
				}
				log.Info("Status", "block", block.Number(), "time", block.Time(), "tx-sent", txSent, "txs", len(block.Transactions()), "gasUsed", head.GasUsed, "feebase", head.BaseFee, "gasTipCap", gasTipCap, "fees", head.Fees,
					"send/persec", fmt.Sprintf("%v, %.3f", sendMeter.Count(), sendMeter.Rate1()), "recv/persec", fmt.Sprintf("%v, %.3f", recvMeter.Count(), recvMeter.Rate1()), "mining-time", minedTime.Mean()/float64(1e9),
					"pending", atomic.LoadInt64(&pendingCount))

			case err := <-sub.Err():
				log.Crit("Subscribe new head error", "error", err)
				return
			}
		}
	}()

	abortC := make(chan os.Signal, 1)
	signal.Notify(abortC, os.Interrupt)
	<-abortC

	t.Log("attempts to stop all testers' sending txs")

	close(sendStopC)

	sendStopWg.Wait()
	t.Log("all testers stop sending txs")
}
