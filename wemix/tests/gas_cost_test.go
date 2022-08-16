package test

/*
type pending struct {
	tx       *types.Transaction
	sender   common.Address
	sendTime time.Time
	gas      uint64
	ch       chan<- interface{}
}

func TestGasTipCap(t *testing.T) {
	var multiFeebase = big.NewInt(100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	// client
	cli, err := rpc.DialContext(ctx, "wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)
	ec := ethclient.NewClient(cli)

	{
		// txpool_status
		var status map[string]hexutil.Uint
		err = cli.CallContext(ctx, &status, "txpool_status")
		require.NoError(t, err)
		for s, v := range status {
			t.Log("txpool", s, uint(v))
		}
	}

	// tx signature handler
	chainID, err := ec.ChainID(ctx)
	require.NoError(t, err)
	signer := types.NewLondonSigner(chainID)

	gasTipCap, err := ec.SuggestGasTipCap(ctx)
	require.NoError(t, err)

	updateGasFeeCap := func(head *types.Header) *big.Int {
		if head == nil {
			var err error
			head, err = ec.HeaderByNumber(ctx, nil)
			require.NoError(t, err)
		}
		return new(big.Int).Add(gasTipCap, new(big.Int).Mul(head.BaseFee, multiFeebase))
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
	for i, fromPK := range getTesters(t)[:2] {
		sendStopWg.Add(1)
		go func(i int, fromPK *ecdsa.PrivateKey) {
			defer sendStopWg.Done()

			var (
				from   = crypto.PubkeyToAddress(fromPK.PublicKey)
				to     = holder
				value  = big.NewInt(0)
				data   = bytes.Repeat([]byte{0xFF}, 0) // max : 262144
				execCh = make(chan interface{})
			)

			for {
				nonce, err := ec.NonceAt(ctx, from, nil)
				require.NoError(t, err)

				gasFeeCap := gasStation.Load().(*big.Int)
				estimateGas, err := ec.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &to, Value: value, Data: data})
				require.NoError(t, err)

				tx, err := types.SignNewTx(fromPK, signer, &types.DynamicFeeTx{ChainID: signer.ChainID(), Nonce: nonce, To: &to, GasTipCap: gasTipCap, GasFeeCap: gasFeeCap, Gas: estimateGas, Data: data})
				require.NoError(t, err)

				// send tx
				if err = ec.SendTransaction(ctx, tx); err != nil {
					log.Error("Send tx error", "sender", from, "index", i, "estimateGas", estimateGas, "gasFeeCap", gasFeeCap, "error", err)
				} else {
					sendMeter.Mark(1)
					pendingTxs.Store(tx.Hash(), &pending{tx: tx, sender: from, gas: estimateGas, sendTime: time.Now(), ch: execCh})
					atomic.AddInt64(&pendingCount, 1)
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
				block, err := ec.BlockByNumber(ctx, head.Number)
				require.NoError(t, err)

				// save gas info
				gasStation.Store(updateGasFeeCap(head))

				// collect receipts
				for _, tx := range block.Transactions() {
					if wri, ok := pendingTxs.LoadAndDelete(tx.Hash()); ok {
						wr := wri.(*pending)

						go func() {
							wr.ch <- time.After(time.Duration(rand.Int63n(1e9)))
						}()

						sender, err := signer.Sender(tx)
						require.NoError(t, err)
						require.Equal(t, wr.sender, sender)

						recvMeter.Mark(1)
						atomic.AddInt64(&pendingCount, -1)
						minedTime.UpdateSince(wr.sendTime)

						//log.Warn("TX confirmed", "tx", tx.Hash().Hex(), "sender", sender, "nonce", tx.Nonce(), "gasFeeCap", tx.GasFeeCap(), "gasTipCap", tx.GasTipCap())
					}
				}
				log.Info("Status", "block", block.Number(), "time", block.Time(), "txs", len(block.Transactions()), "gasUsed", head.GasUsed, "feebase", head.BaseFee, "gasTipCap", gasTipCap, "fees", head.Fees,
					"sendtx/persec/total", fmt.Sprintf("%.3f, %v", sendMeter.Rate1(), sendMeter.Count()), "recv/persec/total", fmt.Sprintf("%.3f, %v", recvMeter.Rate1(), recvMeter.Count()), "mining-time", minedTime.Mean()/float64(1e9),
					"pending", atomic.LoadInt64(&pendingCount))

			case err := <-sub.Err():
				assert.NoError(t, err)
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
	t.Log("all testers stopped sending txs")
}
*/
