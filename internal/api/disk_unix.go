//go:build !windows

package api

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// diskUsage 返回 downloadDir 所在文件系统的 used / available / total（字节）。
func diskUsage(dir string) (used, available, total uint64) {
	if dir == "" {
		return 0, 0, 0
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	var st unix.Statfs_t
	if err := unix.Statfs(abs, &st); err != nil {
		return 0, 0, 0
	}
	// Bsize 在部分平台是 int64
	bsize := uint64(st.Bsize)
	total = st.Blocks * bsize
	available = st.Bavail * bsize
	free := st.Bfree * bsize
	if total >= free {
		used = total - free
	}
	return used, available, total
}
