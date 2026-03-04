package device

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/device/playwright"
)

// SetPlaywrightProcess sets the Playwright sidecar process for browser automation.
func (e *ToolExecutor) SetPlaywrightProcess(p *playwright.Process) {
	e.playwrightProcess = p
}

// pwClient returns a connected Playwright client and ensures browser is launched.
func (e *ToolExecutor) pwClient(ctx context.Context) (*playwright.Client, error) {
	if e.playwrightProcess == nil {
		return nil, fmt.Errorf("playwright sidecar not configured (SetPlaywrightProcess not called)")
	}
	client, err := e.playwrightProcess.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("playwright client: %w", err)
	}
	// Ensure browser is launched (idempotent)
	if err := client.Call(ctx, "browser.launch", map[string]interface{}{"headless": true}, nil); err != nil {
		return nil, fmt.Errorf("browser launch: %w", err)
	}
	return client, nil
}

// toolBrowserNavigate (Playwright) navigates to a URL.
func (e *ToolExecutor) toolBrowserNavigatePW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	url := getString(params, "url", "")
	if url == "" {
		return "", fmt.Errorf("url parameter is required")
	}
	var result struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := client.Call(ctx, "page.navigate", map[string]interface{}{"url": url}, &result); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}
	return fmt.Sprintf("Navigated to: %s\nTitle: %s", result.URL, result.Title), nil
}

// toolBrowserSnapshot returns the page's interactive elements as numbered refs.
func (e *ToolExecutor) toolBrowserSnapshot(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := client.Call(ctx, "page.snapshot", map[string]interface{}{}, &result); err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	return result.Text, nil
}

// toolBrowserClickPW clicks an element by ref number.
func (e *ToolExecutor) toolBrowserClickPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	ref := getInt(params, "ref", 0)
	if ref <= 0 {
		return "", fmt.Errorf("ref parameter is required (use number from device_browser_snapshot)")
	}
	if err := client.Call(ctx, "page.click", map[string]interface{}{"ref": ref}, nil); err != nil {
		return "", fmt.Errorf("click: %w", err)
	}
	return fmt.Sprintf("Clicked element [%d]", ref), nil
}

// toolBrowserFillPW fills an input by ref number.
func (e *ToolExecutor) toolBrowserFillPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	ref := getInt(params, "ref", 0)
	text := getString(params, "text", "")
	if ref <= 0 {
		return "", fmt.Errorf("ref parameter is required (use number from device_browser_snapshot)")
	}
	if err := client.Call(ctx, "page.fill", map[string]interface{}{"ref": ref, "text": text}, nil); err != nil {
		return "", fmt.Errorf("fill: %w", err)
	}
	return fmt.Sprintf("Filled element [%d] with: %s", ref, text), nil
}

// toolBrowserGetTextPW gets text from a selector.
func (e *ToolExecutor) toolBrowserGetTextPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	selector := getString(params, "selector", "body")
	var result struct {
		Text string `json:"text"`
	}
	if err := client.Call(ctx, "page.getText", map[string]interface{}{"selector": selector}, &result); err != nil {
		return "", fmt.Errorf("getText: %w", err)
	}
	return result.Text, nil
}

// toolBrowserGetHTMLPW gets HTML from a selector.
func (e *ToolExecutor) toolBrowserGetHTMLPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	selector := getString(params, "selector", "body")
	var result struct {
		HTML string `json:"html"`
	}
	if err := client.Call(ctx, "page.getHTML", map[string]interface{}{"selector": selector}, &result); err != nil {
		return "", fmt.Errorf("getHTML: %w", err)
	}
	return result.HTML, nil
}

// toolBrowserGetURLPW gets current URL.
func (e *ToolExecutor) toolBrowserGetURLPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := client.Call(ctx, "page.getUrl", map[string]interface{}{}, &result); err != nil {
		return "", fmt.Errorf("getUrl: %w", err)
	}
	return result.URL, nil
}

// toolBrowserGetTitlePW gets page title.
func (e *ToolExecutor) toolBrowserGetTitlePW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	var result struct {
		Title string `json:"title"`
	}
	if err := client.Call(ctx, "page.getTitle", map[string]interface{}{}, &result); err != nil {
		return "", fmt.Errorf("getTitle: %w", err)
	}
	return result.Title, nil
}

// toolBrowserScreenshotPW takes screenshot, optionally uploads to OSS.
func (e *ToolExecutor) toolBrowserScreenshotPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	fullPage := getBool(params, "full_page", false)
	var result struct {
		Base64 string `json:"base64"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	if err := client.Call(ctx, "page.screenshot", map[string]interface{}{"fullPage": fullPage}, &result); err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	if e.imageUploader != nil {
		data, err := base64.StdEncoding.DecodeString(result.Base64)
		if err != nil {
			return "", fmt.Errorf("decode screenshot: %w", err)
		}
		filename := fmt.Sprintf("screenshot_browser_%d.png", time.Now().UnixNano())
		url, err := e.imageUploader.UploadImagePNG(data, filename)
		if err != nil {
			return "", fmt.Errorf("upload screenshot: %w", err)
		}
		log.Info().Int("width", result.Width).Int("height", result.Height).Str("url", url).Msg("browser screenshot uploaded")
		return fmt.Sprintf("Screenshot: %dx%d\nImage URL: %s", result.Width, result.Height, url), nil
	}
	return fmt.Sprintf("Screenshot captured: %dx%d (OSS not configured, base64 omitted)", result.Width, result.Height), nil
}

// toolBrowserEvalPW executes JavaScript.
func (e *ToolExecutor) toolBrowserEvalPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	script := getString(params, "script", "")
	if script == "" {
		return "", fmt.Errorf("script parameter is required")
	}
	var result struct {
		Result interface{} `json:"result"`
	}
	if err := client.Call(ctx, "page.evaluate", map[string]interface{}{"script": script}, &result); err != nil {
		return "", fmt.Errorf("eval: %w", err)
	}
	return fmt.Sprintf("Result: %v", result.Result), nil
}

// toolBrowserWaitForPW waits for a selector.
func (e *ToolExecutor) toolBrowserWaitForPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	selector := getString(params, "selector", "")
	timeoutMs := getInt(params, "timeout_ms", 10000)
	if selector == "" {
		return "", fmt.Errorf("selector parameter is required")
	}
	if err := client.Call(ctx, "page.waitForSelector", map[string]interface{}{"selector": selector, "timeout": timeoutMs}, nil); err != nil {
		return "", fmt.Errorf("waitForSelector: %w", err)
	}
	return fmt.Sprintf("Element found: %s", selector), nil
}

// toolBrowserWaitPW waits for N seconds.
func (e *ToolExecutor) toolBrowserWaitPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	sec := getInt(params, "seconds", 2)
	if sec < 1 {
		sec = 1
	}
	if sec > 10 {
		sec = 10
	}
	if err := client.Call(ctx, "page.wait", map[string]interface{}{"seconds": sec}, nil); err != nil {
		return "", fmt.Errorf("wait: %w", err)
	}
	return fmt.Sprintf("Waited %d seconds", sec), nil
}

// toolBrowserSelectOptionPW selects an option by ref.
func (e *ToolExecutor) toolBrowserSelectOptionPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	ref := getInt(params, "ref", 0)
	value := getString(params, "value", "")
	if ref <= 0 {
		return "", fmt.Errorf("ref parameter is required")
	}
	if value == "" {
		return "", fmt.Errorf("value parameter is required")
	}
	if err := client.Call(ctx, "page.select", map[string]interface{}{"ref": ref, "value": value}, nil); err != nil {
		return "", fmt.Errorf("select: %w", err)
	}
	return fmt.Sprintf("Selected '%s' in element [%d]", value, ref), nil
}

// toolBrowserKeyPW presses a key.
func (e *ToolExecutor) toolBrowserKeyPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	key := getString(params, "key", "")
	if key == "" {
		return "", fmt.Errorf("key parameter is required")
	}
	if err := client.Call(ctx, "page.pressKey", map[string]interface{}{"key": key}, nil); err != nil {
		return "", fmt.Errorf("pressKey: %w", err)
	}
	return fmt.Sprintf("Pressed key: %s", key), nil
}

// toolBrowserScrollPW scrolls the page.
func (e *ToolExecutor) toolBrowserScrollPW(ctx context.Context, params map[string]interface{}) (string, error) {
	client, err := e.pwClient(ctx)
	if err != nil {
		return "", err
	}
	direction := getString(params, "direction", "down")
	amount := getInt(params, "amount", 300)
	if direction != "up" && direction != "down" {
		direction = "down"
	}
	if err := client.Call(ctx, "page.scroll", map[string]interface{}{"direction": direction, "amount": amount}, nil); err != nil {
		return "", fmt.Errorf("scroll: %w", err)
	}
	return fmt.Sprintf("Scrolled %s", direction), nil
}
