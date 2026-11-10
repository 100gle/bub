//go:build e2e

// This file opts Bub Desktop into MyGo's automation plugin, in builds made
// with the e2e tag only, so an end-to-end test runner or an agent can drive a
// running window: list its windows, read the elements of its native UI, press
// and type, and capture it as a PNG.
//
//	cd desktop
//	go build -tags e2e -o build/bub-desktop .
//	MYGO_AUTOMATION=/tmp/bub-desktop.sock ./build/bub-desktop
//	mygo-automation --socket /tmp/bub-desktop.sock windows
//	mygo-automation --socket /tmp/bub-desktop.sock capture --window 1 > shot.png
//
// The plugin listens only when MYGO_AUTOMATION names a socket path, and it is
// left out of release builds. The elements worth driving carry stable keys:
// the composer is keyed by Context.Key("composer"), and the Send, Stop, error
// banner and status chip by Key("send"), Key("stop"), Key("error") and
// Key("status").
package main

import (
	// Registers the automation plugin with the MyGo app.
	_ "github.com/egoist/mygo/plugins/automation"
)
