package test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

const (
	defaultLogLevel     = "error"
	defaultDir          = "wemix.etcd123457"
	defaultIP           = "localhost"
	defaultClusterToken = "wemix"
)

func newEtcdConfig(newCluster bool, name string, port int) *embed.Config {
	cfg := embed.NewConfig()

	cfg.LogLevel = defaultLogLevel
	cfg.Dir = defaultDir + name
	cfg.Name = name

	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", "0.0.0.0", port+1))
	cfg.LPUrls = []url.URL{*u}

	u, _ = url.Parse(fmt.Sprintf("http://%s:%d", defaultIP, port+1))
	cfg.APUrls = []url.URL{*u}

	u, _ = url.Parse(fmt.Sprintf("http://localhost:%d", port+2))
	cfg.LCUrls = []url.URL{*u}
	cfg.ACUrls = []url.URL{*u}

	// if newCluster {
	// 	cfg.ClusterState = embed.ClusterStateFlagNew
	// 	cfg.ForceNewCluster = true
	// } else {
	// 	cfg.ClusterState = embed.ClusterStateFlagExisting
	// }

	cfg.InitialCluster = fmt.Sprintf("%s=http://%s:%d", name, defaultIP, port+1)
	cfg.InitialClusterToken = defaultClusterToken
	return cfg
}

func startEtcd(cfg *embed.Config) (etcd *embed.Etcd, err error) {
	if etcd, err = embed.StartEtcd(cfg); err == nil {
		select {
		case <-etcd.Server.ReadyNotify():
			log.Printf("%s, Server is ready!", cfg.Name)
		case <-time.After(10e9):
			etcd.Server.Stop()
			err = fmt.Errorf("Server took too long to start!")
		}
	}
	return
}

func putEtcd(cli *clientv3.Client, key, value string) (int64, error) {
	if res, err := cli.Put(context.Background(), key, value); err != nil {
		return 0, err
	} else {
		return res.Header.Revision, nil
	}
}

func getEtcd(cli *clientv3.Client, key string) ([]string, error) {
	if res, err := cli.Get(context.Background(), key); err != nil {
		return nil, err
	} else {
		values := make([]string, len(res.Kvs))
		for i, kv := range res.Kvs {
			values[i] = string(kv.Value)
		}
		return values, nil
	}
}

func TestETCD(t *testing.T) {
	etcd1, err := startEtcd(newEtcdConfig(false, "steve", 2000))
	require.NoError(t, err)
	defer etcd1.Server.Stop()

	cli1 := v3client.New(etcd1.Server)
	defer cli1.Close()

	// put
	{
		rev, err := putEtcd(cli1, "key5", "val3")
		require.NoError(t, err)
		t.Logf("revision: %v", rev)
	}

	// get
	{
		values, err := getEtcd(cli1, "key5")
		require.NoError(t, err)
		t.Logf("values: %v", values)
	}

	// member
	{
		members := etcd1.Server.Cluster().Members()
		sort.Slice(members, func(i, j int) bool {
			return members[i].Attributes.Name < members[j].Attributes.Name
		})

		var bb bytes.Buffer
		for _, i := range members {
			if bb.Len() > 0 {
				bb.WriteString(",")
			}
			bb.WriteString(fmt.Sprintf("%s=%s", i.Attributes.Name,
				i.RaftAttributes.PeerURLs[0]))
		}
		t.Logf("members: %v", bb.String())
	}

	// leader
	{
		lid := etcd1.Server.Leader()
		t.Logf("leader: %v", lid)
		for _, m := range etcd1.Server.Cluster().Members() {
			if m.ID == lid {
				t.Logf("leader name: %v, attributes: %v", m.Name, m.Attributes.Name)
			}
		}
	}

	// compact
	// func() {
	// 	ctx, cancel := context.WithTimeout(context.Background(), etcd.Server.Cfg.ReqTimeout())
	// 	defer cancel()

	// 	_, err = cli.Compact(ctx, 3, clientv3.WithCompactPhysical())
	// 	assert.NoError(t, err)
	// 	// WithCompactPhysical makes Compact wait until all compacted entries are
	// 	// removed from the etcd server's storage.
	// }()

	//log.Fatal(<-etcd.Err())
}

func TestPutInNode(t *testing.T) {
	endpoint := privateURL
	gov := governanceCA(t, endpoint)
	require.NotEqual(t, common.Address{}, gov)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec, err := ethclient.Dial(endpoint)
	require.NoError(t, err)

	data, err := ec.CallContract(ctx, ethereum.CallMsg{To: &gov, Data: pack(t, "getNode(uint256)", big.NewInt(1))}, nil)
	require.NoError(t, err)

	//(bytes memory name, bytes memory enode, bytes memory ip, uint port)
	output := unpack(t, data, "bytes", "bytes", "bytes", "uint256")

	name := string(output[0].([]byte))
	ip := string(output[2].([]byte))
	port := output[3].(*big.Int).Int64()

	t.Logf("name: %v, ip: %v, port: %v", name, ip, port)

	// config
	cfg := embed.NewConfig()

	cfg.LogLevel = defaultLogLevel
	cfg.Dir = defaultDir
	cfg.Name = "phx"

	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", "0.0.0.0", port+1))
	cfg.LPUrls = []url.URL{*u}

	u, _ = url.Parse(fmt.Sprintf("http://%s:%d", ip, port+1))
	cfg.APUrls = []url.URL{*u}

	u, _ = url.Parse(fmt.Sprintf("http://localhost:%d", port+2))
	cfg.LCUrls = []url.URL{*u}
	cfg.ACUrls = []url.URL{*u}

	// cfg.InitialCluster = fmt.Sprintf("%s=http://%s:%d", name, ip, port+1)
	// cfg.InitialClusterToken = defaultClusterToken

	etcd, err := startEtcd(cfg)
	require.NoError(t, err)

	cli := v3client.New(etcd.Server)
	defer cli.Close()

	// get
	{
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1)*time.Second)
		defer cancel()
		rsp, err := cli.Get(ctx, "xxx")
		require.NoError(t, err)

		if rsp.Count == 0 {
			t.Log("count is zero")
		} else {
			var v string
			for _, kv := range rsp.Kvs {
				v = string(kv.Value)
			}
			t.Log("value", v)
		}
	}

	// put
	rev, err := putEtcd(cli, "xxx", "yyy")
	require.NoError(t, err)

	t.Log("rev", rev)

}
