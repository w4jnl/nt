//go:build tools

package mobile

// gomobile bind needs its bind package in the module (make ios-core); the command line never
// builds this file.
import _ "golang.org/x/mobile/bind"
