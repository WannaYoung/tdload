//go:build windows

package api

func diskUsage(dir string) (used, available, total uint64) {
	return 0, 0, 0
}
