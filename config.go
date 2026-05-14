package mcache

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete configuration for mcache and its server.
type Config struct {
	Cache   CacheConfig   `yaml:"cache"`
	Server  ServerConfig  `yaml:"server"`
	Monitor MonitorConfig `yaml:"monitor"`
	Cluster ClusterConfig `yaml:"cluster"`
	Client  ClientConfig  `yaml:"client"`
	MBR     MBRConfig     `yaml:"mbr"`
	Raft    RaftConfig    `yaml:"raft"`
}

// CacheConfig defines cache behaviour.
type CacheConfig struct {
	Shards          int    `yaml:"shards"`
	MaxSize         int    `yaml:"max_size"`
	DefaultTTL      string `yaml:"default_ttl"`
	EvictionPolicy  string `yaml:"eviction_policy"`
	Rehasher        string `yaml:"rehasher"`
	CleanupInterval string `yaml:"cleanup_interval"`
	ObserverEnabled bool   `yaml:"observer_enabled"`
}

// ServerConfig defines the TCP server behaviour.
type ServerConfig struct {
	Address                 string        `yaml:"address"`
	Workers                 int           `yaml:"workers"`
	MaxConns                int           `yaml:"max_conns"`
	ReadTimeout             string        `yaml:"read_timeout"`
	WriteTimeout            string        `yaml:"write_timeout"`
	GracefulShutdownTimeout string        `yaml:"graceful_shutdown_timeout"`
	TLS                     TLSConfig     `yaml:"tls"`
	Auth                    AuthConfig    `yaml:"auth"`
	Logging                 LoggingConfig `yaml:"logging"`
	Metrics                 MetricsConfig `yaml:"metrics"`
	MemoryLimit             string        `yaml:"memory_limit"`
}

// TLSConfig holds TLS settings (reserved for future implementation).
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// AuthConfig holds authentication settings (reserved for future implementation).
type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// MetricsConfig holds metrics / observability settings (reserved).
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

// MonitorConfig holds the system monitor sub-package settings.
type MonitorConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
	Capacity int    `yaml:"capacity"`
}

// ClusterConfig holds distributed mode settings (reserved for future implementation).
type ClusterConfig struct {
	Mode              string   `yaml:"mode"`
	Nodes             []string `yaml:"nodes"`
	ReplicationFactor int      `yaml:"replication_factor"`
}

// RaftConfig holds Raft consensus settings.
type RaftConfig struct {
	Enabled  bool             `yaml:"enabled"`
	NodeID   uint64           `yaml:"node_id"`
	BindAddr string           `yaml:"bind_addr"`
	Peers    []RaftPeerConfig `yaml:"peers"`
}

// RaftPeerConfig describes a single Raft peer.
type RaftPeerConfig struct {
	ID   uint64 `yaml:"id"`
	Addr string `yaml:"addr"`
}

// ClientConfig holds SDK client settings.
type ClientConfig struct {
	PoolSize     int    `yaml:"pool_size"`
	DialTimeout  string `yaml:"dial_timeout"`
	ReadTimeout  string `yaml:"read_timeout"`
	WriteTimeout string `yaml:"write_timeout"`
}

// MBRMigrationConfig holds the migration executor settings.
type MBRMigrationConfig struct {
	CheckInterval       string  `yaml:"check_interval"`
	MaxMigrationTime    string  `yaml:"max_migration_time"`
	PauseOnCPUThreshold float64 `yaml:"pause_on_cpu_threshold"`
	PauseOnMemThreshold float64 `yaml:"pause_on_mem_threshold"`
	TargetLoadPerShard  int     `yaml:"target_load_per_shard"`
	MinShards           int     `yaml:"min_shards"`
	MaxShards           int     `yaml:"max_shards"`
}

// MBRConfig holds the MBR decision engine settings.
type MBRConfig struct {
	Enabled          bool    `yaml:"enabled"`
	MatrixCapacity   int     `yaml:"matrix_capacity"`
	DecisionInterval string  `yaml:"decision_interval"`
	Setpoint         float64 `yaml:"setpoint"`
	PID              struct {
		Kp float64 `yaml:"kp"`
		Ki float64 `yaml:"ki"`
		Kd float64 `yaml:"kd"`
	} `yaml:"pid"`
	Weights struct {
		MemGrowth        float64 `yaml:"mem_growth"`
		HitRate          float64 `yaml:"hit_rate"`
		NewKeys          float64 `yaml:"new_keys"`
		EvictionPressure float64 `yaml:"eviction_pressure"`
		BufferPenalty    float64 `yaml:"buffer_penalty"`
	} `yaml:"weights"`
	Migration MBRMigrationConfig `yaml:"migration"`
}

// LoadConfig reads a YAML configuration file from path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return &cfg, nil
}

// BuildOptions converts CacheConfig into mcache Options.
func (c CacheConfig) BuildOptions() (Options, error) {
	opts := NewOptions().
		WithShards(c.Shards).
		WithMaxSize(c.MaxSize).
		WithEvictionPolicy(c.EvictionPolicy).
		WithRehasher(c.Rehasher)

	if c.DefaultTTL != "" {
		d, err := time.ParseDuration(c.DefaultTTL)
		if err != nil {
			return Options{}, fmt.Errorf("invalid default_ttl: %w", err)
		}
		opts = opts.WithDefaultTTL(d)
	}
	return opts, nil
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Cache: CacheConfig{
			Shards:          16,
			MaxSize:         10000,
			DefaultTTL:      "",
			EvictionPolicy:  "lru",
			Rehasher:        "incremental",
			CleanupInterval: "",
			ObserverEnabled: true,
		},
		Server: ServerConfig{
			Address:                 ":11211",
			Workers:                 256,
			MaxConns:                100000,
			ReadTimeout:             "30s",
			WriteTimeout:            "5s",
			GracefulShutdownTimeout: "30s",
			TLS: TLSConfig{
				Enabled: false,
			},
			Auth: AuthConfig{
				Enabled: false,
			},
			Logging: LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
			Metrics: MetricsConfig{
				Enabled: false,
			},
		},
		Monitor: MonitorConfig{
			Enabled:  false,
			Interval: "5s",
			Capacity: 60,
		},
		Cluster: ClusterConfig{
			Mode:              "standalone",
			Nodes:             []string{},
			ReplicationFactor: 1,
		},
		Client: ClientConfig{
			PoolSize:     4,
			DialTimeout:  "5s",
			ReadTimeout:  "10s",
			WriteTimeout: "5s",
		},
		MBR: MBRConfig{
			Enabled:          false,
			MatrixCapacity:   60,
			DecisionInterval: "500ms",
			Setpoint:         0.60,
			PID: struct {
				Kp float64 `yaml:"kp"`
				Ki float64 `yaml:"ki"`
				Kd float64 `yaml:"kd"`
			}{Kp: 1.0, Ki: 0.1, Kd: 0.05},
			Weights: struct {
				MemGrowth        float64 `yaml:"mem_growth"`
				HitRate          float64 `yaml:"hit_rate"`
				NewKeys          float64 `yaml:"new_keys"`
				EvictionPressure float64 `yaml:"eviction_pressure"`
				BufferPenalty    float64 `yaml:"buffer_penalty"`
			}{MemGrowth: 0.35, HitRate: 0.25, NewKeys: 0.20, EvictionPressure: 0.15, BufferPenalty: 0.05},
			Migration: MBRMigrationConfig{
				CheckInterval: "100ms", MaxMigrationTime: "5m", PauseOnCPUThreshold: 0.80, PauseOnMemThreshold: 0.85, TargetLoadPerShard: 512, MinShards: 4, MaxShards: 1024,
			},
		},
		Raft: RaftConfig{
			Enabled:  false,
			NodeID:   1,
			BindAddr: ":12001",
			Peers:    []RaftPeerConfig{},
		},
	}
}
