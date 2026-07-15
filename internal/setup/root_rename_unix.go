//go:build darwin || linux

package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// renameRoot is the Go 1.24-compatible equivalent of os.Root.Rename. It opens
// every parent directory without following symlinks, then renames relative to
// those anchored directory descriptors.
func renameRoot(root *os.Root, oldName, newName string) error {
	rootFile, err := openRootDescriptor(root)
	if err != nil {
		return err
	}
	defer rootFile.Close()
	rootFD := int(rootFile.Fd())

	oldParent, oldBase, closeOld, err := openRootParent(rootFD, oldName)
	if err != nil {
		return err
	}
	defer closeOld()
	newParent, newBase, closeNew, err := openRootParent(rootFD, newName)
	if err != nil {
		return err
	}
	defer closeNew()
	if err := unix.Renameat(oldParent, oldBase, newParent, newBase); err != nil {
		return fmt.Errorf("rename %s to %s inside setup root: %w", oldName, newName, err)
	}
	return nil
}

func openRootDescriptor(root *os.Root) (*os.File, error) {
	rootInfo, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Open(root.Name(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open anchored setup root: %w", err)
	}
	rootFile := os.NewFile(uintptr(rootFD), root.Name())
	if rootFile == nil {
		_ = unix.Close(rootFD)
		return nil, fmt.Errorf("open anchored setup root file")
	}
	openedInfo, err := rootFile.Stat()
	if err != nil {
		rootFile.Close()
		return nil, err
	}
	if !os.SameFile(rootInfo, openedInfo) {
		rootFile.Close()
		return nil, fmt.Errorf("setup target identity changed before anchored operation")
	}
	return rootFile, nil
}

func openRootParent(rootFD int, name string) (int, string, func(), error) {
	clean := filepath.Clean(name)
	if !filepath.IsLocal(clean) || clean == "." {
		return -1, "", func() {}, fmt.Errorf("rename path is not local to setup root: %s", name)
	}
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, "", func() {}, err
	}
	unix.CloseOnExec(current)
	closeCurrent := func() { _ = unix.Close(current) }
	parent := filepath.Dir(clean)
	if parent != "." {
		for _, component := range strings.Split(parent, string(filepath.Separator)) {
			if component == "" || component == "." {
				continue
			}
			next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if openErr != nil {
				closeCurrent()
				return -1, "", func() {}, fmt.Errorf("open rename parent %s: %w", parent, openErr)
			}
			closeCurrent()
			current = next
			closeCurrent = func() { _ = unix.Close(current) }
		}
	}
	return current, filepath.Base(clean), closeCurrent, nil
}
