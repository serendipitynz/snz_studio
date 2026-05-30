//go:build !dev

package main

// Default (production) build: `wails build` compiles with the "desktop" /
// "production" tags, not "dev". The API binds to an ephemeral loopback port
// (chosen by the OS) and the actual address is reported to the frontend via
// GetApiBase, avoiding collisions with any other process on a fixed port.

const isDev = false

func apiListenAddr() string {
	return "127.0.0.1:0"
}
