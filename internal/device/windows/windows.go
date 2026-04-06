//go:build windows
// +build windows

package windows

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/device/types"
)

// Device implements Device interface for Windows
type Device struct {
	host     string
	port     int
	password string
	connected bool
	screenW   int
	screenH   int
}

// NewDevice creates a new Windows device controller
// For local Windows, leave host empty
// For remote Windows, provide host:port
func NewDevice(host string, port int, password string) *Device {
	return &Device{
		host:     host,
		port:     port,
		password: password,
	}
}

// Connect establishes connection to the Windows device
func (d *Device) Connect(ctx context.Context) error {
	log.Info().Str("host", d.host).Msg("connecting to windows device")

	// For local device, initialize robotgo
	if d.host == "" || d.host == "localhost" {
		// Get screen size
		w, h := robotgo.GetScreenSize()
		d.screenW = int(w)
		d.screenH = int(h)
		d.connected = true
		log.Info().Int("width", d.screenW).Int("height", d.screenH).Msg("connected to local windows device")
		return nil
	}

	// TODO: Implement remote connection via RDP/VNC
	return fmt.Errorf("remote connection not yet implemented")
}

// Disconnect closes the connection
func (d *Device) Disconnect(ctx context.Context) error {
	d.connected = false
	log.Info().Msg("disconnected from windows device")
	return nil
}

// Screenshot captures the current screen
func (d *Device) Screenshot(ctx context.Context) (*image.RGBA, error) {
	if !d.connected {
		return nil, fmt.Errorf("device not connected")
	}

	// Capture screen using robotgo
	// robotgo.CaptureScreen returns a CBitmap object
	// For now, return a basic implementation
	// Full image encoding can be added later

	// Get screen bounds
	x := 0
	y := 0
	w, h := robotgo.GetScreenSize()

	tmpFile := fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano())
	robotgo.SaveCapture(tmpFile, x, y, w, h)
	defer os.Remove(tmpFile)

	f, err := os.Open(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("open capture file: %w", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode screenshot png: %w", err)
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	return rgba, nil
}

// Tap clicks at the specified coordinates
func (d *Device) Tap(ctx context.Context, x, y int) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Int("x", x).Int("y", y).Msg("windows tap")

	// Move mouse and click
	robotgo.MoveMouse(x, y)
	time.Sleep(50 * time.Millisecond)
	robotgo.MouseClick("left")

	return nil
}

// Swipe performs a swipe gesture (mouse drag on desktop)
func (d *Device) Swipe(ctx context.Context, x1, y1, x2, y2 int, duration time.Duration) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().
		Int("x1", x1).Int("y1", y1).
		Int("x2", x2).Int("y2", y2).
		Dur("duration", duration).
		Msg("windows swipe")

	// Move to start position
	robotgo.MoveMouse(x1, y1)
	time.Sleep(50 * time.Millisecond)

	// Mouse down
	robotgo.MouseClick("left", true)

	// Move to end position over duration
	steps := 20
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x1 + int(float64(x2-x1)*t)
		y := y1 + int(float64(y2-y1)*t)
		robotgo.MoveMouse(x, y)
		time.Sleep(duration / time.Duration(steps))
	}

	// Mouse up
	robotgo.MouseClick("left", false)

	return nil
}

// InputText types the specified text
func (d *Device) InputText(ctx context.Context, text string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("text", text).Msg("windows input text")

	// Type text using robotgo
	robotgo.TypeStr(text)

	return nil
}

// KeyPress simulates a key press (down + up)
func (d *Device) KeyPress(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("key", key).Msg("windows key press")

	// Map common keys
	k := d.mapKey(key)
	robotgo.KeyTap(k)

	return nil
}

// KeyDown holds a key down
func (d *Device) KeyDown(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("key", key).Msg("windows key down")
	k := d.mapKey(key)
	robotgo.KeyToggle(k, true)
	return nil
}

// KeyUp releases a key
func (d *Device) KeyUp(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("key", key).Msg("windows key up")
	k := d.mapKey(key)
	robotgo.KeyToggle(k, false)
	return nil
}

// LaunchApp launches an application by name
func (d *Device) LaunchApp(ctx context.Context, appName string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Info().Str("app", appName).Msg("windows launch app")

	// Try to find and launch the app
	// This uses Windows ShellExecute to open files/URLs/apps
	_, err := robotgo.Run(appName)
	if err != nil {
		return fmt.Errorf("failed to launch app %s: %w", appName, err)
	}

	return nil
}

// CloseApp closes an application by name
func (d *Device) CloseApp(ctx context.Context, appName string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Info().Str("app", appName).Msg("windows close app")

	// Find window by title and close
	// This is a simplified approach using taskkill on Windows
	// For now, we'll use a different approach - send WM_CLOSE message
	pid, err := robotgo.FindIds(appName)
	if err != nil || len(pid) == 0 {
		return fmt.Errorf("app not found: %s", appName)
	}

	// Try to activate and close the window
	// Use robotgo to send Alt+F4 to the window
	_ = pid  // Use pid
	time.Sleep(100 * time.Millisecond)
	robotgo.KeyTap("f4", "alt")

	return nil
}

// GetScreenSize returns the screen dimensions
func (d *Device) GetScreenSize(ctx context.Context) (width, height int, err error) {
	if !d.connected {
		return 0, 0, fmt.Errorf("device not connected")
	}

	return d.screenW, d.screenH, nil
}

// HealthCheck verifies the device connection
func (d *Device) HealthCheck(ctx context.Context) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	// Try to get screen size as health check
	w, h := robotgo.GetScreenSize()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("invalid screen size: %dx%d", w, h)
	}

	return nil
}

// Platform returns the device platform
func (d *Device) Platform() types.Platform {
	return types.PlatformWindows
}

// mapKey maps common key names to robotgo key names
func (d *Device) mapKey(key string) string {
	keyMap := map[string]string{
		"enter":     "return",
		"return":    "return",
		"space":     "space",
		"tab":       "tab",
		"escape":    "escape",
		"esc":       "escape",
		"backspace": "backspace",
		"delete":    "delete",
		"home":      "home",
		"end":       "end",
		"pageup":    "pageup",
		"pagedown":  "pagedown",
		"up":        "up",
		"down":      "down",
		"left":      "left",
		"right":     "right",
		"f1":        "f1",
		"f2":        "f2",
		"f3":        "f3",
		"f4":        "f4",
		"f5":        "f5",
		"f6":        "f6",
		"f7":        "f7",
		"f8":        "f8",
		"f9":        "f9",
		"f10":       "f10",
		"f11":       "f11",
		"f12":       "f12",
		"ctrl":      "ctrl",
		"alt":       "alt",
		"shift":     "shift",
		"win":       "cmd", // Windows key is "cmd" in robotgo
		"meta":      "cmd",
	}

	if k, ok := keyMap[key]; ok {
		return k
	}
	return key
}
