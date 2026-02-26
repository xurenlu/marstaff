package device

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/device/android"
	"github.com/rocky/marstaff/internal/device/browser"
	"github.com/rocky/marstaff/internal/device/types"
	"github.com/rocky/marstaff/internal/provider"
)

// ToolExecutor provides device control tools for the agent
type ToolExecutor struct {
	manager        *Manager
	engine         *agent.Engine
	imageUploader  ImageUploader
	visionProvider provider.Provider
	visionModel    string
}

// NewToolExecutor creates a new device control tool executor
func NewToolExecutor(engine *agent.Engine) *ToolExecutor {
	return &ToolExecutor{
		manager: NewManager(),
		engine:  engine,
	}
}

// SetImageUploader sets the OSS uploader for screenshots. Required for screenshot tools to return URLs.
func (e *ToolExecutor) SetImageUploader(u ImageUploader) {
	e.imageUploader = u
}

// SetVisionProvider sets the vision provider and model for screen analysis. Required for device_screen_analyze.
func (e *ToolExecutor) SetVisionProvider(p provider.Provider, model string) {
	e.visionProvider = p
	e.visionModel = model
}

// uploadScreenshot encodes RGBA to PNG, uploads to OSS, returns URL. Falls back to size-only text if no uploader.
func (e *ToolExecutor) uploadScreenshot(ctx context.Context, img *image.RGBA, source string) (string, error) {
	if e.imageUploader == nil {
		return "", fmt.Errorf("OSS 未配置，无法上传截图。请在配置中设置 OSS 后重试")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode screenshot failed: %w", err)
	}
	filename := fmt.Sprintf("screenshot_%s_%d.png", source, time.Now().UnixNano())
	url, err := e.imageUploader.UploadImagePNG(buf.Bytes(), filename)
	if err != nil {
		return "", fmt.Errorf("upload screenshot to OSS failed: %w", err)
	}
	return url, nil
}

// RegisterBuiltInTools registers device control tools with the engine
func (e *ToolExecutor) RegisterBuiltInTools() {
	// Windows device tools (registered in tools_windows.go for Windows builds only)
	// e.registerTool("device_windows_connect", "Connects to a Windows device for remote control", e.toolWindowsConnect)
	// e.registerTool("device_windows_tap", "Taps on Windows screen at coordinates", e.toolWindowsTap)
	// e.registerTool("device_windows_swipe", "Swipes on Windows screen", e.toolWindowsSwipe)
	// e.registerTool("device_windows_input", "Inputs text on Windows", e.toolWindowsInput)
	// e.registerTool("device_windows_key", "Presses a key on Windows", e.toolWindowsKey)
	// e.registerTool("device_windows_launch", "Launches an app on Windows", e.toolWindowsLaunch)
	// e.registerTool("device_windows_close", "Closes an app on Windows", e.toolWindowsClose)
	// e.registerTool("device_windows_screenshot", "Takes a screenshot of Windows screen", e.toolWindowsScreenshot)

	// Android device tools
	e.registerTool("device_android_connect", "Connects to an Android device via ADB", e.toolAndroidConnect)
	e.registerTool("device_android_tap", "Taps on Android screen at coordinates", e.toolAndroidTap)
	e.registerTool("device_android_swipe", "Swipes on Android screen", e.toolAndroidSwipe)
	e.registerTool("device_android_input", "Inputs text on Android", e.toolAndroidInput)
	e.registerTool("device_android_key", "Presses a key on Android", e.toolAndroidKey)
	e.registerTool("device_android_launch", "Launches an app on Android", e.toolAndroidLaunch)
	e.registerTool("device_android_close", "Closes an app on Android", e.toolAndroidClose)
	e.registerTool("device_android_screenshot", "Takes a screenshot of Android screen", e.toolAndroidScreenshot)

	// Browser device tools
	e.registerTool("device_browser_connect", "Connects to a browser for automation", e.toolBrowserConnect)
	e.registerBrowserTool("device_browser_navigate", "Navigates to a URL in the browser. Use this first when user asks to view/open a webpage.", map[string]interface{}{
		"url": map[string]interface{}{"type": "string", "description": "Full URL to navigate to (e.g. https://www.baidu.com)"},
	}, e.toolBrowserNavigate)
	e.registerTool("device_browser_click_element", "Clicks an element by CSS selector", e.toolBrowserClickElement)
	e.registerTool("device_browser_input_to", "Inputs text into an element", e.toolBrowserInputTo)
	e.registerBrowserTool("device_browser_get_text", "Extracts plain text from a CSS selector. For analyzing webpage content (e.g. hot news, headlines): use selector 'body' to get full page text, then analyze. Prefer this over screenshot for text-based analysis.", map[string]interface{}{
		"selector": map[string]interface{}{"type": "string", "description": "CSS selector. Use 'body' for full page text when analyzing content. Default: body"},
	}, e.toolBrowserGetText)
	e.registerBrowserTool("device_browser_get_html", "Extracts HTML from a CSS selector. For analyzing webpage content (e.g. hot news, headlines): use selector 'body' to get full page HTML, then analyze. Prefer this over screenshot for text-based analysis.", map[string]interface{}{
		"selector": map[string]interface{}{"type": "string", "description": "CSS selector. Use 'body' for full page when analyzing content. Default: body"},
	}, e.toolBrowserGetHTML)
	e.registerTool("device_browser_get_url", "Gets the current page URL", e.toolBrowserGetURL)
	e.registerTool("device_browser_get_title", "Gets the current page title", e.toolBrowserGetTitle)
	e.registerTool("device_browser_screenshot", "Takes a screenshot of the browser page. Returns image URL. Use for visual reference; for text extraction/analysis prefer device_browser_get_text or device_browser_get_html.", e.toolBrowserScreenshot)
	e.registerTool("device_browser_eval", "Executes JavaScript in the browser", e.toolBrowserEval)
	e.registerTool("device_browser_wait_for", "Waits for an element to appear", e.toolBrowserWaitFor)
	e.registerTool("device_browser_select_option", "Selects an option from a select element", e.toolBrowserSelectOption)
	e.registerTool("device_browser_tap", "Taps/clicks at coordinates in browser", e.toolBrowserTap)
	e.registerTool("device_browser_swipe", "Scrolls in the browser", e.toolBrowserSwipe)
	e.registerTool("device_browser_input", "Inputs text in the browser", e.toolBrowserInput)
	e.registerTool("device_browser_key", "Presses a key in the browser", e.toolBrowserKey)

	// Screen automation tools (vision-based)
	e.registerBrowserTool("device_screen_snapshot", "Takes a screenshot of the connected device (browser/android/windows) and uploads to OSS. Returns image URL for use with device_screen_analyze. Call after device_browser_connect or device_windows_connect.", map[string]interface{}{
		"device_type": map[string]interface{}{"type": "string", "description": "Device type: browser, android, or windows. Default: browser"},
	}, e.toolScreenSnapshot)
	e.registerBrowserTool("device_screen_analyze", "Analyzes a screenshot (from device_screen_snapshot or device_browser_screenshot) and returns UI elements with coordinates. Use image_url from screenshot result. Returns structured elements for tap/click decisions.", map[string]interface{}{
		"image_url":  map[string]interface{}{"type": "string", "description": "Image URL from device_screen_snapshot or device_browser_screenshot"},
		"task_hint":  map[string]interface{}{"type": "string", "description": "Optional hint for current task, e.g. 'identify search box' or 'find first non-ad result'"},
	}, e.toolScreenAnalyze)
	e.registerBrowserTool("device_screen_wait", "Waits for specified seconds before next action. Use after tap/input to allow page to load.", map[string]interface{}{
		"seconds": map[string]interface{}{"type": "number", "description": "Seconds to wait (1-10). Default: 2"},
	}, e.toolScreenWait)

	log.Info().Msg("registered device control tools")
}

// registerTool is a helper to register a tool with basic metadata
func (e *ToolExecutor) registerTool(name, description string, handler agent.ToolHandler) {
	e.engine.RegisterTool(name, description, map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}, handler)
}

// registerBrowserTool registers a browser tool with optional param schema for LLM
func (e *ToolExecutor) registerBrowserTool(name, description string, params map[string]interface{}, handler agent.ToolHandler) {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": params,
	}
	e.engine.RegisterTool(name, description, schema, handler)
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

	if img == nil {
		return "Screenshot captured (no image data)", nil
	}
	url, err := e.uploadScreenshot(ctx, img, "android")
	if err != nil {
		return "", err
	}
	log.Info().Int("width", img.Bounds().Dx()).Int("height", img.Bounds().Dy()).Str("url", url).Msg("android screenshot uploaded")
	return fmt.Sprintf("Screenshot captured: %dx%d\nImage URL: %s", img.Bounds().Dx(), img.Bounds().Dy(), url), nil
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
	selector := getString(params, "selector", "body")
	if selector == "" {
		selector = "body"
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
	selector := getString(params, "selector", "body")
	if selector == "" {
		selector = "body"
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

	url, err := e.uploadScreenshot(ctx, img, "browser")
	if err != nil {
		return "", err
	}
	log.Info().Int("width", img.Bounds().Dx()).Int("height", img.Bounds().Dy()).Str("url", url).Msg("browser screenshot uploaded")
	return fmt.Sprintf("Browser screenshot captured: %dx%d\nImage URL: %s", img.Bounds().Dx(), img.Bounds().Dy(), url), nil
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

// screen analysis response structure for JSON parsing
type screenAnalyzeResponse struct {
	Elements    []screenElement `json:"elements"`
	PageSummary string          `json:"page_summary"`
}

type screenElement struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	Bounds   bounds `json:"bounds"`
	Clickable bool  `json:"clickable"`
}

type bounds struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// toolScreenAnalyze analyzes a screenshot and returns structured UI elements
func (e *ToolExecutor) toolScreenAnalyze(ctx context.Context, params map[string]interface{}) (string, error) {
	imageURL := getString(params, "image_url", "")
	taskHint := getString(params, "task_hint", "")

	if imageURL == "" {
		return "", fmt.Errorf("image_url is required (use URL from device_screen_snapshot or device_browser_screenshot)")
	}

	if e.visionProvider == nil {
		return "", fmt.Errorf("vision provider not configured, cannot analyze screen")
	}

	prompt := `Analyze this screen screenshot and return a JSON object with this exact structure. No other text.
{
  "elements": [
    {"id": 1, "type": "button|input|link|text|other", "text": "visible text", "bounds": {"x": 0, "y": 0, "w": 100, "h": 40}, "clickable": true}
  ],
  "page_summary": "Brief description of the page"
}

Rules:
- id: sequential integer starting from 1
- type: button, input, link, text, or other
- text: visible text on the element (empty string if none)
- bounds: x,y = top-left corner, w,h = width and height in pixels. Estimate from image dimensions.
- clickable: true for buttons, links, inputs; false for plain text
- Include all interactive elements: buttons, inputs, links, search boxes
- For search result pages, mark ads (e.g. "广告" label) in text, use type "link" for result links`

	if taskHint != "" {
		prompt += fmt.Sprintf("\n\nCurrent task hint: %s. Pay special attention to elements relevant to this task.", taskHint)
	}

	prompt += "\n\nReturn ONLY valid JSON, no markdown or explanation."

	req := provider.ChatCompletionRequest{
		Model: e.visionModel,
		Messages: []provider.Message{
			{
				Role: provider.RoleUser,
				ContentParts: []provider.ContentPart{
					provider.NewImageURLPart(imageURL),
					{Type: "text", Text: prompt},
				},
			},
		},
		Temperature: 0.2,
		MaxTokens:   2000,
	}

	resp, err := e.visionProvider.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("vision API failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from vision API")
	}

	content := resp.Choices[0].Message.Content
	if content == "" && len(resp.Choices[0].Message.ContentParts) > 0 {
		for _, p := range resp.Choices[0].Message.ContentParts {
			if p.Type == "text" && p.Text != "" {
				content = p.Text
				break
			}
		}
	}

	if content == "" {
		return "", fmt.Errorf("empty response from vision API")
	}

	// Try to extract JSON from response (model might wrap in markdown)
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		jsonStr = content
	}

	var parsed screenAnalyzeResponse
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		log.Debug().Err(err).Str("raw", content).Msg("screen analyze JSON parse failed, returning raw")
		return content, nil
	}

	// Format for LLM consumption
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Page summary: %s\n\n", parsed.PageSummary))
	sb.WriteString("Elements (use bounds.x, bounds.y center for tap/click):\n")
	for _, el := range parsed.Elements {
		cx, cy := el.Bounds.X+el.Bounds.W/2, el.Bounds.Y+el.Bounds.H/2
		sb.WriteString(fmt.Sprintf("  [%d] type=%s text=%q bounds=(%d,%d,%d,%d) center=(%d,%d) clickable=%v\n",
			el.ID, el.Type, el.Text, el.Bounds.X, el.Bounds.Y, el.Bounds.W, el.Bounds.H, cx, cy, el.Clickable))
	}
	return sb.String(), nil
}

// extractJSON extracts JSON object from text (handles markdown code blocks)
func extractJSON(s string) string {
	// Try ```json ... ``` first
	re := regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// Try raw { ... }
	start := strings.Index(s, "{")
	if start >= 0 {
		depth := 0
		for i := start; i < len(s); i++ {
			if s[i] == '{' {
				depth++
			} else if s[i] == '}' {
				depth--
				if depth == 0 {
					return s[start : i+1]
				}
			}
		}
	}
	return ""
}

// toolScreenSnapshot captures screenshot from connected device and uploads
func (e *ToolExecutor) toolScreenSnapshot(ctx context.Context, params map[string]interface{}) (string, error) {
	deviceType := getString(params, "device_type", "browser")

	if e.imageUploader == nil {
		return "", fmt.Errorf("OSS 未配置，无法上传截图。请在配置中设置 OSS 后重试")
	}

	var dev Device
	switch deviceType {
	case "browser":
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformBrowser {
				dev = d
				break
			}
		}
	case "android":
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformAndroid {
				dev = d
				break
			}
		}
	case "windows":
		for _, id := range e.manager.List() {
			if d, ok := e.manager.Get(id); ok && d.Platform() == types.PlatformWindows {
				dev = d
				break
			}
		}
	default:
		return "", fmt.Errorf("device_type must be browser, android, or windows")
	}

	if dev == nil {
		return "", fmt.Errorf("no %s device connected. Connect first with device_%s_connect", deviceType, deviceType)
	}

	img, err := dev.Screenshot(ctx)
	if err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}

	url, err := e.uploadScreenshot(ctx, img, "screen_"+deviceType)
	if err != nil {
		return "", err
	}

	log.Info().Str("device_type", deviceType).Int("width", img.Bounds().Dx()).Int("height", img.Bounds().Dy()).Str("url", url).Msg("screen snapshot uploaded")
	return fmt.Sprintf("Screen snapshot: %dx%d\nImage URL: %s", img.Bounds().Dx(), img.Bounds().Dy(), url), nil
}

// toolScreenWait waits for specified seconds
func (e *ToolExecutor) toolScreenWait(ctx context.Context, params map[string]interface{}) (string, error) {
	sec := getInt(params, "seconds", 2)
	if sec < 1 {
		sec = 1
	}
	if sec > 10 {
		sec = 10
	}
	time.Sleep(time.Duration(sec) * time.Second)
	return fmt.Sprintf("Waited %d seconds", sec), nil
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
