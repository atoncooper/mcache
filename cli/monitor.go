package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/atoncooper/mcache/monitor"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Show current system resource usage",
	Long: `Collect and display a one-shot snapshot of system resource usage
including CPU, memory, disk I/O, and network statistics.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		collectors := []monitor.Collector{monitor.NewRuntime()}
		proc := monitor.NewProc()
		if _, err := proc.Collect(); err == nil {
			collectors = append(collectors, proc)
		}

		snap := &monitor.SystemSnapshot{}
		for _, c := range collectors {
			partial, err := c.Collect()
			if err != nil {
				continue
			}
			if partial.CPU != nil {
				snap.CPU = partial.CPU
			}
			if partial.Memory != nil {
				snap.Memory = partial.Memory
			}
			snap.IO = append(snap.IO, partial.IO...)
			snap.Network = append(snap.Network, partial.Network...)
		}

		printMonitorSnapshot(snap)
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
}

func printMonitorSnapshot(snap *monitor.SystemSnapshot) {
	fmt.Println("=== System Resource Monitor ===")
	fmt.Println()

	if snap.CPU != nil {
		fmt.Println("CPU:")
		fmt.Printf("  Cores:        %d\n", snap.CPU.CoreCount)
		if snap.CPU.UsagePercent > 0 {
			fmt.Printf("  Usage:        %.2f%%\n", snap.CPU.UsagePercent)
		}
		if snap.CPU.LoadAvg1 > 0 || snap.CPU.LoadAvg5 > 0 || snap.CPU.LoadAvg15 > 0 {
			fmt.Printf("  Load Avg:     %.2f / %.2f / %.2f\n", snap.CPU.LoadAvg1, snap.CPU.LoadAvg5, snap.CPU.LoadAvg15)
		}
		fmt.Println()
	}

	if snap.Memory != nil {
		fmt.Println("Memory:")
		fmt.Printf("  Total:        %s\n", humanBytes(snap.Memory.Total))
		fmt.Printf("  Used:         %s (%.2f%%)\n", humanBytes(snap.Memory.Used), snap.Memory.UsedPercent)
		fmt.Printf("  Free:         %s\n", humanBytes(snap.Memory.Free))
		fmt.Println()
	}

	if len(snap.IO) > 0 {
		fmt.Println("Disk I/O:")
		for _, io := range snap.IO {
			fmt.Printf("  %-12s  read: %s/s  write: %s/s  ops: %d/%d\n",
				io.Device,
				humanBytes(uint64(io.ReadBytesRate)),
				humanBytes(uint64(io.WriteBytesRate)),
				io.ReadOps,
				io.WriteOps,
			)
		}
		fmt.Println()
	}

	if len(snap.Network) > 0 {
		fmt.Println("Network:")
		for _, net := range snap.Network {
			fmt.Printf("  %-12s  recv: %s/s  sent: %s/s  pkts: %d/%d\n",
				net.Interface,
				humanBytes(uint64(net.RecvRate)),
				humanBytes(uint64(net.SendRate)),
				net.PacketsRecv,
				net.PacketsSent,
			)
		}
		fmt.Println()
	}

	fmt.Printf("Go Runtime: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("GoRoutines: %d\n", runtime.NumGoroutine())
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
