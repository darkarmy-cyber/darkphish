//go:build linux

package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// reserveRollbackSpace allocates real blocks, not a sparse file. Unsupported
// filesystems and insufficient space fail before the installation journal.
func reserveRollbackSpace(ctx context.Context, backup, path string) error {
	var size int64 = 16 << 20
	err := filepath.Walk(backup, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.Size() < 0 || info.Size() > (1<<62)-size-8192 {
			return errors.New("backup too large to reserve rollback space")
		}
		// Round each entry up and budget metadata in addition to file contents.
		size += (info.Size()/4096 + 2) * 4096
		return nil
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Fallocate(int(f.Fd()), 0, 0, size); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return f.Sync()
}
