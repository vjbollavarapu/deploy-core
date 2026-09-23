package volume

import (
	"os"
	"path/filepath"
)

// CalculateMountpointDiskUsage computes the disk footprint of a volume mountpoint directory.
// Returns size in bytes, or 0 if inaccessible or not a directory.
func CalculateMountpointDiskUsage(mountpoint string) int64 {
	if mountpoint == "" {
		return 0
	}

	info, err := os.Stat(mountpoint)
	if err != nil || !info.IsDir() {
		return 0
	}

	var totalSize int64
	// Walk the mountpoint directory to aggregate file sizes
	_ = filepath.Walk(mountpoint, func(_ string, fInfo os.FileInfo, err error) error {
		if err != nil {
			return nil // continue walking other entries if permission error on single file
		}
		if fInfo != nil && !fInfo.IsDir() {
			totalSize += fInfo.Size()
		}
		return nil
	})

	return totalSize
}
