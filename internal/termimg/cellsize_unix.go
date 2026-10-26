//go:build unix

package termimg

import (
	"os"

	"golang.org/x/sys/unix"
)

// cellSize asks the terminal driver for the size of one cell in pixels; 0 when it does not know.
func cellSize() (w, h int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 {
		return 0, 0
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
}
