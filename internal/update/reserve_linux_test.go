//go:build linux

package update

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRollbackReserveAllocatesRealBlocks(t *testing.T) {
	root := t.TempDir()
	backup := filepath.Join(root, "backup")
	if err := os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "binary"), make([]byte, 8192), 0600); err != nil {
		t.Fatal(err)
	}
	reserve := filepath.Join(root, "reserve")
	if err := reserveRollbackSpace(context.Background(), backup, reserve); err != nil {
		t.Fatal(err)
	}
	var stat syscall.Stat_t
	if err := syscall.Stat(reserve, &stat); err != nil {
		t.Fatal(err)
	}
	if stat.Size < 8192 || stat.Blocks*512 < stat.Size {
		t.Fatal("rollback reserve is sparse or smaller than backup", stat.Size, stat.Blocks)
	}
}
