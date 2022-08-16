package test

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

func TestJournalLoad(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.DialContext(ctx, apiurl)
	require.NoError(t, err)

	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)
	signer := types.NewLondonSigner(chainID)

	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)
	log.Warn("SuggestGasTipCap", "gasTipCap", gasTipCap)

	journalpath := "./transactions.rlp"

	_, err = os.Stat(journalpath)
	require.False(t, os.IsNotExist(err), err)

	input, err := os.Open(journalpath)
	require.NoError(t, err)
	defer input.Close()

	stream := rlp.NewStream(input, 0)
	var total int

	txs := make(chan *types.Transaction, 1024)
	terminated := make(chan struct{}, 1)

	pending := map[common.Address]uint64{}

	go func() {
		defer close(terminated)

		for {
			tx, ok := <-txs
			if ok {
				sender, err := signer.Sender(tx)
				require.NoError(t, err)

				if _, ok := pending[sender]; !ok {
					nonce, err := ec.NonceAt(ctx, sender, nil)
					require.NoError(t, err)
					pending[sender] = nonce
					log.Info("Pending Tx", "hash", tx.Hash().Hex(), "sender", sender, "gas", tx.Gas(), "gasPrice", tx.GasPrice(), "gasTipCap", tx.GasTipCap(), "gasFeeCap", tx.GasFeeCap(), "nonce", nonce, "txnonce", tx.Nonce())
				}
			} else {
				return
			}
		}
	}()

	for {
		tx := new(types.Transaction)
		if err = stream.Decode(tx); err != nil {
			if err != io.EOF {
				log.Crit("Read error", "error", err)
			} else {
				log.Trace("Read all journal")
			}
			close(txs)
			break
		}

		// receipt, _ := ec.TransactionReceipt(ctx, tx.Hash())
		// if receipt == nil {
		txs <- tx
		total++
		// } else {
		// 	log.Warn("Tx found", "hash", tx.Hash())
		// }
	}

	<-terminated
	log.Info("Loaded local transaction journal", "transactions", total)

}
