//go:build !linux && !darwin && !windows

package agent

func collectRawHostSnapshot(diskPath string) rawHostSnapshot {
	return rawHostSnapshot{
		DiskPath: diskPath,
	}
}
