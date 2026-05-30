//go:build !dev

package bootstrap

// Default (production) build: `wails build` compiles with the "desktop" /
// "production" tags, not "dev". The API binds to an ephemeral loopback port
// (chosen by the OS) and the actual address is reported to the frontend via
// GetApiBase, avoiding collisions with any other process on a fixed port.

// IsDev reports whether this is a development build.
const IsDev = false

// ListenAddr is the loopback address the API server binds to.
func ListenAddr() string {
	return "127.0.0.1:0"
}
