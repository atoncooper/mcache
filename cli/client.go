package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/atoncooper/mcache/cluster"
	mnet "github.com/atoncooper/mcache/net"
	"gopkg.in/yaml.v3"
)

var (
	globalAddr        string
	globalTimeout     time.Duration
	globalPool        int
	clusterConfigPath string
	clusterCfg        cluster.Options
	useCluster        bool
)

// EnableCluster sets the cluster configuration and switches the CLI to
// multi-node mode. Call before creating any command client.
func EnableCluster(opts cluster.Options) {
	clusterCfg = opts
	useCluster = true
}

// newCmdClient returns a client that transparently uses either a direct
// connection or a cluster manager depending on whether EnableCluster was called
// or --cluster-config is provided.
func newCmdClient() (*cmdClient, error) {
	// Load cluster config from file if specified.
	if clusterConfigPath != "" {
		if err := loadClusterConfig(clusterConfigPath); err != nil {
			return nil, err
		}
		useCluster = true
	}

	direct, err := mnet.NewClient(globalAddr,
		mnet.WithPoolSize(globalPool),
		mnet.WithDialTimeout(globalTimeout),
		mnet.WithClientReadTimeout(globalTimeout),
		mnet.WithClientWriteTimeout(globalTimeout),
	)
	if err != nil {
		return nil, err
	}

	cc := &cmdClient{direct: direct}

	if useCluster {
		cm, err := cluster.New(
			cluster.WithMode(clusterCfg.Mode),
			cluster.WithNodes(clusterCfg.Nodes),
			cluster.WithSentinels(clusterCfg.Sentinels),
			cluster.WithMaster(clusterCfg.Master),
			cluster.WithSlaves(clusterCfg.Slaves),
			cluster.WithHealthCheckInterval(clusterCfg.HealthCheckInterval),
			cluster.WithHealthCheckTimeout(clusterCfg.HealthCheckTimeout),
			cluster.WithFailoverTimeout(clusterCfg.FailoverTimeout),
		)
		if err != nil {
			direct.Close()
			return nil, err
		}
		cc.cluster = cm
	}

	return cc, nil
}

// loadClusterConfig reads a YAML cluster configuration file.
func loadClusterConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read cluster config: %w", err)
	}
	var cfg clusterFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse cluster config: %w", err)
	}

	clusterCfg.Mode = cfg.Mode
	clusterCfg.Master = cfg.Master
	clusterCfg.Sentinels = cfg.Sentinels
	clusterCfg.Slaves = cfg.Slaves
	for _, n := range cfg.Nodes {
		clusterCfg.Nodes = append(clusterCfg.Nodes, cluster.NodeConfig{
			Addr:     n.Addr,
			Weight:   n.Weight,
			IsMaster: n.IsMaster,
		})
	}
	if d, err := time.ParseDuration(cfg.HealthCheckInterval); err == nil && d > 0 {
		clusterCfg.HealthCheckInterval = d
	}
	if d, err := time.ParseDuration(cfg.HealthCheckTimeout); err == nil && d > 0 {
		clusterCfg.HealthCheckTimeout = d
	}
	if d, err := time.ParseDuration(cfg.FailoverTimeout); err == nil && d > 0 {
		clusterCfg.FailoverTimeout = d
	}
	return nil
}

type clusterFileConfig struct {
	Mode                string              `yaml:"mode"`
	Nodes               []clusterNodeConfig `yaml:"nodes"`
	Sentinels           []string            `yaml:"sentinels"`
	Master              string              `yaml:"master"`
	Slaves              []string            `yaml:"slaves"`
	HealthCheckInterval string              `yaml:"health_check_interval"`
	HealthCheckTimeout  string              `yaml:"health_check_timeout"`
	FailoverTimeout     string              `yaml:"failover_timeout"`
}

type clusterNodeConfig struct {
	Addr     string `yaml:"addr"`
	Weight   int    `yaml:"weight"`
	IsMaster bool   `yaml:"is_master"`
}

func newClient() (*mnet.Client, error) {
	return mnet.NewClient(globalAddr,
		mnet.WithPoolSize(globalPool),
		mnet.WithDialTimeout(globalTimeout),
		mnet.WithClientReadTimeout(globalTimeout),
		mnet.WithClientWriteTimeout(globalTimeout),
	)
}

// cmdClient wraps either a direct single-node client or a ClusterManager.
type cmdClient struct {
	direct  *mnet.Client
	cluster *cluster.ClusterManager
}

func (c *cmdClient) Close() error {
	if c.cluster != nil {
		return c.cluster.Close()
	}
	return c.direct.Close()
}

// --- KV ---

func (c *cmdClient) Get(key string) ([]byte, error) {
	if c.cluster != nil {
		return c.cluster.Get(key)
	}
	return c.direct.Get(key)
}

func (c *cmdClient) Set(key string, value []byte, ttl time.Duration) error {
	if c.cluster != nil {
		return c.cluster.Set(key, value, ttl)
	}
	return c.direct.Set(key, value, ttl)
}

func (c *cmdClient) Del(key string) error {
	if c.cluster != nil {
		return c.cluster.Del(key)
	}
	return c.direct.Del(key)
}

func (c *cmdClient) Len() (int, error) {
	if c.cluster != nil {
		return c.cluster.Len()
	}
	return c.direct.Len()
}

func (c *cmdClient) Cleanup() (int, error) { return c.direct.Cleanup() }
func (c *cmdClient) Stats() ([]byte, error) { return c.direct.Stats() }

// --- Hash ---

func (c *cmdClient) HSet(key, field, value string) (int, error) {
	if c.cluster != nil { return c.cluster.HSet(key, field, value) }
	return c.direct.HSet(key, field, value)
}
func (c *cmdClient) HSetNX(key, field, value string) (bool, error) {
	if c.cluster != nil { return c.cluster.HSetNX(key, field, value) }
	return c.direct.HSetNX(key, field, value)
}
func (c *cmdClient) HGet(key, field string) (string, error) {
	if c.cluster != nil { return c.cluster.HGet(key, field) }
	return c.direct.HGet(key, field)
}
func (c *cmdClient) HDel(key string, fields ...string) (int, error) {
	if c.cluster != nil { return c.cluster.HDel(key, fields...) }
	return c.direct.HDel(key, fields...)
}
func (c *cmdClient) HExists(key, field string) (bool, error) {
	if c.cluster != nil { return c.cluster.HExists(key, field) }
	return c.direct.HExists(key, field)
}
func (c *cmdClient) HGetAll(key string) (map[string]string, error) {
	if c.cluster != nil { return c.cluster.HGetAll(key) }
	return c.direct.HGetAll(key)
}
func (c *cmdClient) HKeys(key string) ([]string, error) {
	if c.cluster != nil { return c.cluster.HKeys(key) }
	return c.direct.HKeys(key)
}
func (c *cmdClient) HVals(key string) ([]string, error) {
	if c.cluster != nil { return c.cluster.HVals(key) }
	return c.direct.HVals(key)
}
func (c *cmdClient) HLen(key string) (int, error) {
	if c.cluster != nil { return c.cluster.HLen(key) }
	return c.direct.HLen(key)
}
func (c *cmdClient) HStrLen(key, field string) (int, error) {
	if c.cluster != nil { return c.cluster.HStrLen(key, field) }
	return c.direct.HStrLen(key, field)
}
func (c *cmdClient) HIncrBy(key, field string, delta int64) (int64, error) {
	if c.cluster != nil { return c.cluster.HIncrBy(key, field, delta) }
	return c.direct.HIncrBy(key, field, delta)
}
func (c *cmdClient) HIncrByFloat(key, field string, delta float64) (float64, error) {
	if c.cluster != nil { return c.cluster.HIncrByFloat(key, field, delta) }
	return c.direct.HIncrByFloat(key, field, delta)
}
func (c *cmdClient) HMGet(key string, fields ...string) ([]any, error) {
	if c.cluster != nil { return c.cluster.HMGet(key, fields...) }
	return c.direct.HMGet(key, fields...)
}
func (c *cmdClient) HMSet(key string, fvPairs ...string) error {
	if c.cluster != nil { return c.cluster.HMSet(key, fvPairs...) }
	return c.direct.HMSet(key, fvPairs...)
}

// --- List ---

func (c *cmdClient) LPush(key string, elems ...string) (int, error) {
	if c.cluster != nil { return c.cluster.LPush(key, elems...) }
	return c.direct.LPush(key, elems...)
}
func (c *cmdClient) RPush(key string, elems ...string) (int, error) {
	if c.cluster != nil { return c.cluster.RPush(key, elems...) }
	return c.direct.RPush(key, elems...)
}
func (c *cmdClient) LPop(key string) (string, error) {
	if c.cluster != nil { return c.cluster.LPop(key) }
	return c.direct.LPop(key)
}
func (c *cmdClient) RPop(key string) (string, error) {
	if c.cluster != nil { return c.cluster.RPop(key) }
	return c.direct.RPop(key)
}
func (c *cmdClient) LLen(key string) (int, error) {
	if c.cluster != nil { return c.cluster.LLen(key) }
	return c.direct.LLen(key)
}
func (c *cmdClient) LRange(key string, start, stop int) ([]string, error) {
	if c.cluster != nil { return c.cluster.LRange(key, start, stop) }
	return c.direct.LRange(key, start, stop)
}
func (c *cmdClient) LIndex(key string, index int) (string, error) {
	if c.cluster != nil { return c.cluster.LIndex(key, index) }
	return c.direct.LIndex(key, index)
}
func (c *cmdClient) LSet(key string, index int, value string) error {
	if c.cluster != nil { return c.cluster.LSet(key, index, value) }
	return c.direct.LSet(key, index, value)
}
func (c *cmdClient) LRem(key string, count int, value string) (int, error) {
	if c.cluster != nil { return c.cluster.LRem(key, count, value) }
	return c.direct.LRem(key, count, value)
}
func (c *cmdClient) LTrim(key string, start, stop int) error {
	if c.cluster != nil { return c.cluster.LTrim(key, start, stop) }
	return c.direct.LTrim(key, start, stop)
}
func (c *cmdClient) LInsert(key string, before bool, pivot, value string) (int, error) {
	if c.cluster != nil { return c.cluster.LInsert(key, before, pivot, value) }
	return c.direct.LInsert(key, before, pivot, value)
}
func (c *cmdClient) BLPop(key string, timeout time.Duration) (string, error) {
	if c.cluster != nil { return c.cluster.BLPop(key, timeout) }
	return c.direct.BLPop(key, timeout)
}
func (c *cmdClient) BRPop(key string, timeout time.Duration) (string, error) {
	if c.cluster != nil { return c.cluster.BRPop(key, timeout) }
	return c.direct.BRPop(key, timeout)
}
func (c *cmdClient) LPos(key, value string, rank, count, maxLen int) ([]int, error) {
	if c.cluster != nil { return c.cluster.LPos(key, value, rank, count, maxLen) }
	return c.direct.LPos(key, value, rank, count, maxLen)
}

// --- Set ---

func (c *cmdClient) SAdd(key string, elems ...string) (int, error) {
	if c.cluster != nil { return c.cluster.SAdd(key, elems...) }
	return c.direct.SAdd(key, elems...)
}
func (c *cmdClient) SRem(key string, elems ...string) (int, error) {
	if c.cluster != nil { return c.cluster.SRem(key, elems...) }
	return c.direct.SRem(key, elems...)
}
func (c *cmdClient) SIsMember(key, elem string) (bool, error) {
	if c.cluster != nil { return c.cluster.SIsMember(key, elem) }
	return c.direct.SIsMember(key, elem)
}
func (c *cmdClient) SMembers(key string) ([]string, error) {
	if c.cluster != nil { return c.cluster.SMembers(key) }
	return c.direct.SMembers(key)
}
func (c *cmdClient) SCard(key string) (int, error) {
	if c.cluster != nil { return c.cluster.SCard(key) }
	return c.direct.SCard(key)
}
func (c *cmdClient) SPop(key string) (string, error) {
	if c.cluster != nil { return c.cluster.SPop(key) }
	return c.direct.SPop(key)
}
func (c *cmdClient) SRandMember(key string, count int) ([]string, error) {
	if c.cluster != nil { return c.cluster.SRandMember(key, count) }
	return c.direct.SRandMember(key, count)
}
func (c *cmdClient) SUnion(keys ...string) ([]string, error) {
	if c.cluster != nil { return c.cluster.SUnion(keys...) }
	return c.direct.SUnion(keys...)
}
func (c *cmdClient) SInter(keys ...string) ([]string, error) {
	if c.cluster != nil { return c.cluster.SInter(keys...) }
	return c.direct.SInter(keys...)
}
func (c *cmdClient) SDiff(keys ...string) ([]string, error) {
	if c.cluster != nil { return c.cluster.SDiff(keys...) }
	return c.direct.SDiff(keys...)
}

// --- Key management ---

func (c *cmdClient) Exists(key string) (bool, error) {
	if c.cluster != nil { return c.cluster.Exists(key) }
	return c.direct.Exists(key)
}
func (c *cmdClient) Type(key string) (string, error) {
	if c.cluster != nil { return c.cluster.Type(key) }
	b, err := c.direct.Type(key)
	if err != nil { return "", err }
	return typeToString(b), nil
}
func (c *cmdClient) Expire(key string, seconds int64) (bool, error) {
	if c.cluster != nil { return c.cluster.Expire(key, seconds) }
	return c.direct.Expire(key, seconds)
}
func (c *cmdClient) PExpire(key string, ms int64) (bool, error) {
	if c.cluster != nil { return c.cluster.PExpire(key, ms) }
	return c.direct.PExpire(key, ms)
}
func (c *cmdClient) TTL(key string) (int64, error) {
	if c.cluster != nil { return c.cluster.TTL(key) }
	return c.direct.TTL(key)
}
func (c *cmdClient) PTTL(key string) (int64, error) {
	if c.cluster != nil { return c.cluster.PTTL(key) }
	return c.direct.PTTL(key)
}
func (c *cmdClient) Persist(key string) (bool, error) {
	if c.cluster != nil { return c.cluster.Persist(key) }
	return c.direct.Persist(key)
}
func (c *cmdClient) Keys(pattern string) ([]string, error) {
	if c.cluster != nil { return c.cluster.Keys(pattern) }
	return c.direct.Keys(pattern)
}
