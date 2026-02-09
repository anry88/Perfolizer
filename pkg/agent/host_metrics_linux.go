//go:build linux

package agent

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func collectRawHostSnapshot(diskPath string) rawHostSnapshot {
	snapshot := rawHostSnapshot{
		DiskPath: diskPath,
	}

	parseLinuxProcStat(&snapshot)
	parseLinuxMemory(&snapshot)
	parseLinuxDiskUsage(&snapshot, diskPath)
	parseLinuxDiskIOCounters(&snapshot)
	parseLinuxThrottling(&snapshot)

	return snapshot
}

func parseLinuxProcStat(snapshot *rawHostSnapshot) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				continue
			}

			user := parseFloat(fields[1])
			nice := parseFloat(fields[2])
			system := parseFloat(fields[3])
			idle := parseFloat(fields[4])
			iowait := parseFloat(fields[5])
			irq := parseFloat(fields[6])
			softirq := parseFloat(fields[7])

			var total float64
			for i := 1; i < len(fields); i++ {
				total += parseFloat(fields[i])
			}

			snapshot.CPUTimesAvailable = true
			snapshot.CPUUserTotal = user + nice
			snapshot.CPUSystemTotal = system + irq + softirq
			snapshot.CPUIdleTotal = idle + iowait
			snapshot.CPUTotal = total
			continue
		}

		if strings.HasPrefix(line, "ctxt ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				continue
			}
			snapshot.HasContextSwitches = true
			snapshot.ContextSwitchesTotal = value
		}
	}
}

func parseLinuxMemory(snapshot *rawHostSnapshot) {
	memInfo, err := parseLinuxKeyValueFile("/proc/meminfo")
	if err == nil {
		total := memInfo["MemTotal"] * 1024
		free := memInfo["MemFree"] * 1024
		available := memInfo["MemAvailable"] * 1024
		cached := memInfo["Cached"] * 1024
		buffers := memInfo["Buffers"] * 1024

		if available == 0 {
			available = free
		}

		used := uint64(0)
		if total > available {
			used = total - available
		}

		usedPercent := 0.0
		if total > 0 {
			usedPercent = float64(used) / float64(total) * 100
		}

		snapshot.MemoryAvailable = total > 0
		snapshot.MemoryTotalBytes = total
		snapshot.MemoryUsedBytes = used
		snapshot.MemoryFreeBytes = free
		snapshot.MemoryAvailableBytes = available
		snapshot.MemoryCachedBytes = cached
		snapshot.MemoryBuffersBytes = buffers
		snapshot.MemoryUsedPercent = usedPercent

		swapTotal := memInfo["SwapTotal"] * 1024
		swapFree := memInfo["SwapFree"] * 1024
		swapUsed := uint64(0)
		if swapTotal > swapFree {
			swapUsed = swapTotal - swapFree
		}

		swapUsedPercent := 0.0
		if swapTotal > 0 {
			swapUsedPercent = float64(swapUsed) / float64(swapTotal) * 100
		}

		snapshot.SwapAvailable = true
		snapshot.SwapTotalBytes = swapTotal
		snapshot.SwapUsedBytes = swapUsed
		snapshot.SwapFreeBytes = swapFree
		snapshot.SwapUsedPercent = swapUsedPercent
	}

	vmstat, err := parseLinuxKeyValueFile("/proc/vmstat")
	if err == nil {
		if value, ok := vmstat["pgfault"]; ok {
			snapshot.HasPageFaults = true
			snapshot.PageFaultsTotal = value
		}
		if value, ok := vmstat["pgmajfault"]; ok {
			snapshot.HasMajorPageFaults = true
			snapshot.MajorPageFaultsTotal = value
		}
		if value, ok := vmstat["pgpgin"]; ok {
			snapshot.HasPageIn = true
			snapshot.PageInTotal = value
		}
		if value, ok := vmstat["pgpgout"]; ok {
			snapshot.HasPageOut = true
			snapshot.PageOutTotal = value
		}
		if value, ok := vmstat["pswpin"]; ok {
			snapshot.SwapInBytesTotal = value * uint64(os.Getpagesize())
		}
		if value, ok := vmstat["pswpout"]; ok {
			snapshot.SwapOutBytesTotal = value * uint64(os.Getpagesize())
		}
	}
}

func parseLinuxDiskUsage(snapshot *rawHostSnapshot, diskPath string) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(diskPath, &fs); err != nil {
		return
	}

	total := fs.Blocks * uint64(fs.Bsize)
	free := fs.Bavail * uint64(fs.Bsize)
	used := uint64(0)
	if total > free {
		used = total - free
	}

	usedPercent := 0.0
	if total > 0 {
		usedPercent = float64(used) / float64(total) * 100
	}

	snapshot.DiskAvailable = true
	snapshot.DiskTotalBytes = total
	snapshot.DiskFreeBytes = free
	snapshot.DiskUsedBytes = used
	snapshot.DiskUsedPercent = usedPercent
}

func parseLinuxDiskIOCounters(snapshot *rawHostSnapshot) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return
	}
	defer file.Close()

	var readOps uint64
	var writeOps uint64
	var readSectors uint64
	var writeSectors uint64
	var ioTimeMs uint64
	seen := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}

		name := fields[2]
		if !includeLinuxDiskDevice(name) {
			continue
		}

		readOps += parseUint(fields[3])
		readSectors += parseUint(fields[5])
		writeOps += parseUint(fields[7])
		writeSectors += parseUint(fields[9])
		ioTimeMs += parseUint(fields[12])
		seen = true
	}

	if !seen {
		return
	}

	snapshot.HasDiskIOCounters = true
	snapshot.DiskReadOpsTotal = readOps
	snapshot.DiskWriteOpsTotal = writeOps
	snapshot.DiskReadBytesTotal = readSectors * 512
	snapshot.DiskWriteBytesTotal = writeSectors * 512
	snapshot.HasDiskIOTime = true
	snapshot.DiskIOTimeSeconds = float64(ioTimeMs) / 1000
}

func parseLinuxThrottling(snapshot *rawHostSnapshot) {
	paths := []string{
		"/sys/fs/cgroup/cpu.stat",
		"/sys/fs/cgroup/cpu/cpu.stat",
	}

	for _, path := range paths {
		stats, err := parseLinuxKeyValueFile(path)
		if err != nil {
			continue
		}

		if value, ok := stats["nr_throttled"]; ok {
			snapshot.HasThrottledTotal = true
			snapshot.ThrottledTotal = value
		}
		if value, ok := stats["throttled_usec"]; ok {
			snapshot.HasThrottledSeconds = true
			snapshot.ThrottledSeconds = float64(value) / 1_000_000
			return
		}
		if value, ok := stats["throttled_time"]; ok {
			snapshot.HasThrottledSeconds = true
			snapshot.ThrottledSeconds = float64(value) / 1_000_000_000
			return
		}
	}
}

func parseLinuxKeyValueFile(path string) (map[string]uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		result[key] = value
	}
	return result, scanner.Err()
}

func includeLinuxDiskDevice(name string) bool {
	if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
		return false
	}

	if strings.HasPrefix(name, "nvme") && strings.Contains(name, "p") {
		return false
	}

	if strings.HasPrefix(name, "mmcblk") && strings.Contains(name, "p") {
		return false
	}

	if strings.HasPrefix(name, "sd") ||
		strings.HasPrefix(name, "hd") ||
		strings.HasPrefix(name, "vd") ||
		strings.HasPrefix(name, "xvd") ||
		strings.HasPrefix(name, "nvme") ||
		strings.HasPrefix(name, "mmcblk") ||
		strings.HasPrefix(name, "dm-") ||
		strings.HasPrefix(name, "md") {
		return true
	}

	return false
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseUint(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
