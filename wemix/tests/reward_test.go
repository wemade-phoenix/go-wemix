package test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

type reward struct {
	Addr   common.Address `json:"addr"`
	Reward *big.Int       `json:"reward"`
}

func (r *reward) String() string {
	return fmt.Sprintf("%v:%v", hexutil.Encode(r.Addr[:]), r.Reward)
}

func TestRewardDecode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	blockNumber := big.NewInt(10000)

	header, err := ec.HeaderByNumber(ctx, blockNumber)
	require.NoError(t, err)

	// unmarshal header.Rewards
	var rewardsJSON []reward
	if len(header.Rewards) > 0 {
		err = json.Unmarshal(header.Rewards, &rewardsJSON)
		require.NoError(t, err)
	}

	// RLP encode
	rlpBytes, err := rlp.EncodeToBytes(rewardsJSON)
	require.NoError(t, err)

	// RLP decode
	var rewardsRLP []reward
	err = rlp.DecodeBytes(rlpBytes, &rewardsRLP)
	require.NoError(t, err)

	require.Equal(t, len(rewardsJSON), len(rewardsRLP))

	for i := 0; i < len(rewardsJSON); i++ {
		require.True(t, reflect.DeepEqual(rewardsJSON[i], rewardsRLP[i]))
		t.Logf("%vth reward, json:%v, rlp:%v", i, rewardsJSON[i], rewardsRLP[i])
	}
	t.Logf("rlp size: %v, json size: %v, rewards size: %v", len(rlpBytes), len(header.Rewards), len(rewardsJSON))
}

func TestRewardSerialize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial("wss://ws-twemix.phnxops.in/")
	require.NoError(t, err)

	blockNumber := big.NewInt(10000)

	header, err := ec.HeaderByNumber(ctx, blockNumber)
	require.NoError(t, err)

	// unmarshal header.Rewards
	var rewardsJSON []reward
	if len(header.Rewards) > 0 {
		err = json.Unmarshal(header.Rewards, &rewardsJSON)
		require.NoError(t, err)
	}

	serialize := []byte{}

	for _, r := range rewardsJSON {
		serialize = append(serialize, r.Addr[:]...)
		serialize = append(serialize, common.LeftPadBytes(r.Reward.Bytes(), 16)...)
	}

	var rewardsUnpacked []reward
	for i := 0; i < len(serialize); i += 36 {
		data := serialize[i : i+36]

		rewardsUnpacked = append(rewardsUnpacked, reward{
			common.BytesToAddress(data[:20]),
			new(big.Int).SetBytes(data[20:]),
		})
	}

	// 확인
	for i := 0; i < len(rewardsJSON); i++ {
		require.True(t, reflect.DeepEqual(rewardsJSON[i], rewardsUnpacked[i]))
		t.Logf("%vth reward, json:%v, serialize:%v", i, rewardsJSON[i], rewardsUnpacked[i])
	}

	t.Logf("serialze size: %v, json size: %v, rewards size: %v", len(serialize), len(header.Rewards), len(rewardsJSON))

}
