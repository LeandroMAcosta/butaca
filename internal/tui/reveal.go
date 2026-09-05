package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// fileManager names the desktop's file browser, for the status line.
func fileManager() string {
	switch runtime.GOOS {
	case "darwin":
		return "Finder"
	case "windows":
		return "Explorer"
	default:
		return "the file manager"
	}
}

// revealCommand builds the platform's "show me this in the file browser" call.
// On macOS `open -R` selects the file inside its folder rather than merely
// opening the folder, which is what you want when a directory holds the video
// and several subtitle files.
func revealCommand(path string, isFile bool) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		if isFile {
			return "open", []string{"-R", path}, nil
		}
		return "open", []string{path}, nil
	case "windows":
		if isFile {
			return "explorer", []string{"/select,", filepath.FromSlash(path)}, nil
		}
		return "explorer", []string{filepath.FromSlash(path)}, nil
	case "linux":
		// xdg-open takes a directory; there is no portable "select this file".
		target := path
		if isFile {
			target = filepath.Dir(path)
		}
		return "xdg-open", []string{target}, nil
	default:
		return "", nil, fmt.Errorf("no file manager known for %s", runtime.GOOS)
	}
}

// reveal opens the item in the desktop file browser. It runs detached: the TUI
// must not wait for a graphical application to exit.
func reveal(path string, isFile bool) tea.Cmd {
	return func() tea.Msg {
		if path == "" {
			return actionDone{err: fmt.Errorf("this item has no path on disk")}
		}
		name, args, err := revealCommand(path, isFile)
		if err != nil {
			return actionDone{err: err}
		}
		if err := exec.Command(name, args...).Start(); err != nil {
			return actionDone{err: fmt.Errorf("could not open %s: %w", fileManager(), err)}
		}
		return actionDone{msg: "opened " + filepath.Base(path) + " in " + fileManager()}
	}
}

// revealTarget picks what to show: the actual media file when there is one, so
// the user lands on the video rather than a folder they still have to open.
func (d *detailState) revealTarget() (string, bool) {
	if len(d.files) > 0 && d.files[0].Path != "" {
		return d.files[0].Path, true
	}
	if d.item != nil {
		return d.item.Path, false
	}
	return "", false
}
