//go:build darwin

package main

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// setNativeAppName gives the process the name macOS shows in the menu bar of an
// unbundled binary. mygo.App.SetName names the menu roles, but AppKit titles the
// application menu from NSProcessInfo's process name, which a bare executable
// takes from its file name.
func setNativeAppName(name string) {
	_, _ = purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	_, _ = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	infoClass := objc.GetClass("NSProcessInfo")
	if infoClass == 0 {
		return
	}
	info := objc.ID(infoClass).Send(objc.RegisterName("processInfo"))
	if info == 0 {
		return
	}
	setter := objc.RegisterName("setProcessName:")
	if uintptr(info.Send(objc.RegisterName("respondsToSelector:"), setter)) != 0 {
		info.Send(setter, nsString(name))
	}
}

// setNativeMenuTitle titles the application menu's item as well, once the menu
// is installed, so a menu AppKit already built takes the new name.
func setNativeMenuTitle(name string) {
	appClass := objc.GetClass("NSApplication")
	if appClass == 0 {
		return
	}
	app := objc.ID(appClass).Send(objc.RegisterName("sharedApplication"))
	if app == 0 {
		return
	}
	menu := app.Send(objc.RegisterName("mainMenu"))
	if menu == 0 {
		return
	}
	item := menu.Send(objc.RegisterName("itemAtIndex:"), 0)
	if item == 0 {
		return
	}
	item.Send(objc.RegisterName("setTitle:"), nsString(name))
}

// nsString makes an NSString from a Go string; the caller's copy lives through
// the call.
func nsString(s string) objc.ID {
	b := append([]byte(s), 0)
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), unsafe.Pointer(&b[0]))
}
