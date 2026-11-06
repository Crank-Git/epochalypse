//go:build !linux

package agent

import (
	"errors"
	"os"
)

func reservePhysicalFile(_ *os.File, _ int64) error {
	return errors.New("physical reservation requires supported Linux storage")
}

func exchangePhysicalCheckpoints(_ *os.Root) error {
	return errors.New("atomic checkpoint exchange requires supported Linux storage")
}
