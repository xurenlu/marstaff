package types

// Platform represents the device platform
type Platform string

const (
	PlatformWindows Platform = "windows"
	PlatformAndroid Platform = "android"
	PlatformMacOS   Platform = "macos"
	PlatformLinux   Platform = "linux"
)
