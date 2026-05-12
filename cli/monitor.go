package cli

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	mnet "github.com/atoncooper/mcache/net"
)

var monitorWatch time.Duration

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Show server process resource usage",
	Long: `Display a one-shot snapshot of the mcache server's process-level
resource usage including memory, goroutines, network I/O, and cache stats.
Use --watch to refresh continuously.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if monitorWatch > 0 {
			ticker := time.NewTicker(monitorWatch)
			defer ticker.Stop()
			for {
				printMonitor()
				fmt.Println(strings.Repeat("-", 50))
				<-ticker.C
			}
		} else {
			printMonitor()
		}
	},
}

func init() {
	monitorCmd.Flags().DurationVarP(&monitorWatch, "watch", "w", 0, "refresh interval (e.g. 2s, 5s)")
	rootCmd.AddCommand(monitorCmd)
}

func printMonitor() {
	client, err := newCmdClient()
	if err != nil {
		exitError("%v", err)
	}
	defer client.Close()

	data, err := client.Stats()
	if err != nil {
		exitError("%v", err)
	}

	var stats mnet.ServerStats
	if err := json.Unmarshal(data, &stats); err != nil {
		exitError("decode stats: %v", err)
	}

	uptime := time.Duration(stats.UptimeMs) * time.Millisecond

	fmt.Println("=== mcache Process Monitor ===")
	fmt.Printf("Uptime:      %s\n", uptime.Round(time.Second))
	fmt.Printf("Go Version:  %s %s/%s\n", stats.GoVersion, stats.OS, stats.Arch)
	fmt.Println()

	// Memory section
	fmt.Println("Memory:")
	if stats.MemoryLimit > 0 {
		pct := float64(stats.CacheMemory) / float64(stats.MemoryLimit) * 100
		bar := renderBar(pct, 20)
		fmt.Printf("  Heap Used:    %s / %s  %s  %.1f%%\n",
			humanBytes(stats.CacheMemory), humanBytes(stats.MemoryLimit), bar, pct)
	} else {
		fmt.Printf("  Heap Used:    %s (no limit set)\n", humanBytes(stats.CacheMemory))
	}
	fmt.Printf("  Goroutines:   %d\n", stats.Goroutines)
	fmt.Println()

	// Cache section
	fmt.Println("Cache:")
	fmt.Printf("  Entries:      %d\n", stats.CacheEntries)
	fmt.Println()

	// Network I/O section
	fmt.Println("Network I/O:")
	fmt.Printf("  Connections:  %d (peak %d)\n", stats.Connections, stats.PeakConns)
	fmt.Printf("  Requests:     %d total\n", stats.TotalRequests)
	fmt.Printf("  Bytes Read:   %s\n", humanBytes(stats.BytesRead))
	fmt.Printf("  Bytes Written:%s\n", humanBytes(stats.BytesWritten))
	if stats.UptimeMs > 0 {
		secs := float64(stats.UptimeMs) / 1000.0
		fmt.Printf("  Read Rate:    %s/s\n", humanBytes(uint64(float64(stats.BytesRead)/secs)))
		fmt.Printf("  Write Rate:   %s/s\n", humanBytes(uint64(float64(stats.BytesWritten)/secs)))
		fmt.Printf("  Req Rate:     %.1f/s\n", float64(stats.TotalRequests)/secs)
	}
	fmt.Println()

	// Local CLI process info
	var localMS runtime.MemStats
	runtime.ReadMemStats(&localMS)
	fmt.Println("CLI Process (local):")
	fmt.Printf("  Heap Used:    %s\n", humanBytes(localMS.Alloc))
	fmt.Printf("  Heap Sys:     %s\n", humanBytes(localMS.Sys))
	fmt.Printf("  Goroutines:   %d\n", runtime.NumGoroutine())
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

// renderBar draws an ASCII progress bar of width blocks.
func renderBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return "[" + strings.Repeat("=", filled) + strings.Repeat("-", empty) + "]"
}
