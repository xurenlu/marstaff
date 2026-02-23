package android

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/device/types"
)

// Device implements Device interface for Android via ADB
type Device struct {
	deviceID string
	adbPath  string
	connected bool
	screenW   int
	screenH   int
}

// NewDevice creates a new Android device controller
func NewDevice(deviceID, adbPath string) *Device {
	if adbPath == "" {
		// Try to find adb in PATH
		adbPath = "adb"
	}

	return &Device{
		deviceID: deviceID,
		adbPath:  adbPath,
	}
}

// Connect establishes connection to the Android device
func (d *Device) Connect(ctx context.Context) error {
	log.Info().Str("device_id", d.deviceID).Msg("connecting to android device")

	// Check if adb is available
	if err := d.checkADB(); err != nil {
		return fmt.Errorf("adb not available: %w", err)
	}

	// List devices to verify connection
	devices, err := d.listDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	if d.deviceID == "" {
		// Use first available device
		if len(devices) == 0 {
			return fmt.Errorf("no android devices found")
		}
		d.deviceID = devices[0]
		log.Info().Str("device_id", d.deviceID).Msg("using first available device")
	} else {
		// Verify specific device is connected
		found := false
		for _, dev := range devices {
			if dev == d.deviceID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("device %s not found", d.deviceID)
		}
	}

	// Get screen size
	w, h, err := d.getScreenSize()
	if err != nil {
		log.Warn().Err(err).Msg("failed to get screen size")
	} else {
		d.screenW = w
		d.screenH = h
	}

	d.connected = true
	log.Info().Str("device_id", d.deviceID).Msg("connected to android device")
	return nil
}

// Disconnect closes the connection
func (d *Device) Disconnect(ctx context.Context) error {
	d.connected = false
	log.Info().Str("device_id", d.deviceID).Msg("disconnected from android device")
	return nil
}

// Screenshot captures the current screen
func (d *Device) Screenshot(ctx context.Context) (*image.RGBA, error) {
	if !d.connected {
		return nil, fmt.Errorf("device not connected")
	}

	// Create temp file for screenshot
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano()))

	// Capture screenshot
	cmd := d.adbCmd("shell", "screencap", "-p", tmpFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to capture screenshot: %s: %w", string(output), err)
	}

	// Pull file from device
	pullPath := filepath.Join(os.TempDir(), fmt.Sprintf("screenshot_local_%d.png", time.Now().UnixNano()))
	cmd = d.adbCmd("pull", tmpFile, pullPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to pull screenshot: %s: %w", string(output), err)
	}

	// Clean up remote file
	_ = d.adbCmd("shell", "rm", tmpFile).Run()

	// TODO: Decode image file to image.RGBA
	// For now, return error to indicate this needs implementation
	_ = pullPath
	return nil, fmt.Errorf("image decoding not yet implemented")
}

// Tap clicks at the specified coordinates
func (d *Device) Tap(ctx context.Context, x, y int) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("device_id", d.deviceID).Int("x", x).Int("y", y).Msg("android tap")

	cmd := d.adbCmd("shell", "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tap failed: %s: %w", string(output), err)
	}

	return nil
}

// Swipe performs a swipe gesture
func (d *Device) Swipe(ctx context.Context, x1, y1, x2, y2 int, duration time.Duration) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().
		Str("device_id", d.deviceID).
		Int("x1", x1).Int("y1", y1).
		Int("x2", x2).Int("y2", y2).
		Dur("duration", duration).
		Msg("android swipe")

	durationMs := duration.Milliseconds()
	cmd := d.adbCmd("shell", "input", "swipe",
		strconv.Itoa(x1), strconv.Itoa(y1),
		strconv.Itoa(x2), strconv.Itoa(y2),
		strconv.Itoa(int(durationMs)))

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("swipe failed: %s: %w", string(output), err)
	}

	return nil
}

// InputText types the specified text
func (d *Device) InputText(ctx context.Context, text string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("device_id", d.deviceID).Str("text", text).Msg("android input text")

	// Escape special characters for shell
	text = strings.ReplaceAll(text, " ", "%s")
	text = strings.ReplaceAll(text, "&", "\\&")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	text = strings.ReplaceAll(text, ";", "\\;")
	text = strings.ReplaceAll(text, "|", "\\|")
	text = strings.ReplaceAll(text, "<", "\\<")
	text = strings.ReplaceAll(text, ">", "\\>")
	text = strings.ReplaceAll(text, "'", "\\'")
	text = strings.ReplaceAll(text, "\"", "\\\"")

	cmd := d.adbCmd("shell", "input", "text", text)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("input text failed: %s: %w", string(output), err)
	}

	return nil
}

// KeyPress simulates a key press
func (d *Device) KeyPress(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("device_id", d.deviceID).Str("key", key).Msg("android key press")

	androidKey := d.mapKeyToAndroid(key)
	cmd := d.adbCmd("shell", "input", "keyevent", androidKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("key press failed: %s: %w", string(output), err)
	}

	return nil
}

// KeyDown holds a key down (not supported on Android)
func (d *Device) KeyDown(ctx context.Context, key string) error {
	return fmt.Errorf("keydown not supported on android")
}

// KeyUp releases a key (not supported on Android)
func (d *Device) KeyUp(ctx context.Context, key string) error {
	return fmt.Errorf("keyup not supported on android")
}

// LaunchApp launches an application by package name or activity
func (d *Device) LaunchApp(ctx context.Context, appName string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Info().Str("device_id", d.deviceID).Str("app", appName).Msg("android launch app")

	// Try as package name first
	cmd := d.adbCmd("shell", "monkey", "-p", appName, "-c", "android.intent.category.LAUNCHER", "1")
	if _, err := cmd.CombinedOutput(); err != nil {
		// Try as activity name
		cmd = d.adbCmd("shell", "am", "start", "-n", appName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("launch app failed: %s: %w", string(output), err)
		}
	}

	return nil
}

// CloseApp closes an application by package name
func (d *Device) CloseApp(ctx context.Context, appName string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Info().Str("device_id", d.deviceID).Str("app", appName).Msg("android close app")

	// Force stop the app
	cmd := d.adbCmd("shell", "am", "force-stop", appName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("close app failed: %s: %w", string(output), err)
	}

	return nil
}

// GetScreenSize returns the screen dimensions
func (d *Device) GetScreenSize(ctx context.Context) (width, height int, err error) {
	if !d.connected {
		return 0, 0, fmt.Errorf("device not connected")
	}

	return d.getScreenSize()
}

// HealthCheck verifies the device connection
func (d *Device) HealthCheck(ctx context.Context) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	// Check if device is still connected
	cmd := d.adbCmd("shell", "echo", "ping")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("device not responding: %s: %w", string(output), err)
	}

	if !strings.Contains(string(output), "ping") {
		return fmt.Errorf("device not responding correctly")
	}

	return nil
}

// Platform returns the device platform
func (d *Device) Platform() types.Platform {
	return types.PlatformAndroid
}

// checkADB verifies adb is available
func (d *Device) checkADB() error {
	cmd := exec.Command(d.adbPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb not found: %s: %w", string(output), err)
	}

	if !strings.Contains(string(output), "Android Debug Bridge") {
		return fmt.Errorf("invalid adb version: %s", string(output))
	}

	return nil
}

// listDevices returns list of connected device IDs
func (d *Device) listDevices() ([]string, error) {
	cmd := exec.Command(d.adbPath, "devices")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %s: %w", string(output), err)
	}

	lines := strings.Split(string(output), "\n")
	var devices []string

	for _, line := range lines[1:] { // Skip header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == "device" {
			devices = append(devices, parts[0])
		}
	}

	return devices, nil
}

// getScreenSize gets the screen dimensions
func (d *Device) getScreenSize() (width, height int, err error) {
	cmd := d.adbCmd("shell", "wm", "size")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get screen size: %s: %w", string(output), err)
	}

	// Parse output: "Physical size: 1080x2400"
	re := regexp.MustCompile(`(\d+)x(\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 3 {
		return 0, 0, fmt.Errorf("invalid screen size output: %s", string(output))
	}

	width, _ = strconv.Atoi(matches[1])
	height, _ = strconv.Atoi(matches[2])

	return width, height, nil
}

// adbCmd creates an adb command for this device
func (d *Device) adbCmd(args ...string) *exec.Cmd {
	if d.deviceID != "" {
		args = append([]string{"-s", d.deviceID}, args...)
	}
	return exec.Command(d.adbPath, args...)
}

// mapKeyToAndroid maps key names to Android key codes
func (d *Device) mapKeyToAndroid(key string) string {
	keyMap := map[string]string{
		"enter":     "KEYCODE_ENTER",
		"return":    "KEYCODE_ENTER",
		"space":     "KEYCODE_SPACE",
		"tab":       "KEYCODE_TAB",
		"escape":    "KEYCODE_BACK",
		"esc":       "KEYCODE_BACK",
		"backspace": "KEYCODE_DEL",
		"delete":    "KEYCODE_FORWARD_DEL",
		"home":      "KEYCODE_HOME",
		"end":       "KEYCODE_MOVE_END",
		"pageup":    "KEYCODE_PAGE_UP",
		"pagedown":  "KEYCODE_PAGE_DOWN",
		"up":        "KEYCODE_DPAD_UP",
		"down":      "KEYCODE_DPAD_DOWN",
		"left":      "KEYCODE_DPAD_LEFT",
		"right":     "KEYCODE_DPAD_RIGHT",
		"f1":        "KEYCODE_F1",
		"f2":        "KEYCODE_F2",
		"f3":        "KEYCODE_F3",
		"f4":        "KEYCODE_F4",
		"f5":        "KEYCODE_F5",
		"f6":        "KEYCODE_F6",
		"f7":        "KEYCODE_F7",
		"f8":        "KEYCODE_F8",
		"f9":        "KEYCODE_F9",
		"f10":       "KEYCODE_F10",
		"f11":       "KEYCODE_F11",
		"f12":       "KEYCODE_F12",
		"ctrl":      "KEYCODE_CTRL_LEFT",
		"alt":       "KEYCODE_ALT_LEFT",
		"shift":     "KEYCODE_SHIFT_LEFT",
		"menu":      "KEYCODE_MENU",
		"search":    "KEYCODE_SEARCH",
	}

	if k, ok := keyMap[strings.ToLower(key)]; ok {
		return k
	}

	// Try direct keycode
	if strings.HasPrefix(key, "KEYCODE_") {
		return key
	}

	return "KEYCODE_" + strings.ToUpper(key)
}
