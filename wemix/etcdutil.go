/* etcdutil.go */

package wemix

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/membership"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/log"
	wemixapi "github.com/ethereum/go-ethereum/wemix/api"
	wemixminer "github.com/ethereum/go-ethereum/wemix/miner"
)

var (
	etcdLock = &SpinLock{0}
)

func (ma *wemixAdmin) etcdMemberExists(name, cluster string) (bool, error) {
	var node *wemixNode
	ma.lock.Lock()
	for _, i := range ma.nodes {
		if i.Name == name || i.Id == name || i.Ip == name {
			node = i
			break
		}
	}
	ma.lock.Unlock()

	if node == nil {
		return false, ethereum.NotFound
	}
	host := fmt.Sprintf("%s:%d", node.Ip, node.Port+1)

	var ss []string
	if ss = strings.Split(cluster, ","); len(ss) <= 0 {
		return false, ethereum.NotFound
	}

	for _, i := range ss {
		if j := strings.Split(i, "="); len(j) == 2 {
			u, err := url.Parse(j[1])
			if err == nil && u.Host == host {
				return true, nil
			}
		}
	}

	return false, nil
}

// fill the missing name in cluster string when a member is just added, like
// "=http://1.1.1.1:8590,wemix2=http:/1.1.1.2:8590"
func (ma *wemixAdmin) etcdFixCluster(cluster string) (string, error) {
	if ma.self == nil {
		return "", ethereum.NotFound
	}

	host := fmt.Sprintf("%s:%d", ma.self.Ip, ma.self.Port+1)

	var ss []string
	if ss = strings.Split(cluster, ","); len(ss) <= 0 {
		return "", ethereum.NotFound
	}

	var bb bytes.Buffer
	for _, i := range ss {
		if j := strings.Split(i, "="); len(j) == 2 {
			if bb.Len() > 0 {
				bb.WriteString(",")
			}

			if len(j[0]) != 0 {
				bb.WriteString(i)
			} else {
				u, err := url.Parse(j[1])
				if err != nil || u.Host != host {
					bb.WriteString(i)
				} else {
					bb.WriteString(fmt.Sprintf("%s=%s", ma.self.Name, j[1]))
				}
			}
		}
	}

	return bb.String(), nil
}

func (ma *wemixAdmin) etcdNewConfig(newCluster bool) *embed.Config {
	// LPUrls: listening peer urls
	// APUrls: advertised peer urls
	// LCUrls: listening client urls
	// LPUrls: advertised client urls
	cfg := embed.NewConfig()
	cfg.LogLevel = "error"
	cfg.Dir = ma.etcdDir
	cfg.Name = ma.self.Name
	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", "0.0.0.0", ma.self.Port+1))
	cfg.LPUrls = []url.URL{*u}
	u, _ = url.Parse(fmt.Sprintf("http://%s:%d", ma.self.Ip, ma.self.Port+1))
	cfg.APUrls = []url.URL{*u}
	u, _ = url.Parse(fmt.Sprintf("http://localhost:%d", ma.self.Port+2))
	cfg.LCUrls = []url.URL{*u}
	cfg.ACUrls = []url.URL{*u}
	if newCluster {
		cfg.ClusterState = embed.ClusterStateFlagNew
		cfg.ForceNewCluster = true
	} else {
		cfg.ClusterState = embed.ClusterStateFlagExisting
	}
	cfg.InitialCluster = fmt.Sprintf("%s=http://%s:%d", ma.self.Name,
		ma.self.Ip, ma.self.Port+1)
	cfg.InitialClusterToken = etcdClusterName
	return cfg
}

func (ma *wemixAdmin) etcdIsRunning() bool {
	return ma.etcd != nil && ma.etcdCli != nil
}

func (ma *wemixAdmin) etcdGetCluster() string {
	if !ma.etcdIsRunning() {
		return ""
	}

	var ms []*membership.Member
	ms = append(ms, ma.etcd.Server.Cluster().Members()...)
	sort.Slice(ms, func(i, j int) bool {
		return ms[i].Attributes.Name < ms[j].Attributes.Name
	})

	var bb bytes.Buffer
	for _, i := range ms {
		if bb.Len() > 0 {
			bb.WriteString(",")
		}
		bb.WriteString(fmt.Sprintf("%s=%s", i.Attributes.Name,
			i.RaftAttributes.PeerURLs[0]))
	}
	return bb.String()
}

// returns new cluster string if adding the member is successful
func (ma *wemixAdmin) etcdAddMember(name string) (string, error) {
	if !ma.etcdIsRunning() {
		return "", ErrNotRunning
	}

	if ok, _ := ma.etcdMemberExists(name, ma.etcdGetCluster()); ok {
		return ma.etcdGetCluster(), nil
	}

	var node *wemixNode
	ma.lock.Lock()
	for _, i := range ma.nodes {
		if i.Name == name || i.Enode == name || i.Id == name || i.Ip == name {
			node = i
			break
		}
	}
	ma.lock.Unlock()

	if node == nil {
		return "", ethereum.NotFound
	}

	_, err := ma.etcdCli.MemberAdd(context.Background(),
		[]string{fmt.Sprintf("http://%s:%d", node.Ip, node.Port+1)})
	if err != nil {
		log.Error("failed to add a new member",
			"name", name, "ip", node.Ip, "port", node.Port+1, "error", err)
		return "", err
	} else {
		log.Info("a new member added",
			"name", name, "ip", node.Ip, "port", node.Port+1, "error", err)
		return ma.etcdGetCluster(), nil
	}
}

// returns new cluster string if removing the member is successful
func (ma *wemixAdmin) etcdRemoveMember(name string) (string, error) {
	if !ma.etcdIsRunning() {
		return "", ErrNotRunning
	}

	var id uint64
	for _, i := range ma.etcd.Server.Cluster().Members() {
		if i.Attributes.Name == name {
			id = uint64(i.ID)
			break
		}
	}
	if id == 0 {
		id, _ = strconv.ParseUint(name, 16, 64)
		if id == 0 {
			return "", ethereum.NotFound
		}
	}

	_, err := ma.etcdCli.MemberRemove(context.Background(), id)
	if err != nil {
		return "", err
	}

	return ma.etcdGetCluster(), nil
}

func (ma *wemixAdmin) etcdMoveLeader(name string) error {
	if !ma.etcdIsRunning() {
		return ErrNotRunning
	}

	var id uint64
	for _, i := range ma.etcd.Server.Cluster().Members() {
		if i.Attributes.Name == name {
			id = uint64(i.ID)
			break
		}
	}
	if id == 0 {
		id, _ = strconv.ParseUint(name, 16, 64)
		if id == 0 {
			return ethereum.NotFound
		}
	}
	to := 1500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), to)
	err := ma.etcd.Server.MoveLeader(ctx, ma.etcd.Server.Lead(), id)
	cancel()
	return err
}

func (ma *wemixAdmin) etcdWipe() error {
	if ma.etcdIsRunning() {
		ma.etcdCli.Close()
		ma.etcd.Server.Stop()
		ma.etcd = nil
		ma.etcdCli = nil
	}

	if _, err := os.Stat(ma.etcdDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	} else {
		return os.RemoveAll(ma.etcdDir)
	}
}

func (ma *wemixAdmin) etcdInit() error {
	if ma.etcdIsRunning() {
		return ErrAlreadyRunning
	} else if ma.self == nil {
		return ErrNotRunning
	}

	cfg := ma.etcdNewConfig(true)
	etcd, err := embed.StartEtcd(cfg)
	if err != nil {
		log.Error("failed to initialize etcd", "error", err)
		return err
	} else {
		log.Info("initialized etcd server")
	}

	ma.etcd = etcd
	ma.etcdCli = v3client.New(etcd.Server)
	return nil
}

func (ma *wemixAdmin) etcdStart() error {
	if ma.etcdIsRunning() {
		return ErrAlreadyRunning
	}

	cfg := ma.etcdNewConfig(false)
	etcd, err := embed.StartEtcd(cfg)
	if err != nil {
		log.Error("failed to start etcd", "error", err)
		return err
	} else {
		log.Info("started etcd server")
	}

	ma.etcd = etcd
	ma.etcdCli = v3client.New(etcd.Server)

	// capture leader changes
	go func() {
		for {
			if !ma.etcdIsRunning() {
				break
			}
			<-etcd.Server.LeaderChangedNotify()
			log.Info("Leader changed", "serverID", ma.etcd.Server.ID(), "leader", ma.etcd.Server.Leader())
			if ma.etcd.Server.ID() == ma.etcd.Server.Leader() {
				log.Info("Feed leadership", "serverID", ma.etcd.Server.ID(), "leader", ma.etcd.Server.Leader())
				wemixminer.FeedLeadership()
			}
		}
	}()
	return nil
}

func (ma *wemixAdmin) etcdJoin_old(cluster string) error {
	if ma.etcdIsRunning() {
		return ErrAlreadyRunning
	}

	cfg := ma.etcdNewConfig(false)
	cfg.InitialCluster = cluster
	etcd, err := embed.StartEtcd(cfg)
	if err != nil {
		log.Error("failed to join etcd", "error", err)
		return err
	} else {
		log.Info("started etcd server")
	}

	ma.etcd = etcd
	ma.etcdCli = v3client.New(etcd.Server)
	return nil
}

func (ma *wemixAdmin) etcdJoin(name string) error {
	var node *wemixNode

	ma.lock.Lock()
	for _, i := range ma.nodes {
		if i.Name == name || i.Enode == name || i.Id == name || i.Ip == name {
			node = i
			break
		}
	}
	ma.lock.Unlock()

	if node == nil {
		return ethereum.NotFound
	}

	msgch := make(chan interface{}, 32)
	wemixapi.SetMsgChannel(msgch)
	defer func() {
		wemixapi.SetMsgChannel(nil)
		close(msgch)
	}()

	timer := time.NewTimer(30 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	err := admin.rpcCli.CallContext(ctx, nil, "admin_requestEtcdAddMember", &node.Id)
	cancel()
	if err != nil {
		log.Error("admin_requestEtcdAddMember failed", "id", node.Id, "error", err)
		return err
	}

	for {
		select {
		case msg := <-msgch:
			cluster, ok := msg.(string)
			if !ok {
				continue
			}

			cluster, _ = ma.etcdFixCluster(cluster)

			cfg := ma.etcdNewConfig(false)
			cfg.InitialCluster = cluster
			etcd, err := embed.StartEtcd(cfg)
			if err != nil {
				log.Error("failed to join etcd", "error", err)
				return err
			} else {
				log.Info("started etcd server")
			}

			ma.etcd = etcd
			ma.etcdCli = v3client.New(etcd.Server)
			return nil

		case <-timer.C:
			return fmt.Errorf("Timed Out")
		}
	}
}

func (ma *wemixAdmin) etcdStop() error {
	if !ma.etcdIsRunning() {
		return ErrNotRunning
	}
	if ma.etcdCli != nil {
		ma.etcdCli.Close()
	}
	if ma.etcd != nil {
		ma.etcd.Server.HardStop()
	}
	ma.etcd = nil
	ma.etcdCli = nil
	return nil
}

func (ma *wemixAdmin) etcdIsLeader() bool {
	if !ma.etcdIsRunning() {
		return false
	} else {
		return ma.etcd.Server.ID() == ma.etcd.Server.Leader()
	}
}

// returns leader id and node
func (ma *wemixAdmin) etcdLeader(locked bool) (uint64, *wemixNode) {
	if !ma.etcdIsRunning() {
		return 0, nil
	}

	lid := uint64(ma.etcd.Server.Leader())
	for _, i := range ma.etcd.Server.Cluster().Members() {
		if uint64(i.ID) == lid {
			var node *wemixNode
			if !locked {
				ma.lock.Lock()
			}
			for _, j := range ma.nodes {
				if i.Attributes.Name == j.Name {
					node = j
					break
				}
			}
			if !locked {
				ma.lock.Unlock()
			}
			return lid, node
		}
	}

	return 0, nil
}

func (ma *wemixAdmin) etcdPut(key, value string) (int64, error) {
	if !ma.etcdIsRunning() {
		return 0, ErrNotRunning
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		ma.etcd.Server.Cfg.ReqTimeout())
	defer cancel()
	resp, err := ma.etcdCli.Put(ctx, key, value)
	if err == nil {
		return resp.Header.Revision, err
	} else {
		return 0, err
	}
}

func (ma *wemixAdmin) etcdGet(key string) (string, error) {
	if !ma.etcdIsRunning() {
		return "", ErrNotRunning
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(1)*time.Second)
	defer cancel()
	rsp, err := ma.etcdCli.Get(ctx, key)
	if err != nil {
		return "", err
	} else if rsp.Count == 0 {
		return "", nil
	} else {
		var v string
		for _, kv := range rsp.Kvs {
			v = string(kv.Value)
		}
		return v, nil
	}
}

func (ma *wemixAdmin) etcdDelete(key string) error {
	if !ma.etcdIsRunning() {
		return ErrNotRunning
	}
	ctx, cancel := context.WithTimeout(context.Background(),
		ma.etcd.Server.Cfg.ReqTimeout())
	defer cancel()
	_, err := ma.etcdCli.Delete(ctx, key)
	return err
}

func (ma *wemixAdmin) etcdCompact(rev int64) error {
	if !ma.etcdIsRunning() {
		return ErrNotRunning
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		ma.etcd.Server.Cfg.ReqTimeout())
	defer cancel()
	_, err := ma.etcdCli.Compact(ctx, rev, clientv3.WithCompactPhysical())
	// WithCompactPhysical makes Compact wait until all compacted entries are
	// removed from the etcd server's storage.
	return err
}

func (ma *wemixAdmin) etcdInfo() interface{} {
	if ma.etcd == nil {
		return ErrNotRunning
	}

	getMemberInfo := func(member *etcdserverpb.Member) *map[string]interface{} {
		return &map[string]interface{}{
			"name":       member.Name,
			"id":         fmt.Sprintf("%x", member.ID),
			"clientUrls": strings.Join(member.ClientURLs, ","),
			"peerUrls":   strings.Join(member.PeerURLs, ","),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		ma.etcd.Server.Cfg.ReqTimeout())
	rsp, err := ma.etcdCli.MemberList(ctx)
	cancel()

	var ms []*etcdserverpb.Member
	if err == nil {
		ms = append(ms, rsp.Members...)
		sort.Slice(ms, func(i, j int) bool {
			return ms[i].Name < ms[j].Name
		})
	}

	var bb bytes.Buffer
	var self, leader *etcdserverpb.Member
	var members []interface{}
	for _, i := range ms {
		if i.ID == uint64(ma.etcd.Server.ID()) {
			self = i
		}
		if i.ID == uint64(ma.etcd.Server.Leader()) {
			leader = i
		}
		members = append(members, getMemberInfo(i))
		if bb.Len() > 0 {
			bb.WriteString(",")
		}
		bb.WriteString(fmt.Sprintf("%s=%s", i.Name,
			strings.Join(i.PeerURLs, ",")))
	}

	info := map[string]interface{}{
		"cluster": bb.String(),
		"members": members,
	}
	if self != nil {
		info["self"] = &map[string]interface{}{
			"name": self.Name,
			"id":   fmt.Sprintf("%x", self.ID),
		}
	}
	if leader != nil {
		info["leader"] = &map[string]interface{}{
			"name": leader.Name,
			"id":   fmt.Sprintf("%x", leader.ID),
		}
	}

	return info
}

func EtcdInit() error {
	etcdLock.Lock()
	defer etcdLock.Unlock()

	if admin == nil {
		return ErrNotRunning
	}
	return admin.etcdInit()
}

func EtcdStart() {
	if !etcdLock.TryLock() {
		return
	}
	defer etcdLock.Unlock()
	if admin == nil {
		return
	}

	admin.etcdStart()
	if !admin.etcdIsRunning() {
		// try to join a random peer
		var node *wemixNode
		admin.lock.Lock()
		if len(admin.nodes) > 0 {
			ix := rand.Int() % len(admin.nodes)
			for _, i := range admin.nodes {
				if ix <= 0 {
					node = i
					break
				}
				ix--
			}
		}
		admin.lock.Unlock()

		if node != nil && admin.isPeerUp(node.Id) {
			log.Info("Wemix", "Trying to join", node.Name)
			admin.etcdJoin(node.Name)
		}
	}
}

func EtcdAddMember(name string) (string, error) {
	etcdLock.Lock()
	defer etcdLock.Unlock()

	if admin == nil {
		return "", ErrNotRunning
	}
	return admin.etcdAddMember(name)
}

func EtcdRemoveMember(name string) (string, error) {
	etcdLock.Lock()
	defer etcdLock.Unlock()

	if admin == nil {
		return "", ErrNotRunning
	}
	return admin.etcdRemoveMember(name)
}

func EtcdMoveLeader(name string) error {
	etcdLock.Lock()
	defer etcdLock.Unlock()

	if admin == nil {
		return ErrNotRunning
	}
	return admin.etcdMoveLeader(name)
}

func EtcdJoin(name string) error {
	etcdLock.Lock()
	defer etcdLock.Unlock()

	if admin == nil {
		return ErrNotRunning
	}
	return admin.etcdJoin(name)
}

func EtcdGetWork() (string, error) {
	if admin == nil {
		return "", ErrNotRunning
	}
	return admin.etcdGet("work")
}

func EtcdDeleteWork() error {
	if admin == nil {
		return ErrNotRunning
	}
	return admin.etcdDelete("work")
}

/* EOF */
