//go:build windows

package device

import "github.com/rs/zerolog/log"

// registerWindowsDeviceTools registers robotgo-based desktop automation tools (local Windows only).
func (e *ToolExecutor) registerWindowsDeviceTools() {
	e.registerTool("device_windows_connect", "Connects to a Windows desktop for automation. Use empty host for this machine; remote hosts are not yet supported.", e.toolWindowsConnect)
	e.registerTool("device_windows_tap", "Taps (left click) at screen coordinates on Windows.", e.toolWindowsTap)
	e.registerTool("device_windows_swipe", "Drags the mouse between two points (swipe) on Windows.", e.toolWindowsSwipe)
	e.registerTool("device_windows_input", "Types Unicode text on Windows (focused control).", e.toolWindowsInput)
	e.registerTool("device_windows_key", "Presses a key or shortcut on Windows (e.g. enter, ctrl, alt+f4).", e.toolWindowsKey)
	e.registerTool("device_windows_launch", "Launches an application or opens a path/URL via ShellExecute.", e.toolWindowsLaunch)
	e.registerTool("device_windows_close", "Attempts to close an app by name (best-effort).", e.toolWindowsClose)
	e.registerTool("device_windows_screenshot", "Captures the full screen to PNG and uploads when OSS is configured.", e.toolWindowsScreenshot)
	log.Info().Msg("registered Windows desktop (robotgo) tools")
}
