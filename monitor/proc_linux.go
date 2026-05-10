//go:build linux

package monitor

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcCollector reads system metrics from the Linux /proc pseudo-filesystem.
// It computes rates (CPU, IO, network) by comparing consecutive samples.
type ProcCollector struct {
	mu      sync.Mutex
	prevCPU cpuSample
	prevIO  map[string]ioSample
	prevNet map[string]netSample
	prevAt  time.Time
	inited  bool
}

type cpuSample struct {
	total uint64
	idle  uint64
}

type ioSample struct {
	readBytes  uint64
	writeBytes uint64
	readOps    uint64
	writeOps   uint64
}

type netSample struct {
	bytesSent   uint64
	bytesRecv   uint64
	packetsSent uint64
	packetsRecv uint64
}

// NewProc returns a Linux /proc collector.
func NewProc() *ProcCollector {
	return &ProcCollector{
		prevIO:  make(map[string]ioSample),
		prevNet: make(map[string]netSample),
	}
}

func (c *ProcCollector) Name() string { return "proc" }

func (c *ProcCollector) Collect() (*SystemSnapshot, error) {
	snap := &SystemSnapshot{}

	cpu, err := c.collectCPU()
	if err == nil {
		snap.CPU = cpu
	}

	mem, err := c.collectMemory()
	if err == nil {
		snap.Memory = mem
	}

	ioList, err := c.collectIO()
	if err == nil {
		snap.IO = ioList
	}

	netList, err := c.collectNet()
	if err == nil {
		snap.Network = netList
	}

	return snap, nil
}

// collectCPU parses /proc/stat and /proc/loadavg.
func (c *ProcCollector) collectCPU() (*CPUMetrics, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var user, nice, system, idle, iowait, irq, softirq, steal uint64
	var coreCount int

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				continue
			}
			user, _ = strconv.ParseUint(fields[1], 10, 64)
			nice, _ = strconv.ParseUint(fields[2], 10, 64)
			system, _ = strconv.ParseUint(fields[3], 10, 64)
			idle, _ = strconv.ParseUint(fields[4], 10, 64)
			iowait, _ = strconv.ParseUint(fields[5], 10, 64)
			irq, _ = strconv.ParseUint(fields[6], 10, 64)
			softirq, _ = strconv.ParseUint(fields[7], 10, 64)
			if len(fields) > 8 {
				steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}
		}
		if strings.HasPrefix(line, "cpu") && !strings.HasPrefix(line, "cpu ") {
			coreCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	total := user + nice + system + idle + iowait + irq + softirq + steal
	idleTotal := idle + iowait

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	m := &CPUMetrics{CoreCount: coreCount}

	if c.inited {
		deltaTotal := total - c.prevCPU.total
		deltaIdle := idleTotal - c.prevCPU.idle
		if deltaTotal > 0 {
			m.UsagePercent = float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
		}
	}
	c.prevCPU = cpuSample{total: total, idle: idleTotal}

	// loadavg
	load1, load5, load15, _ := readLoadAvg()
	m.LoadAvg1 = load1
	m.LoadAvg5 = load5
	m.LoadAvg15 = load15

	c.prevAt = now
	c.inited = true
	return m, nil
}

func readLoadAvg() (float64, float64, float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid /proc/loadavg")
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)
	return load1, load5, load15, nil
}

// collectMemory parses /proc/meminfo.
func (c *ProcCollector) collectMemory() (*MemoryMetrics, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var total, free, buffers, cached, available uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseKB(line)
		} else if strings.HasPrefix(line, "MemFree:") {
			free = parseKB(line)
		} else if strings.HasPrefix(line, "Buffers:") {
			buffers = parseKB(line)
		} else if strings.HasPrefix(line, "Cached:") {
			cached = parseKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available = parseKB(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var used uint64
	if available > 0 {
		used = total - available
	} else {
		used = total - free - buffers - cached
	}

	m := &MemoryMetrics{
		Total: total * 1024,
		Used:  used * 1024,
		Free:  free * 1024,
	}
	if m.Total > 0 {
		m.UsedPercent = float64(m.Used) / float64(m.Total) * 100
	}
	return m, nil
}

func parseKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[1], 10, 64)
	return v
}

// collectIO parses /proc/diskstats.
func (c *ProcCollector) collectIO() ([]*IOMetrics, error) {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	var results []*IOMetrics
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// Need at least 14 fields for standard diskstats format.
		if len(fields) < 14 {
			continue
		}
		device := fields[2]
		// Skip loop devices and ram disks.
		if strings.HasPrefix(device, "loop") || strings.HasPrefix(device, "ram") {
			continue
		}

		readOps, _ := strconv.ParseUint(fields[3], 10, 64)
		readSectors, _ := strconv.ParseUint(fields[5], 10, 64)
		writeOps, _ := strconv.ParseUint(fields[7], 10, 64)
		writeSectors, _ := strconv.ParseUint(fields[9], 10, 64)

		readBytes := readSectors * 512
		writeBytes := writeSectors * 512

		m := &IOMetrics{
			Device:     device,
			ReadBytes:  readBytes,
			WriteBytes: writeBytes,
			ReadOps:    readOps,
			WriteOps:   writeOps,
		}

		if prev, ok := c.prevIO[device]; ok && c.inited {
			dt := now.Sub(c.prevAt).Seconds()
			if dt > 0 {
				m.ReadBytesRate = float64(int64(readBytes)-int64(prev.readBytes)) / dt
				m.WriteBytesRate = float64(int64(writeBytes)-int64(prev.writeBytes)) / dt
				if m.ReadBytesRate < 0 {
					m.ReadBytesRate = 0
				}
				if m.WriteBytesRate < 0 {
					m.WriteBytesRate = 0
				}
			}
		}
		c.prevIO[device] = ioSample{readBytes: readBytes, writeBytes: writeBytes, readOps: readOps, writeOps: writeOps}
		results = append(results, m)
	}
	return results, scanner.Err()
}

// collectNet parses /proc/net/dev.
func (c *ProcCollector) collectNet() ([]*NetMetrics, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	var results []*NetMetrics
	scanner := bufio.NewScanner(f)
	// Skip header lines.
	for i := 0; i < 2 && scanner.Scan(); i++ {
	}
	for scanner.Scan() {
		line := scanner.Text()
		iface, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		iface = strings.TrimSpace(iface)
		// Skip loopback.
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		bytesRecv, _ := strconv.ParseUint(fields[0], 10, 64)
		packetsRecv, _ := strconv.ParseUint(fields[1], 10, 64)
		bytesSent, _ := strconv.ParseUint(fields[8], 10, 64)
		packetsSent, _ := strconv.ParseUint(fields[9], 10, 64)

		m := &NetMetrics{
			Interface:   iface,
			BytesRecv:   bytesRecv,
			PacketsRecv: packetsRecv,
			BytesSent:   bytesSent,
			PacketsSent: packetsSent,
		}

		if prev, ok := c.prevNet[iface]; ok && c.inited {
			dt := now.Sub(c.prevAt).Seconds()
			if dt > 0 {
				m.RecvRate = float64(int64(bytesRecv)-int64(prev.bytesRecv)) / dt
				m.SendRate = float64(int64(bytesSent)-int64(prev.bytesSent)) / dt
				if m.RecvRate < 0 {
					m.RecvRate = 0
				}
				if m.SendRate < 0 {
					m.SendRate = 0
				}
			}
		}
		c.prevNet[iface] = netSample{bytesRecv: bytesRecv, bytesSent: bytesSent, packetsRecv: packetsRecv, packetsSent: packetsSent}
		results = append(results, m)
	}
	return results, scanner.Err()
}
