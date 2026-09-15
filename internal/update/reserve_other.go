//go:build !linux

package update

import (
	"context"
	"errors"
)

func reserveRollbackSpace(context.Context, string, string) error {
	return errors.New("one-click apply requires Linux rollback space reservation")
}
