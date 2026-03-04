package device

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/device/android"
	"github.com/rocky/marstaff/internal/device/playwright"
	"github.com/rocky/marstaff/internal/device/types"
	"github.com/rocky/marstaff/internal/provider"
)

// ToolExecutor provides device control tools for the agent
type ToolExecutor struct {
	manager           *Manager
	engine            *agent.Engine
	imageUploader     ImageUploader
	visionProvider    provider.Provider
	visionModel       string
	playwrightProcess *playwright.Process
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
	e.registerBrowserTool("device_browser_navigate", "Opens a webpage in browser for viewing/interaction. Use ONLY when user wants to browse, view, or interact with a web page. Do NOT use for: Feishu webhook, API URLs, sending notifications, 发飞书, 发到飞书 — use afk_send_notification for those.", map[string]interface{}{
		"url": map[string]interface{}{"type": "string", "description": "Full URL to navigate to (e.g. https://www.baidu.com). Must be a browsable webpage, NOT webhook/API URLs."},
	}, e.toolBrowserNavigatePW)
	e.registerBrowserTool("device_browser_snapshot", "Returns all interactive elements with numbered refs. Use refs with device_browser_click and device_browser_fill.", map[string]interface{}{
		"focus_ref": map[string]interface{}{"type": "number", "description": "Optional ref to focus"},
	}, e.toolBrowserSnapshot)
	e.registerBrowserTool("device_browser_click", "Clicks element by ref from snapshot.", map[string]interface{}{
		"ref": map[string]interface{}{"type": "number", "description": "Element ref from snapshot"},
	}, e.toolBrowserClickPW)
	e.registerBrowserTool("device_browser_fill", "Fills input by ref from snapshot.", map[string]interface{}{
		"ref": map[string]interface{}{"type": "number", "description": "Element ref"},
		"text": map[string]interface{}{"type": "string", "description": "Text to fill"},
	}, e.toolBrowserFillPW)
	e.registerBrowserTool("device_browser_get_text", "Extracts plain text from a CSS selector. For analyzing webpage content (e.g. hot news, headlines): use selector 'body' to get full page text, then analyze. Prefer this over screenshot for text-based analysis.", map[string]interface{}{
		"selector": map[string]interface{}{"type": "string", "description": "CSS selector. Use 'body' for full page text when analyzing content. Default: body"},
	}, e.toolBrowserGetTextPW)
	e.registerBrowserTool("device_browser_get_html", "Extracts HTML from a CSS selector. For analyzing webpage content (e.g. hot news, headlines): use selector 'body' to get full page HTML, then analyze. Prefer this over screenshot for text-based analysis.", map[string]interface{}{
		"selector": map[string]interface{}{"type": "string", "description": "CSS selector. Use 'body' for full page when analyzing content. Default: body"},
	}, e.toolBrowserGetHTMLPW)
	e.registerTool("device_browser_get_url", "Gets the current page URL", e.toolBrowserGetURLPW)
	e.registerTool("device_browser_get_title", "Gets the current page title", e.toolBrowserGetTitlePW)
	e.registerTool("device_browser_screenshot", "Takes a screenshot of the browser page. Returns image URL. Use for visual reference; for text extraction/analysis prefer device_browser_get_text or device_browser_get_html.", e.toolBrowserScreenshotPW)
	e.registerTool("device_browser_eval", "Executes JavaScript in the browser", e.toolBrowserEvalPW)
	e.registerBrowserTool("device_browser_wait_for", "Waits for element to appear", map[string]interface{}{
		"selector":   map[string]interface{}{"type": "string", "description": "CSS selector"},
		"timeout_ms": map[string]interface{}{"type": "number", "description": "Timeout ms. Default: 10000"},
	}, e.toolBrowserWaitForPW)
	e.registerBrowserTool("device_browser_wait", "Waits N seconds. Use after click/fill.", map[string]interface{}{
		"seconds": map[string]interface{}{"type": "number", "description": "Seconds 1-10. Default: 2"},
	}, e.toolBrowserWaitPW)
	e.registerBrowserTool("device_browser_select_option", "Selects option in select by ref.", map[string]interface{}{
		"ref":   map[string]interface{}{"type": "number", "description": "Select ref from snapshot"},
		"value": map[string]interface{}{"type": "string", "description": "Option value"},
	}, e.toolBrowserSelectOptionPW)
	e.registerTool("device_browser_key", "Presses a key in the browser", e.toolBrowserKeyPW)
	e.registerBrowserTool("device_browser_scroll", "Scrolls page up or down.", map[string]interface{}{
		"direction": map[string]interface{}{"type": "string", "description": "up or down"},
		"amount":   map[string]interface{}{"type": "number", "description": "Pixels. Default: 300"},
	}, e.toolBrowserScrollPW)

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
