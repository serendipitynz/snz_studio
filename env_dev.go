//go:build dev

package main

// The `dev` build tag is set automatically by `wails dev` (its build output
// type is "dev"). In development the API listens on a fixed port so the Vite
// proxy in frontend/vite.config.ts (/api and /files -> 127.0.0.1:8787) can
// reach it without configuration.

const isDev = true

func apiListenAddr() string {
	return "127.0.0.1:8787"
}
