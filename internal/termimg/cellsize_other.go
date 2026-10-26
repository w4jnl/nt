//go:build !unix

package termimg

func cellSize() (w, h int) { return 0, 0 }
