//go:build linux

package api

import "syscall"

const linuxTMPFSMagic = 0x01021994

func renderedProfileRootIsBounded(root string) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil || stat.Type != linuxTMPFSMagic || stat.Bsize <= 0 {
		return false
	}
	blockSize := uint64(stat.Bsize)
	if stat.Blocks == 0 || stat.Blocks > maxRenderedProfileFilesystemBytes/blockSize {
		return false
	}
	return stat.Files > 0 && stat.Files <= maxRenderedProfileInodes
}
