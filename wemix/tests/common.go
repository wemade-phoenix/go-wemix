package test

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	bip39 "github.com/tyler-smith/go-bip39"
)

var (
	holderPK, _    = crypto.ToECDSA(hexutil.MustDecode("0x34cd26ea3ad7f2d8dce56cc23354571505f81768270b5d1e9ba850c04a1a9264"))
	holder         = crypto.PubkeyToAddress(holderPK.PublicKey) // 0x7fC08Fbd63254522Fd048Bde46b32B3E3CA4Dbf9
	testerMnemonic = "jaguar total program nerve merge kangaroo unit opera snap medal draw supreme"
	testerNum      = uint32(5000)
)

var (
	wsurl      = "wss://ws-twemix.phnxops.in/"
	apiurl     = "https://api.test.wemix.com"
	pnWsUrl    = "ws://20.41.113.133:8598"
	pnHttpUrl  = "http://20.41.113.133:8588"
	privateURL = "wss://ws-pwemix.phnxops.in"
)

func getTesters(t *testing.T) (ecdsaPrivKeys []*ecdsa.PrivateKey) {
	masterKey, err := hdkeychain.NewMaster(bip39.NewSeed(testerMnemonic, ""), &chaincfg.MainNetParams)
	require.NoError(t, err)
	for _, n := range []uint32{0x80000000 + 44, 0x80000000 + 60, 0x80000000 + 0, 0} {
		masterKey, err = masterKey.Child(n)
		require.NoError(t, err)
	}

	for i := uint32(0); i < testerNum; i++ {
		extendedkey, err := masterKey.Child(i)
		require.NoError(t, err)
		ecPrivKey, err := extendedkey.ECPrivKey()
		require.NoError(t, err)
		ecdsaPrivKeys = append(ecdsaPrivKeys, ecPrivKey.ToECDSA())
	}
	return
}

func getTestersAddress(t *testing.T) (testers []common.Address) {
	for _, privKey := range getTesters(t) {
		testers = append(testers, crypto.PubkeyToAddress(privKey.PublicKey))
	}
	return
}

type Contracts struct {
	Contracts map[string]struct {
		Abi           interface{}
		Bin, Metadata string
		BinRuntime    string `json:"bin-runtime"`
	}
}

func compile(t *testing.T, files ...string) (out *Contracts) {
	var stderr, stdout bytes.Buffer

	cmd := exec.Command("./solc-0.8.6", append([]string{
		"--combined-json", "bin,bin-runtime,abi,metadata",
		"--optimize",
		"@openzeppelin/=./openzeppelin-contracts/",
		"--allow-paths", "., ./, ../",
		"--",
	}, files...)...)

	cmd.Stderr, cmd.Stdout = &stderr, &stdout

	err := cmd.Run()
	require.NoError(t, err, "compile: %s", stderr.Bytes())

	err = json.Unmarshal(stdout.Bytes(), &out)
	require.NoError(t, err)
	return
}

func pack(t *testing.T, method string, datas ...interface{}) (encoded []byte) {
	s := strings.LastIndex(method, "(")
	e := strings.LastIndex(method, ")")

	var args abi.Arguments
	for _, ty := range strings.Split(method[s+1:e], ",") {
		if ty == "" {
			break
		}
		tpy, err := abi.NewType(ty, "", nil)
		require.NoError(t, err)
		args = append(args, abi.Argument{Type: tpy})
	}

	packed, err := args.Pack(datas...)
	require.NoError(t, err)
	encoded = append(crypto.Keccak256([]byte(method))[:4], packed...)
	return
}

func unpack(t *testing.T, data []byte, types ...string) (unpacked []interface{}) {
	var args abi.Arguments
	for i := 0; i < len(types); i++ {
		ty, err := abi.NewType(types[i], "", nil)
		require.NoError(t, err)
		args = append(args, abi.Argument{Type: ty})
	}

	var err error
	unpacked, err = args.Unpack(data)
	require.NoError(t, err)
	return
}

func utf8ToHash(utf string) common.Hash {
	return common.BytesToHash(common.RightPadBytes([]byte(utf), 32))
}
