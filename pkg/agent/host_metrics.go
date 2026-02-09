package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type hostMetricsCollector struct {
	mu sync.Mutex

	diskPath string

	prevCPUUserTotal   float64
	prevCPUSystemTotal float64
	prevCPUIdleTotal   float64
	prevCPUTotal       float64
	hasPrevCPU         bool

	prevDiskIOTimeSeconds float64
	prevDiskAt            time.Time
	hasPrevDisk           bool
}

type hostMetricsSnapshot struct {
	CPUAvailable         bool
	CPUIdlePercent       float64
	CPUUserPercent       float64
	CPUSystemPercent     float64
	CPUUtilizationPct    float64
	ContextSwitchesTotal uint64
	HasContextSwitches   bool
	ThrottledTotal       uint64
	HasThrottledTotal    bool
	ThrottledSeconds     float64
	HasThrottledSeconds  bool

	MemoryAvailable      bool
	MemoryTotalBytes     uint64
	MemoryUsedBytes      uint64
	MemoryFreeBytes      uint64
	MemoryAvailableBytes uint64
	MemoryCachedBytes    uint64
	MemoryBuffersBytes   uint64
	MemoryUsedPercent    float64

	SwapAvailable     bool
	SwapTotalBytes    uint64
	SwapUsedBytes     uint64
	SwapFreeBytes     uint64
	SwapUsedPercent   float64
	SwapInBytesTotal  uint64
	SwapOutBytesTotal uint64

	HasPageFaults        bool
	PageFaultsTotal      uint64
	HasMajorPageFaults   bool
	MajorPageFaultsTotal uint64
	HasPageIn            bool
	PageInTotal          uint64
	HasPageOut           bool
	PageOutTotal         uint64

	DiskAvailable       bool
	DiskPath            string
	DiskTotalBytes      uint64
	DiskUsedBytes       uint64
	DiskFreeBytes       uint64
	DiskUsedPercent     float64
	DiskReadBytesTotal  uint64
	DiskWriteBytesTotal uint64
	DiskReadOpsTotal    uint64
	DiskWriteOpsTotal   uint64
	HasDiskIOTime       bool
	DiskIOTimeSeconds   float64

	HasDiskUtilization bool
	DiskUtilizationPct float64
}

type rawHostSnapshot struct {
	CPUUsageAvailable bool
	CPUIdlePercent    float64
	CPUUserPercent    float64
	CPUSystemPercent  float64

	CPUTimesAvailable bool
	CPUUserTotal      float64
	CPUSystemTotal    float64
	CPUIdleTotal      float64
	CPUTotal          float64

	HasContextSwitches   bool
	ContextSwitchesTotal uint64
	HasThrottledTotal    bool
	ThrottledTotal       uint64
	HasThrottledSeconds  bool
	ThrottledSeconds     float64

	MemoryAvailable      bool
	MemoryTotalBytes     uint64
	MemoryUsedBytes      uint64
	MemoryFreeBytes      uint64
	MemoryAvailableBytes uint64
	MemoryCachedBytes    uint64
	MemoryBuffersBytes   uint64
	MemoryUsedPercent    float64

	SwapAvailable     bool
	SwapTotalBytes    uint64
	SwapUsedBytes     uint64
	SwapFreeBytes     uint64
	SwapUsedPercent   float64
	SwapInBytesTotal  uint64
	SwapOutBytesTotal uint64

	HasPageFaults        bool
	PageFaultsTotal      uint64
	HasMajorPageFaults   bool
	MajorPageFaultsTotal uint64
	HasPageIn            bool
	PageInTotal          uint64
	HasPageOut           bool
	PageOutTotal         uint64

	DiskAvailable       bool
	DiskPath            string
	DiskTotalBytes      uint64
	DiskUsedBytes       uint64
	DiskFreeBytes       uint64
	DiskUsedPercent     float64
	HasDiskIOCounters   bool
	DiskReadBytesTotal  uint64
	DiskWriteBytesTotal uint64
	DiskReadOpsTotal    uint64
	DiskWriteOpsTotal   uint64
	HasDiskIOTime       bool
	DiskIOTimeSeconds   float64

	DiskUtilizationAvailable bool
	DiskUtilizationPercent   float64
}

func newHostMetricsCollector() *hostMetricsCollector {
	return &hostMetricsCollector{
		diskPath: defaultDiskPath(),
	}
}

func defaultDiskPath() string {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		if runtime.GOOS == "windows" {
			return `C:\`
		}
		return "/"
	}

	if runtime.GOOS != "windows" {
		return "/"
	}

	volume := filepath.VolumeName(cwd)
	if volume == "" {
		return `C:\`
	}
	return volume + `\`
}

func (c *hostMetricsCollector) collect() hostMetricsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	raw := collectRawHostSnapshot(c.diskPath)
	now := time.Now()

	snapshot := hostMetricsSnapshot{
		DiskPath: raw.DiskPath,
	}

	c.applyCPU(&snapshot, raw)
	c.applyMemory(&snapshot, raw)
	c.applyDisk(&snapshot, raw, now)

	snapshot.HasContextSwitches = raw.HasContextSwitches
	snapshot.ContextSwitchesTotal = raw.ContextSwitchesTotal
	snapshot.HasThrottledTotal = raw.HasThrottledTotal
	snapshot.ThrottledTotal = raw.ThrottledTotal
	snapshot.HasThrottledSeconds = raw.HasThrottledSeconds
	snapshot.ThrottledSeconds = raw.ThrottledSeconds

	return snapshot
}

func (c *hostMetricsCollector) applyCPU(snapshot *hostMetricsSnapshot, raw rawHostSnapshot) {
	if raw.CPUUsageAvailable {
		snapshot.CPUAvailable = true
		snapshot.CPUIdlePercent = clampPercent(raw.CPUIdlePercent)
		snapshot.CPUUserPercent = clampPercent(raw.CPUUserPercent)
		snapshot.CPUSystemPercent = clampPercent(raw.CPUSystemPercent)
		snapshot.CPUUtilizationPct = clampPercent(100 - snapshot.CPUIdlePercent)
		return
	}

	if !raw.CPUTimesAvailable {
		return
	}

	if c.hasPrevCPU {
		deltaTotal := raw.CPUTotal - c.prevCPUTotal
		if deltaTotal > 0 {
			userDelta := clampNonNegative(raw.CPUUserTotal - c.prevCPUUserTotal)
			systemDelta := clampNonNegative(raw.CPUSystemTotal - c.prevCPUSystemTotal)
			idleDelta := clampNonNegative(raw.CPUIdleTotal - c.prevCPUIdleTotal)

			snapshot.CPUAvailable = true
			snapshot.CPUUserPercent = clampPercent(userDelta / deltaTotal * 100)
			snapshot.CPUSystemPercent = clampPercent(systemDelta / deltaTotal * 100)
			snapshot.CPUIdlePercent = clampPercent(idleDelta / deltaTotal * 100)
			snapshot.CPUUtilizationPct = clampPercent(100 - snapshot.CPUIdlePercent)
		}
	}

	c.prevCPUUserTotal = raw.CPUUserTotal
	c.prevCPUSystemTotal = raw.CPUSystemTotal
	c.prevCPUIdleTotal = raw.CPUIdleTotal
	c.prevCPUTotal = raw.CPUTotal
	c.hasPrevCPU = true
}

func (c *hostMetricsCollector) applyMemory(snapshot *hostMetricsSnapshot, raw rawHostSnapshot) {
	snapshot.MemoryAvailable = raw.MemoryAvailable
	snapshot.MemoryTotalBytes = raw.MemoryTotalBytes
	snapshot.MemoryUsedBytes = raw.MemoryUsedBytes
	snapshot.MemoryFreeBytes = raw.MemoryFreeBytes
	snapshot.MemoryAvailableBytes = raw.MemoryAvailableBytes
	snapshot.MemoryCachedBytes = raw.MemoryCachedBytes
	snapshot.MemoryBuffersBytes = raw.MemoryBuffersBytes
	snapshot.MemoryUsedPercent = raw.MemoryUsedPercent

	snapshot.SwapAvailable = raw.SwapAvailable
	snapshot.SwapTotalBytes = raw.SwapTotalBytes
	snapshot.SwapUsedBytes = raw.SwapUsedBytes
	snapshot.SwapFreeBytes = raw.SwapFreeBytes
	snapshot.SwapUsedPercent = raw.SwapUsedPercent
	snapshot.SwapInBytesTotal = raw.SwapInBytesTotal
	snapshot.SwapOutBytesTotal = raw.SwapOutBytesTotal

	snapshot.HasPageFaults = raw.HasPageFaults
	snapshot.PageFaultsTotal = raw.PageFaultsTotal
	snapshot.HasMajorPageFaults = raw.HasMajorPageFaults
	snapshot.MajorPageFaultsTotal = raw.MajorPageFaultsTotal
	snapshot.HasPageIn = raw.HasPageIn
	snapshot.PageInTotal = raw.PageInTotal
	snapshot.HasPageOut = raw.HasPageOut
	snapshot.PageOutTotal = raw.PageOutTotal
}

func (c *hostMetricsCollector) applyDisk(snapshot *hostMetricsSnapshot, raw rawHostSnapshot, now time.Time) {
	snapshot.DiskAvailable = raw.DiskAvailable
	snapshot.DiskPath = raw.DiskPath
	snapshot.DiskTotalBytes = raw.DiskTotalBytes
	snapshot.DiskUsedBytes = raw.DiskUsedBytes
	snapshot.DiskFreeBytes = raw.DiskFreeBytes
	snapshot.DiskUsedPercent = raw.DiskUsedPercent

	if raw.HasDiskIOCounters {
		snapshot.DiskReadBytesTotal = raw.DiskReadBytesTotal
		snapshot.DiskWriteBytesTotal = raw.DiskWriteBytesTotal
		snapshot.DiskReadOpsTotal = raw.DiskReadOpsTotal
		snapshot.DiskWriteOpsTotal = raw.DiskWriteOpsTotal
	}

	if raw.HasDiskIOTime {
		snapshot.HasDiskIOTime = true
		snapshot.DiskIOTimeSeconds = raw.DiskIOTimeSeconds
	}

	if raw.DiskUtilizationAvailable {
		snapshot.HasDiskUtilization = true
		snapshot.DiskUtilizationPct = clampPercent(raw.DiskUtilizationPercent)
	}

	if raw.HasDiskIOTime {
		if c.hasPrevDisk && now.After(c.prevDiskAt) && raw.DiskIOTimeSeconds >= c.prevDiskIOTimeSeconds {
			elapsed := now.Sub(c.prevDiskAt).Seconds()
			if elapsed > 0 {
				deltaBusy := raw.DiskIOTimeSeconds - c.prevDiskIOTimeSeconds
				snapshot.HasDiskUtilization = true
				snapshot.DiskUtilizationPct = clampPercent(deltaBusy / elapsed * 100)
			}
		}

		c.prevDiskIOTimeSeconds = raw.DiskIOTimeSeconds
		c.prevDiskAt = now
		c.hasPrevDisk = true
	}
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampNonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
