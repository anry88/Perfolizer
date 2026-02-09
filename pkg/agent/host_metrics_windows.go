//go:build windows

package agent

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32DLL              = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes       = kernel32DLL.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32DLL.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceExW  = kernel32DLL.NewProc("GetDiskFreeSpaceExW")
)

type filetime struct {
	lowDateTime  uint32
	highDateTime uint32
}

type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

type windowsCounterSample struct {
	Path        string  `json:"Path"`
	CookedValue float64 `json:"CookedValue"`
}

var (
	windowsPerfInitialized      bool
	windowsPerfLastAt           time.Time
	windowsContextSwitchesTotal float64
	windowsPageFaultsTotal      float64
	windowsDiskReadBytesTotal   float64
	windowsDiskWriteBytesTotal  float64
	windowsDiskReadOpsTotal     float64
	windowsDiskWriteOpsTotal    float64
	windowsDiskBusySecondsTotal float64
	windowsPerfMu               sync.Mutex
)

func collectRawHostSnapshot(diskPath string) rawHostSnapshot {
	snapshot := rawHostSnapshot{
		DiskPath: diskPath,
	}

	parseWindowsCPU(&snapshot)
	parseWindowsMemory(&snapshot)
	parseWindowsDiskUsage(&snapshot, diskPath)
	parseWindowsPerformanceCounters(&snapshot)

	return snapshot
}

func parseWindowsCPU(snapshot *rawHostSnapshot) {
	idle, kernel, user, ok := getSystemTimes()
	if !ok {
		return
	}

	system := uint64(0)
	if kernel > idle {
		system = kernel - idle
	}

	total := user + kernel
	if total == 0 {
		return
	}

	snapshot.CPUTimesAvailable = true
	snapshot.CPUUserTotal = float64(user)
	snapshot.CPUSystemTotal = float64(system)
	snapshot.CPUIdleTotal = float64(idle)
	snapshot.CPUTotal = float64(total)
}

func parseWindowsMemory(snapshot *rawHostSnapshot) {
	status, ok := getMemoryStatus()
	if !ok || status.totalPhys == 0 {
		return
	}

	usedPhys := uint64(0)
	if status.totalPhys > status.availPhys {
		usedPhys = status.totalPhys - status.availPhys
	}

	usedPercent := 0.0
	if status.totalPhys > 0 {
		usedPercent = float64(usedPhys) / float64(status.totalPhys) * 100
	}

	snapshot.MemoryAvailable = true
	snapshot.MemoryTotalBytes = status.totalPhys
	snapshot.MemoryUsedBytes = usedPhys
	snapshot.MemoryFreeBytes = status.availPhys
	snapshot.MemoryAvailableBytes = status.availPhys
	snapshot.MemoryUsedPercent = usedPercent

	if status.totalPageFile > 0 {
		swapUsed := uint64(0)
		if status.totalPageFile > status.availPageFile {
			swapUsed = status.totalPageFile - status.availPageFile
		}

		swapUsedPercent := 0.0
		if status.totalPageFile > 0 {
			swapUsedPercent = float64(swapUsed) / float64(status.totalPageFile) * 100
		}

		snapshot.SwapAvailable = true
		snapshot.SwapTotalBytes = status.totalPageFile
		snapshot.SwapUsedBytes = swapUsed
		snapshot.SwapFreeBytes = status.availPageFile
		snapshot.SwapUsedPercent = swapUsedPercent
	}
}

func parseWindowsDiskUsage(snapshot *rawHostSnapshot, diskPath string) {
	free, total, ok := getDiskSpace(diskPath)
	if !ok || total == 0 {
		return
	}

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

func parseWindowsPerformanceCounters(snapshot *rawHostSnapshot) {
	windowsPerfMu.Lock()
	defer windowsPerfMu.Unlock()

	now := time.Now()
	contextSwitchRate, pageFaultRate, diskReadBps, diskWriteBps, diskReadOpsPs, diskWriteOpsPs, diskUtilPct, ok := queryWindowsRates()
	if !ok {
		return
	}

	if windowsPerfInitialized {
		elapsed := now.Sub(windowsPerfLastAt).Seconds()
		if elapsed > 0 {
			windowsContextSwitchesTotal += math.Max(0, contextSwitchRate) * elapsed
			windowsPageFaultsTotal += math.Max(0, pageFaultRate) * elapsed
			windowsDiskReadBytesTotal += math.Max(0, diskReadBps) * elapsed
			windowsDiskWriteBytesTotal += math.Max(0, diskWriteBps) * elapsed
			windowsDiskReadOpsTotal += math.Max(0, diskReadOpsPs) * elapsed
			windowsDiskWriteOpsTotal += math.Max(0, diskWriteOpsPs) * elapsed
			windowsDiskBusySecondsTotal += clampPercent(diskUtilPct) / 100 * elapsed
		}
	}

	windowsPerfInitialized = true
	windowsPerfLastAt = now

	snapshot.HasContextSwitches = true
	snapshot.ContextSwitchesTotal = uint64(windowsContextSwitchesTotal)
	snapshot.HasPageFaults = true
	snapshot.PageFaultsTotal = uint64(windowsPageFaultsTotal)

	snapshot.HasDiskIOCounters = true
	snapshot.DiskReadBytesTotal = uint64(windowsDiskReadBytesTotal)
	snapshot.DiskWriteBytesTotal = uint64(windowsDiskWriteBytesTotal)
	snapshot.DiskReadOpsTotal = uint64(windowsDiskReadOpsTotal)
	snapshot.DiskWriteOpsTotal = uint64(windowsDiskWriteOpsTotal)
	snapshot.DiskUtilizationAvailable = true
	snapshot.DiskUtilizationPercent = clampPercent(diskUtilPct)
	snapshot.HasDiskIOTime = true
	snapshot.DiskIOTimeSeconds = windowsDiskBusySecondsTotal
}

func queryWindowsRates() (float64, float64, float64, float64, float64, float64, float64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	script := `$c=Get-Counter '\System\Context Switches/sec','\Memory\Page Faults/sec','\PhysicalDisk(_Total)\Disk Read Bytes/sec','\PhysicalDisk(_Total)\Disk Write Bytes/sec','\PhysicalDisk(_Total)\Disk Reads/sec','\PhysicalDisk(_Total)\Disk Writes/sec','\PhysicalDisk(_Total)\% Disk Time';$c.CounterSamples | Select-Object Path,CookedValue | ConvertTo-Json -Compress`

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, false
	}

	var samples []windowsCounterSample
	if err := json.Unmarshal(output, &samples); err != nil {
		var one windowsCounterSample
		if err := json.Unmarshal(output, &one); err != nil {
			return 0, 0, 0, 0, 0, 0, 0, false
		}
		samples = []windowsCounterSample{one}
	}

	var contextSwitchRate float64
	var pageFaultRate float64
	var diskReadBps float64
	var diskWriteBps float64
	var diskReadOpsPs float64
	var diskWriteOpsPs float64
	var diskUtilPct float64

	for _, sample := range samples {
		path := strings.ToLower(sample.Path)
		switch {
		case strings.Contains(path, `\system\context switches/sec`):
			contextSwitchRate = sample.CookedValue
		case strings.Contains(path, `\memory\page faults/sec`):
			pageFaultRate = sample.CookedValue
		case strings.Contains(path, `\physicaldisk(_total)\disk read bytes/sec`):
			diskReadBps = sample.CookedValue
		case strings.Contains(path, `\physicaldisk(_total)\disk write bytes/sec`):
			diskWriteBps = sample.CookedValue
		case strings.Contains(path, `\physicaldisk(_total)\disk reads/sec`):
			diskReadOpsPs = sample.CookedValue
		case strings.Contains(path, `\physicaldisk(_total)\disk writes/sec`):
			diskWriteOpsPs = sample.CookedValue
		case strings.Contains(path, `\physicaldisk(_total)\% disk time`):
			diskUtilPct = sample.CookedValue
		}
	}

	return contextSwitchRate, pageFaultRate, diskReadBps, diskWriteBps, diskReadOpsPs, diskWriteOpsPs, diskUtilPct, true
}

func getSystemTimes() (uint64, uint64, uint64, bool) {
	var idle filetime
	var kernel filetime
	var user filetime

	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r1 == 0 {
		return 0, 0, 0, false
	}

	return filetimeToUint64(idle), filetimeToUint64(kernel), filetimeToUint64(user), true
}

func getMemoryStatus() (memoryStatusEx, bool) {
	var status memoryStatusEx
	status.length = uint32(unsafe.Sizeof(status))

	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return memoryStatusEx{}, false
	}
	return status, true
}

func getDiskSpace(path string) (uint64, uint64, bool) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}

	var freeBytes uint64
	var totalBytes uint64
	var totalFreeBytes uint64

	r1, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0, false
	}

	return totalFreeBytes, totalBytes, true
}

func filetimeToUint64(t filetime) uint64 {
	return (uint64(t.highDateTime) << 32) | uint64(t.lowDateTime)
}
