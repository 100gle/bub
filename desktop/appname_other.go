//go:build !darwin

package main

// The application name comes from mygo.App.SetName on these platforms.
func setNativeAppName(string) {}

func setNativeMenuTitle(string) {}
