//go:build !darwin && !linux

package setup

import (
	"fmt"
	"os"
)

func renameRoot(_ *os.Root, oldName, newName string) error {
	return fmt.Errorf("anchored setup rename from %s to %s is unsupported on this platform", oldName, newName)
}
