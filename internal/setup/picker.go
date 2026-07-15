package setup

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrPickerCanceled    = errors.New("folder picker canceled")
	ErrPickerUnavailable = errors.New("native folder picker unavailable")
)

type FolderPicker interface {
	Pick(context.Context) (string, error)
}

type NativeFolderPicker struct{}

func (NativeFolderPicker) Pick(ctx context.Context) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", ErrPickerUnavailable
	}
	command := exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "Choose or create the project folder for Skillet Setup")`)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(message, "User canceled") || strings.Contains(message, "(-128)") {
			return "", ErrPickerCanceled
		}
		return "", fmt.Errorf("%w: %s", ErrPickerUnavailable, message)
	}
	path := strings.TrimSpace(string(output))
	if path == "" || strings.ContainsRune(path, '\x00') {
		return "", fmt.Errorf("%w: malformed empty path", ErrPickerUnavailable)
	}
	return path, nil
}
