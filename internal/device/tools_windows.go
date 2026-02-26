//go:build windows
// +build windows

package device

import (
	"context"
	"fmt"
	"image"
	"time"

	"github.com/rocky/marstaff/internal/device/types"
	"github.com/rocky/marstaff/internal/device/windows"
)

// toolWindowsConnect connects to a Windows device
func (e *ToolExecutor) toolWindowsConnect(ctx context.Context, params map[string]interface{}) (string, error) {
	host := getString(params, "host", "")
	port := getInt(params, "port", 0)
	password := getString(params, "password", "")

	deviceID := fmt.Sprintf("windows_%s_%d", host, port)

	dev := windows.NewDevice(host, port, password)
	if err := dev.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to Windows device: %w", err)
	}

	e.manager.Register(deviceID, dev)

	return fmt.Sprintf("Connected to Windows device: %s", deviceID), nil
}

// toolWindowsTap taps on Windows screen
func (e *ToolExecutor) toolWindowsTap(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")
	x := getInt(params, "x", 0)
	y := getInt(params, "y", 0)

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		// Try to get default Windows device
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				deviceID = id
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	if err := dev.Tap(ctx, x, y); err != nil {
		return "", fmt.Errorf("tap failed: %w", err)
	}

	return fmt.Sprintf("Tapped at (%d, %d)", x, y), nil
}

// toolWindowsSwipe swipes on Windows screen
func (e *ToolExecutor) toolWindowsSwipe(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")
	x1 := getInt(params, "x1", 0)
	y1 := getInt(params, "y1", 0)
	x2 := getInt(params, "x2", 0)
	y2 := getInt(params, "y2", 0)
	durationMs := getInt(params, "duration_ms", 500)

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	duration := time.Duration(durationMs) * time.Millisecond
	if err := dev.Swipe(ctx, x1, y1, x2, y2, duration); err != nil {
		return "", fmt.Errorf("swipe failed: %w", err)
	}

	return fmt.Sprintf("Swiped from (%d, %d) to (%d, %d)", x1, y1, x2, y2), nil
}

// toolWindowsInput types text on Windows
func (e *ToolExecutor) toolWindowsInput(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")
	text := getString(params, "text", "")

	if text == "" {
		return "", fmt.Errorf("text parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	if err := dev.InputText(ctx, text); err != nil {
		return "", fmt.Errorf("input failed: %w", err)
	}

	return fmt.Sprintf("Typed: %s", text), nil
}

// toolWindowsKey presses a key on Windows
func (e *ToolExecutor) toolWindowsKey(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")
	key := getString(params, "key", "")

	if key == "" {
		return "", fmt.Errorf("key parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	if err := dev.KeyPress(ctx, key); err != nil {
		return "", fmt.Errorf("key press failed: %w", err)
	}

	return fmt.Sprintf("Pressed key: %s", key), nil
}

// toolWindowsLaunch launches an app on Windows
func (e *ToolExecutor) toolWindowsLaunch(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")
	appName := getString(params, "app_name", "")

	if appName == "" {
		return "", fmt.Errorf("app_name parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	if err := dev.LaunchApp(ctx, appName); err != nil {
		return "", fmt.Errorf("launch app failed: %w", err)
	}

	return fmt.Sprintf("Launched app: %s", appName), nil
}

// toolWindowsClose closes an app on Windows
func (e *ToolExecutor) toolWindowsClose(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")
	appName := getString(params, "app_name", "")

	if appName == "" {
		return "", fmt.Errorf("app_name parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	if err := dev.CloseApp(ctx, appName); err != nil {
		return "", fmt.Errorf("close app failed: %w", err)
	}

	return fmt.Sprintf("Closed app: %s", appName), nil
}

// toolWindowsScreenshot captures screenshot on Windows
func (e *ToolExecutor) toolWindowsScreenshot(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "windows__0")

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
		if dev == nil {
			return "", fmt.Errorf("no Windows device connected")
		}
	}

	img, err := dev.Screenshot(ctx)
	if err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}

	url, err := e.uploadScreenshot(ctx, img, "windows")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Screenshot captured: %dx%d\nImage URL: %s", img.Bounds().Dx(), img.Bounds().Dy(), url), nil
}
