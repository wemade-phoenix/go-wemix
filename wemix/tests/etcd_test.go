package test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"sort"
	"testing"
	"time"

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
	cfg.Dir = defaultDir
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

	leadershipSink := make(chan int, 1)

	for i := 0; i < 10; i++ {
		select {
		case leadershipSink <- 100:
			fmt.Println("input")
		default:
			<-leadershipSink
			fmt.Println("output")
		}
	}

	// go func() {
	// 	for {
	// 		v := <-leadershipSink
	// 		fmt.Println(v)
	// 	}
	// }()

	etcd, err := startEtcd(newEtcdConfig(false, "steve", 2000))
	require.NoError(t, err)
	defer etcd.Server.Stop()

	cli := v3client.New(etcd.Server)
	defer cli.Close()

	// put
	{
		rev, err := putEtcd(cli, "key5", "val3")
		require.NoError(t, err)
		t.Logf("revision: %v", rev)
	}

	// get
	{
		values, err := getEtcd(cli, "key5")
		require.NoError(t, err)
		t.Logf("values: %v", values)
	}

	// member
	{
		members := etcd.Server.Cluster().Members()
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
		lid := etcd.Server.Leader()
		t.Logf("leader: %v", lid)
		for _, m := range etcd.Server.Cluster().Members() {
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
