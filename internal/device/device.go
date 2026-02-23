package device

import (
	"context"
	image "image"
	"time"

	"github.com/rocky/marstaff/internal/device/types"
)

// Device represents a controllable device
type Device interface {
	// Connect establishes connection to the device
	Connect(ctx context.Context) error

	// Disconnect closes the connection
	Disconnect(ctx context.Context) error

	// Screenshot captures the current screen
	Screenshot(ctx context.Context) (*image.RGBA, error)

	// Tap clicks at the specified coordinates
	Tap(ctx context.Context, x, y int) error

	// Swipe performs a swipe gesture from (x1,y1) to (x2,y2)
	Swipe(ctx context.Context, x1, y1, x2, y2 int, duration time.Duration) error

	// InputText types the specified text
	InputText(ctx context.Context, text string) error

	// KeyPress simulates a key press
	KeyPress(ctx context.Context, key string) error

	// KeyDown holds a key down
	KeyDown(ctx context.Context, key string) error

	// KeyUp releases a key
	KeyUp(ctx context.Context, key string) error

	// LaunchApp launches an application by name
	LaunchApp(ctx context.Context, appName string) error

	// CloseApp closes an application by name
	CloseApp(ctx context.Context, appName string) error

	// GetScreenSize returns the screen dimensions
	GetScreenSize(ctx context.Context) (width, height int, err error)

	// HealthCheck verifies the device connection
	HealthCheck(ctx context.Context) error

	// Platform returns the device platform
	Platform() types.Platform
}

// Image represents a captured screenshot
type Image struct {
	Data     []byte
	Width    int
	Height   int
	Format   string // "png", "jpeg", etc.
	Captured time.Time
}

// ScreenContent represents parsed screen content
type ScreenContent struct {
	Text       string                 // OCR text
	Elements   []UIElement            // UI elements
	Windows    []WindowInfo           // Open windows (desktop)
	Metadata   map[string]interface{} // Additional info
}

// UIElement represents a detected UI element
type UIElement struct {
	Type     string   // "button", "text", "input", etc.
	Text     string   // Element text
	Bounds   Rect     // Element bounds
	Clickable bool    // Whether element can be clicked
	Children []UIElement // Child elements
}

// Rect represents a rectangular region
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// WindowInfo represents information about a window
type WindowInfo struct {
	ID      int
	Title   string
	AppName string
	Bounds  Rect
	Visible bool
}

// Manager manages multiple devices
type Manager struct {
	devices map[string]Device
}

// NewManager creates a new device manager
func NewManager() *Manager {
	return &Manager{
		devices: make(map[string]Device),
	}
}

// Register registers a device
func (m *Manager) Register(id string, device Device) {
	m.devices[id] = device
}

// Get retrieves a device by ID
func (m *Manager) Get(id string) (Device, bool) {
	dev, ok := m.devices[id]
	return dev, ok
}

// List returns all registered device IDs
func (m *Manager) List() []string {
	ids := make([]string, 0, len(m.devices))
	for id := range m.devices {
		ids = append(ids, id)
	}
	return ids
}

// DisconnectAll disconnects all devices
func (m *Manager) DisconnectAll(ctx context.Context) {
	for id, dev := range m.devices {
		_ = dev.Disconnect(ctx)
		delete(m.devices, id)
	}
}
