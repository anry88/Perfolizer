//go:build darwin

package agent

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func collectRawHostSnapshot(diskPath string) rawHostSnapshot {
	snapshot := rawHostSnapshot{
		DiskPath: diskPath,
	}

	parseDarwinCPU(&snapshot)
	parseDarwinContextSwitches(&snapshot)
	parseDarwinMemory(&snapshot)
	parseDarwinSwap(&snapshot)
	parseDarwinDiskUsage(&snapshot, diskPath)
	parseDarwinDiskIOCounters(&snapshot)

	return snapshot
}

func parseDarwinCPU(snapshot *rawHostSnapshot) {
	if parseDarwinCPUFromTop(snapshot) {
		return
	}

	output, err := runCommand("sysctl", "-n", "kern.cp_time")
	if err != nil {
		return
	}

	fields := strings.Fields(output)
	if len(fields) < 5 {
		return
	}

	user := parseFloat(fields[0])
	nice := parseFloat(fields[1])
	system := parseFloat(fields[2])
	idle := parseFloat(fields[3])
	intr := parseFloat(fields[4])

	total := user + nice + system + idle + intr
	if total <= 0 {
		return
	}

	snapshot.CPUTimesAvailable = true
	snapshot.CPUUserTotal = user + nice
	snapshot.CPUSystemTotal = system + intr
	snapshot.CPUIdleTotal = idle
	snapshot.CPUTotal = total
}

func parseDarwinCPUFromTop(snapshot *rawHostSnapshot) bool {
	output, err := runCommand("top", "-l", "1", "-n", "0")
	if err != nil {
		return false
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "cpu usage:") {
			continue
		}

		rest := strings.TrimSpace(line[len("CPU usage:"):])
		parts := strings.Split(rest, ",")

		var user float64
		var system float64
		var idle float64
		var hasUser bool
		var hasSystem bool
		var hasIdle bool

		for _, part := range parts {
			value, label, ok := parseTopCPUChunk(part)
			if !ok {
				continue
			}

			switch label {
			case "user":
				user = value
				hasUser = true
			case "sys", "system":
				system = value
				hasSystem = true
			case "idle":
				idle = value
				hasIdle = true
			}
		}

		if hasIdle && (hasUser || hasSystem) {
			snapshot.CPUUsageAvailable = true
			snapshot.CPUUserPercent = user
			snapshot.CPUSystemPercent = system
			snapshot.CPUIdlePercent = idle
			return true
		}
	}

	return false
}

func parseTopCPUChunk(part string) (float64, string, bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return 0, "", false
	}

	fields := strings.Fields(part)
	if len(fields) < 2 {
		return 0, "", false
	}

	valueText := strings.TrimSuffix(strings.TrimSpace(fields[0]), "%")
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return 0, "", false
	}

	label := strings.ToLower(strings.Trim(strings.TrimSpace(fields[1]), ".,:;"))
	return value, label, true
}

func parseDarwinContextSwitches(snapshot *rawHostSnapshot) {
	output, err := runCommand("sysctl", "-n", "vm.stats.sys.v_swtch")
	if err != nil {
		return
	}

	value, err := strconv.ParseUint(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return
	}

	snapshot.HasContextSwitches = true
	snapshot.ContextSwitchesTotal = value
}

func parseDarwinMemory(snapshot *rawHostSnapshot) {
	memsizeRaw, err := runCommand("sysctl", "-n", "hw.memsize")
	if err != nil {
		return
	}
	totalBytes, err := strconv.ParseUint(strings.TrimSpace(memsizeRaw), 10, 64)
	if err != nil || totalBytes == 0 {
		return
	}

	vmStatOutput, err := runCommand("vm_stat")
	if err != nil {
		return
	}

	pageSize, metrics := parseDarwinVMStat(vmStatOutput)
	if pageSize == 0 {
		pageSize = 4096
	}

	freePages := metrics["Pages free"] + metrics["Pages speculative"]
	availablePages := freePages + metrics["Pages inactive"]
	freeBytes := freePages * pageSize
	availableBytes := availablePages * pageSize
	if availableBytes > totalBytes {
		availableBytes = totalBytes
	}

	usedBytes := uint64(0)
	if totalBytes > availableBytes {
		usedBytes = totalBytes - availableBytes
	}

	cachedBytes := metrics["File-backed pages"] * pageSize
	buffersBytes := metrics["Pages occupied by compressor"] * pageSize

	usedPercent := 0.0
	if totalBytes > 0 {
		usedPercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	snapshot.MemoryAvailable = true
	snapshot.MemoryTotalBytes = totalBytes
	snapshot.MemoryUsedBytes = usedBytes
	snapshot.MemoryFreeBytes = freeBytes
	snapshot.MemoryAvailableBytes = availableBytes
	snapshot.MemoryCachedBytes = cachedBytes
	snapshot.MemoryBuffersBytes = buffersBytes
	snapshot.MemoryUsedPercent = usedPercent

	if value, ok := metrics["Faults"]; ok {
		snapshot.HasPageFaults = true
		snapshot.PageFaultsTotal = value
	}
	if value, ok := metrics["Pageins"]; ok {
		snapshot.HasPageIn = true
		snapshot.PageInTotal = value
	}
	if value, ok := metrics["Pageouts"]; ok {
		snapshot.HasPageOut = true
		snapshot.PageOutTotal = value
	}
}

func parseDarwinSwap(snapshot *rawHostSnapshot) {
	output, err := runCommand("sysctl", "-n", "vm.swapusage")
	if err != nil {
		return
	}

	total := parseDarwinSizeField(output, "total =")
	used := parseDarwinSizeField(output, "used =")
	free := parseDarwinSizeField(output, "free =")

	if total == 0 && used == 0 && free == 0 {
		return
	}

	usedPercent := 0.0
	if total > 0 {
		usedPercent = float64(used) / float64(total) * 100
	}

	snapshot.SwapAvailable = true
	snapshot.SwapTotalBytes = total
	snapshot.SwapUsedBytes = used
	snapshot.SwapFreeBytes = free
	snapshot.SwapUsedPercent = usedPercent
}

func parseDarwinDiskUsage(snapshot *rawHostSnapshot, diskPath string) {
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

func parseDarwinDiskIOCounters(snapshot *rawHostSnapshot) {
	output, err := runCommand("ioreg", "-rd1", "-c", "IOBlockStorageDriver")
	if err != nil {
		return
	}

	var readBytes uint64
	var writeBytes uint64
	var readOps uint64
	var writeOps uint64
	var totalReadTimeNs uint64
	var totalWriteTimeNs uint64

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, `"Statistics" = {`) {
			continue
		}

		open := strings.IndexByte(line, '{')
		close := strings.LastIndexByte(line, '}')
		if open == -1 || close == -1 || close <= open {
			continue
		}

		body := line[open+1 : close]
		entries := strings.Split(body, ",")
		for _, entry := range entries {
			kv := strings.SplitN(entry, "=", 2)
			if len(kv) != 2 {
				continue
			}

			key := strings.TrimSpace(kv[0])
			key = strings.Trim(key, `"`)
			value, err := strconv.ParseUint(strings.TrimSpace(kv[1]), 10, 64)
			if err != nil {
				continue
			}

			switch key {
			case "Bytes (Read)":
				readBytes += value
			case "Bytes (Write)":
				writeBytes += value
			case "Operations (Read)":
				readOps += value
			case "Operations (Write)":
				writeOps += value
			case "Total Time (Read)":
				totalReadTimeNs += value
			case "Total Time (Write)":
				totalWriteTimeNs += value
			}
		}
	}

	if readBytes == 0 && writeBytes == 0 && readOps == 0 && writeOps == 0 {
		return
	}

	snapshot.HasDiskIOCounters = true
	snapshot.DiskReadBytesTotal = readBytes
	snapshot.DiskWriteBytesTotal = writeBytes
	snapshot.DiskReadOpsTotal = readOps
	snapshot.DiskWriteOpsTotal = writeOps

	totalNs := totalReadTimeNs + totalWriteTimeNs
	if totalNs > 0 {
		snapshot.HasDiskIOTime = true
		snapshot.DiskIOTimeSeconds = float64(totalNs) / 1_000_000_000
	}
}

func parseDarwinVMStat(output string) (uint64, map[string]uint64) {
	result := make(map[string]uint64)
	pageSize := uint64(0)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.Contains(line, "page size of") && strings.Contains(line, "bytes") {
			for _, field := range strings.Fields(line) {
				value, err := strconv.ParseUint(field, 10, 64)
				if err == nil && value > 0 {
					pageSize = value
					break
				}
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		valuePart := strings.TrimSpace(parts[1])
		valuePart = strings.TrimSuffix(valuePart, ".")
		valuePart = strings.ReplaceAll(valuePart, ".", "")
		valuePart = strings.ReplaceAll(valuePart, ",", "")

		value, err := strconv.ParseUint(valuePart, 10, 64)
		if err != nil {
			continue
		}
		result[key] = value
	}

	return pageSize, result
}

func parseDarwinSizeField(raw string, marker string) uint64 {
	idx := strings.Index(raw, marker)
	if idx == -1 {
		return 0
	}

	rest := strings.TrimSpace(raw[idx+len(marker):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0
	}

	valueStr := fields[0]
	if len(valueStr) < 2 {
		return 0
	}

	unit := valueStr[len(valueStr)-1]
	numPart := valueStr[:len(valueStr)-1]
	num, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0
	}

	switch unit {
	case 'B', 'b':
		return uint64(num)
	case 'K', 'k':
		return uint64(num * 1024)
	case 'M', 'm':
		return uint64(num * 1024 * 1024)
	case 'G', 'g':
		return uint64(num * 1024 * 1024 * 1024)
	case 'T', 't':
		return uint64(num * 1024 * 1024 * 1024 * 1024)
	default:
		return 0
	}
}

func runCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
