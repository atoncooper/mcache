package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/atoncooper/mcache/cluster"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Cluster management commands",
	Long: `Connect to and manage a cluster of mcache nodes.

Use --cluster-config to specify a YAML file with node topology.
When --cluster-config is set, all KV/Hash/List/Set commands automatically
route to the correct node based on the cluster mode.

Cluster modes:
  shard        - Distribute keys across nodes using consistent hashing
  sentinel     - Monitor a master and auto-failover to replicas
  master_slave - Writes to master, reads load-balanced across slaves

Example cluster-config.yaml:
  mode: shard
  nodes:
    - addr: 127.0.0.1:11212
      weight: 1
    - addr: 127.0.0.1:11213
      weight: 1`,
}

var clusterNodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "List cluster nodes and their health status",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if clusterConfigPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --cluster-config is required")
			os.Exit(1)
		}

		start := time.Now()
		cliLogger.Info("request start", map[string]any{"cmd": "cluster nodes", "config": clusterConfigPath})

		if err := loadClusterConfig(clusterConfigPath); err != nil {
			cliLogger.Info("request done", map[string]any{"cmd": "cluster nodes", "config": clusterConfigPath, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}

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
			cliLogger.Info("request done", map[string]any{"cmd": "cluster nodes", "config": clusterConfigPath, "duration_ms": time.Since(start).Milliseconds(), "success": false, "error": err.Error()})
			exitError("%v", err)
		}
		defer cm.Close()

		nodes := cm.Nodes()
		cliLogger.Info("request done", map[string]any{"cmd": "cluster nodes", "config": clusterConfigPath, "duration_ms": time.Since(start).Milliseconds(), "success": true, "node_count": len(nodes)})

		fmt.Printf("Mode:  %s\n", clusterCfg.Mode)
		fmt.Printf("Nodes: %d\n\n", len(nodes))
		for _, n := range nodes {
			status := "healthy"
			if !n.Healthy {
				status = "unhealthy"
			}
			fmt.Printf("  %-20s  weight=%-4d  %s\n", n.Addr, n.Weight, status)
		}
	},
}

func init() {
	clusterCmd.AddCommand(clusterNodesCmd)
	rootCmd.AddCommand(clusterCmd)
}
