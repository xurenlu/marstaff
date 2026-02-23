package device

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/device/android"
	"github.com/rocky/marstaff/internal/device/browser"
	"github.com/rocky/marstaff/internal/device/types"
	"github.com/rocky/marstaff/internal/device/windows"
)

// ToolExecutor provides device control tools for the agent
type ToolExecutor struct {
	manager *Manager
	engine  *agent.Engine
}

// NewToolExecutor creates a new device control tool executor
func NewToolExecutor(engine *agent.Engine) *ToolExecutor {
	return &ToolExecutor{
		manager: NewManager(),
		engine:  engine,
	}
}

// RegisterBuiltInTools registers device control tools with the engine
func (e *ToolExecutor) RegisterBuiltInTools() {
	// Windows device tools
	e.engine.RegisterTool("device_windows_connect", e.toolWindowsConnect)
	e.engine.RegisterTool("device_windows_tap", e.toolWindowsTap)
	e.engine.RegisterTool("device_windows_swipe", e.toolWindowsSwipe)
	e.engine.RegisterTool("device_windows_input", e.toolWindowsInput)
	e.engine.RegisterTool("device_windows_key", e.toolWindowsKey)
	e.engine.RegisterTool("device_windows_launch", e.toolWindowsLaunch)
	e.engine.RegisterTool("device_windows_close", e.toolWindowsClose)
	e.engine.RegisterTool("device_windows_screenshot", e.toolWindowsScreenshot)

	// Android device tools
	e.engine.RegisterTool("device_android_connect", e.toolAndroidConnect)
	e.engine.RegisterTool("device_android_tap", e.toolAndroidTap)
	e.engine.RegisterTool("device_android_swipe", e.toolAndroidSwipe)
	e.engine.RegisterTool("device_android_input", e.toolAndroidInput)
	e.engine.RegisterTool("device_android_key", e.toolAndroidKey)
	e.engine.RegisterTool("device_android_launch", e.toolAndroidLaunch)
	e.engine.RegisterTool("device_android_close", e.toolAndroidClose)
	e.engine.RegisterTool("device_android_screenshot", e.toolAndroidScreenshot)

	// Browser device tools
	e.engine.RegisterTool("device_browser_connect", e.toolBrowserConnect)
	e.engine.RegisterTool("device_browser_navigate", e.toolBrowserNavigate)
	e.engine.RegisterTool("device_browser_click_element", e.toolBrowserClickElement)
	e.engine.RegisterTool("device_browser_input_to", e.toolBrowserInputTo)
	e.engine.RegisterTool("device_browser_get_text", e.toolBrowserGetText)
	e.engine.RegisterTool("device_browser_get_html", e.toolBrowserGetHTML)
	e.engine.RegisterTool("device_browser_get_url", e.toolBrowserGetURL)
	e.engine.RegisterTool("device_browser_get_title", e.toolBrowserGetTitle)
	e.engine.RegisterTool("device_browser_screenshot", e.toolBrowserScreenshot)
	e.engine.RegisterTool("device_browser_eval", e.toolBrowserEval)
	e.engine.RegisterTool("device_browser_wait_for", e.toolBrowserWaitFor)
	e.engine.RegisterTool("device_browser_select_option", e.toolBrowserSelectOption)
	e.engine.RegisterTool("device_browser_tap", e.toolBrowserTap)
	e.engine.RegisterTool("device_browser_swipe", e.toolBrowserSwipe)
	e.engine.RegisterTool("device_browser_input", e.toolBrowserInput)
	e.engine.RegisterTool("device_browser_key", e.toolBrowserKey)

	log.Info().Msg("registered device control tools")
}

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

	// TODO: Convert image to base64 and return
	log.Info().Int("width", img.Bounds().Dx()).Int("height", img.Bounds().Dy()).Msg("captured screenshot")

	return fmt.Sprintf("Screenshot captured: %dx%d", img.Bounds().Dx(), img.Bounds().Dy()), nil
}

// toolAndroidConnect connects to an Android device
func (e *ToolExecutor) toolAndroidConnect(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	adbPath := getString(params, "adb_path", "")

	if deviceID == "" {
		return "", fmt.Errorf("device_id parameter is required (use empty string for first available device)")
	}

	dev := android.NewDevice(deviceID, adbPath)
	if err := dev.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to Android device: %w", err)
	}

	e.manager.Register(deviceID, dev)

	return fmt.Sprintf("Connected to Android device: %s", deviceID), nil
}

// toolAndroidTap taps on Android screen
func (e *ToolExecutor) toolAndroidTap(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	x := getInt(params, "x", 0)
	y := getInt(params, "y", 0)

	if deviceID == "" {
		// Try to get default Android device
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	if err := dev.Tap(ctx, x, y); err != nil {
		return "", fmt.Errorf("tap failed: %w", err)
	}

	return fmt.Sprintf("Tapped at (%d, %d)", x, y), nil
}

// toolAndroidSwipe swipes on Android screen
func (e *ToolExecutor) toolAndroidSwipe(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	x1 := getInt(params, "x1", 0)
	y1 := getInt(params, "y1", 0)
	x2 := getInt(params, "x2", 0)
	y2 := getInt(params, "y2", 0)
	durationMs := getInt(params, "duration_ms", 500)

	if deviceID == "" {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	duration := time.Duration(durationMs) * time.Millisecond
	if err := dev.Swipe(ctx, x1, y1, x2, y2, duration); err != nil {
		return "", fmt.Errorf("swipe failed: %w", err)
	}

	return fmt.Sprintf("Swiped from (%d, %d) to (%d, %d)", x1, y1, x2, y2), nil
}

// toolAndroidInput types text on Android
func (e *ToolExecutor) toolAndroidInput(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	text := getString(params, "text", "")

	if text == "" {
		return "", fmt.Errorf("text parameter is required")
	}

	if deviceID == "" {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	if err := dev.InputText(ctx, text); err != nil {
		return "", fmt.Errorf("input failed: %w", err)
	}

	return fmt.Sprintf("Typed: %s", text), nil
}

// toolAndroidKey presses a key on Android
func (e *ToolExecutor) toolAndroidKey(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	key := getString(params, "key", "")

	if key == "" {
		return "", fmt.Errorf("key parameter is required")
	}

	if deviceID == "" {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	if err := dev.KeyPress(ctx, key); err != nil {
		return "", fmt.Errorf("key press failed: %w", err)
	}

	return fmt.Sprintf("Pressed key: %s", key), nil
}

// toolAndroidLaunch launches an app on Android
func (e *ToolExecutor) toolAndroidLaunch(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	appName := getString(params, "app_name", "")

	if appName == "" {
		return "", fmt.Errorf("app_name parameter is required")
	}

	if deviceID == "" {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	if err := dev.LaunchApp(ctx, appName); err != nil {
		return "", fmt.Errorf("launch app failed: %w", err)
	}

	return fmt.Sprintf("Launched app: %s", appName), nil
}

// toolAndroidClose closes an app on Android
func (e *ToolExecutor) toolAndroidClose(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")
	appName := getString(params, "app_name", "")

	if appName == "" {
		return "", fmt.Errorf("app_name parameter is required")
	}

	if deviceID == "" {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	if err := dev.CloseApp(ctx, appName); err != nil {
		return "", fmt.Errorf("close app failed: %w", err)
	}

	return fmt.Sprintf("Closed app: %s", appName), nil
}

// toolAndroidScreenshot captures screenshot on Android
func (e *ToolExecutor) toolAndroidScreenshot(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "")

	if deviceID == "" {
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			return "", fmt.Errorf("no Android device connected")
		}
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("device not found: %s", deviceID)
	}

	img, err := dev.Screenshot(ctx)
	if err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}

	// TODO: Convert image to base64 and return
	if img != nil {
		log.Info().Int("width", img.Bounds().Dx()).Int("height", img.Bounds().Dy()).Msg("captured screenshot")
	}

	return "Screenshot captured", nil
}

// Browser device tools

// toolBrowserConnect connects to a browser device
func (e *ToolExecutor) toolBrowserConnect(ctx context.Context, params map[string]interface{}) (string, error) {
	remoteURL := getString(params, "remote_url", "")
	headless := getBool(params, "headless", true)

	deviceID := "browser_default"
	if remoteURL != "" {
		deviceID = fmt.Sprintf("browser_%s", remoteURL)
	}

	dev := browser.NewDevice(remoteURL, headless)
	if err := dev.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to browser: %w", err)
	}

	e.manager.Register(deviceID, dev)

	return fmt.Sprintf("Connected to browser device: %s", deviceID), nil
}

// toolBrowserNavigate navigates to a URL
func (e *ToolExecutor) toolBrowserNavigate(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	url := getString(params, "url", "")

	if url == "" {
		return "", fmt.Errorf("url parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	if err := browserDev.Navigate(ctx, url); err != nil {
		return "", fmt.Errorf("navigate failed: %w", err)
	}

	return fmt.Sprintf("Navigated to: %s", url), nil
}

// toolBrowserClickElement clicks an element by CSS selector
func (e *ToolExecutor) toolBrowserClickElement(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	selector := getString(params, "selector", "")

	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	if err := browserDev.ClickElement(ctx, selector); err != nil {
		return "", fmt.Errorf("click element failed: %w", err)
	}

	return fmt.Sprintf("Clicked element: %s", selector), nil
}

// toolBrowserInputTo inputs text into an element
func (e *ToolExecutor) toolBrowserInputTo(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	selector := getString(params, "selector", "")
	text := getString(params, "text", "")

	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}
	if text == "" {
		return "", fmt.Errorf("text parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	if err := browserDev.InputTo(ctx, selector, text); err != nil {
		return "", fmt.Errorf("input to element failed: %w", err)
	}

	return fmt.Sprintf("Input text to %s: %s", selector, text), nil
}

// toolBrowserGetText retrieves text from an element
func (e *ToolExecutor) toolBrowserGetText(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	selector := getString(params, "selector", "")

	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	text, err := browserDev.GetText(ctx, selector)
	if err != nil {
		return "", fmt.Errorf("get text failed: %w", err)
	}

	return text, nil
}

// toolBrowserGetHTML retrieves HTML from an element
func (e *ToolExecutor) toolBrowserGetHTML(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	selector := getString(params, "selector", "")

	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	html, err := browserDev.GetHTML(ctx, selector)
	if err != nil {
		return "", fmt.Errorf("get html failed: %w", err)
	}

	return html, nil
}

// toolBrowserGetURL gets the current page URL
func (e *ToolExecutor) toolBrowserGetURL(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	url, err := browserDev.GetURL(ctx)
	if err != nil {
		return "", fmt.Errorf("get url failed: %w", err)
	}

	return url, nil
}

// toolBrowserGetTitle gets the current page title
func (e *ToolExecutor) toolBrowserGetTitle(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	title, err := browserDev.GetTitle(ctx)
	if err != nil {
		return "", fmt.Errorf("get title failed: %w", err)
	}

	return title, nil
}

// toolBrowserScreenshot captures a screenshot
func (e *ToolExecutor) toolBrowserScreenshot(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	img, err := dev.Screenshot(ctx)
	if err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}

	log.Info().Int("width", img.Bounds().Dx()).Int("height", img.Bounds().Dy()).Msg("browser screenshot captured")

	return fmt.Sprintf("Browser screenshot captured: %dx%d", img.Bounds().Dx(), img.Bounds().Dy()), nil
}

// toolBrowserEval executes JavaScript
func (e *ToolExecutor) toolBrowserEval(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	script := getString(params, "script", "")

	if script == "" {
		return "", fmt.Errorf("script parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	result, err := browserDev.Eval(ctx, script)
	if err != nil {
		return "", fmt.Errorf("eval failed: %w", err)
	}

	return fmt.Sprintf("Eval result: %v", result), nil
}

// toolBrowserWaitFor waits for an element
func (e *ToolExecutor) toolBrowserWaitFor(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	selector := getString(params, "selector", "")
	timeoutMs := getInt(params, "timeout_ms", 30000)

	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	if err := browserDev.WaitFor(ctx, selector, time.Duration(timeoutMs)*time.Millisecond); err != nil {
		return "", fmt.Errorf("wait for element failed: %w", err)
	}

	return fmt.Sprintf("Element found: %s", selector), nil
}

// toolBrowserSelectOption selects an option from a select element
func (e *ToolExecutor) toolBrowserSelectOption(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	selector := getString(params, "selector", "")
	value := getString(params, "value", "")

	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}
	if value == "" {
		return "", fmt.Errorf("value parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	browserDev, ok := dev.(*browser.Device)
	if !ok {
		return "", fmt.Errorf("device is not a browser device")
	}

	if err := browserDev.SelectOption(ctx, selector, value); err != nil {
		return "", fmt.Errorf("select option failed: %w", err)
	}

	return fmt.Sprintf("Selected option '%s' in %s", value, selector), nil
}

// toolBrowserTap taps at coordinates
func (e *ToolExecutor) toolBrowserTap(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	x := getInt(params, "x", 0)
	y := getInt(params, "y", 0)

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	if err := dev.Tap(ctx, x, y); err != nil {
		return "", fmt.Errorf("tap failed: %w", err)
	}

	return fmt.Sprintf("Tapped at (%d, %d)", x, y), nil
}

// toolBrowserSwipe scrolls the page
func (e *ToolExecutor) toolBrowserSwipe(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	x1 := getInt(params, "x1", 0)
	y1 := getInt(params, "y1", 0)
	x2 := getInt(params, "x2", 0)
	y2 := getInt(params, "y2", 0)
	durationMs := getInt(params, "duration_ms", 500)

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	duration := time.Duration(durationMs) * time.Millisecond
	if err := dev.Swipe(ctx, x1, y1, x2, y2, duration); err != nil {
		return "", fmt.Errorf("swipe failed: %w", err)
	}

	return fmt.Sprintf("Scrolled from (%d, %d) to (%d, %d)", x1, y1, x2, y2), nil
}

// toolBrowserInput types text
func (e *ToolExecutor) toolBrowserInput(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	text := getString(params, "text", "")

	if text == "" {
		return "", fmt.Errorf("text parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	if err := dev.InputText(ctx, text); err != nil {
		return "", fmt.Errorf("input failed: %w", err)
	}

	return fmt.Sprintf("Typed: %s", text), nil
}

// toolBrowserKey presses a key
func (e *ToolExecutor) toolBrowserKey(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceID := getString(params, "device_id", "browser_default")
	key := getString(params, "key", "")

	if key == "" {
		return "", fmt.Errorf("key parameter is required")
	}

	dev, ok := e.manager.Get(deviceID)
	if !ok {
		return "", fmt.Errorf("browser device not found: %s", deviceID)
	}

	if err := dev.KeyPress(ctx, key); err != nil {
		return "", fmt.Errorf("key press failed: %w", err)
	}

	return fmt.Sprintf("Pressed key: %s", key), nil
}

// Helper functions
func getString(params map[string]interface{}, key, defaultValue string) string {
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

func getInt(params map[string]interface{}, key string, defaultValue int) int {
	if v, ok := params[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		case string:
			// Try to parse as int
			var i int
			if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
				return i
			}
		}
	}
	return defaultValue
}

func getBool(params map[string]interface{}, key string, defaultValue bool) bool {
	if v, ok := params[key]; ok {
		switch val := v.(type) {
		case bool:
			return val
		case string:
			if val == "true" || val == "1" {
				return true
			}
			if val == "false" || val == "0" {
				return false
			}
		case float64:
			return val != 0
		case int:
			return val != 0
		}
	}
	return defaultValue
}
