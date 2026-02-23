package browser

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/device/types"
)

// Device implements Device interface for Browser automation
type Device struct {
	ctx       context.Context
	cancel    context.CancelFunc
	connected bool
	remoteURL string
	headless  bool
}

// NewDevice creates a new browser device controller
func NewDevice(remoteURL string, headless bool) *Device {
	return &Device{
		remoteURL: remoteURL,
		headless:  headless,
	}
}

// Connect establishes connection to the browser
func (d *Device) Connect(ctx context.Context) error {
	log.Info().
		Str("remote_url", d.remoteURL).
		Bool("headless", d.headless).
		Msg("connecting to browser")

	// Allocate options
	allocOpts := []chromedp.ExecAllocatorOption{}
	ctxOpts := []chromedp.ContextOption{}

	if d.remoteURL != "" {
		// Connect to remote browser - parse the URL and setup allocator
		allocOpts = append(allocOpts,
			chromedp.ProxyServer(d.remoteURL),
		)
	} else {
		// Launch local browser with comprehensive options
		allocOpts = append(allocOpts,
			// Headless mode
			chromedp.Flag("headless", d.headless),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("mute-audio", true),
			// Window size for consistent rendering
			chromedp.Flag("window-size", "1920,1080"),
			// Disable various Chrome features for stability
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("disable-software-rasterizer", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-background-timer-throttling", true),
			chromedp.Flag("disable-backgrounding-occluded-windows", true),
			chromedp.Flag("disable-renderer-backgrounding", true),
			// Prevent detection
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			// User agent to appear as a normal browser
			chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
			// Ignore certificate errors for testing
			chromedp.Flag("ignore-certificate-errors", true),
		)
	}

	// Create allocator
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)

	// Create context
	d.ctx, d.cancel = chromedp.NewContext(allocCtx, ctxOpts...)

	// Test connection
	err := chromedp.Run(d.ctx)
	if err != nil {
		allocCancel()
		return fmt.Errorf("failed to connect to browser: %w", err)
	}

	d.connected = true
	log.Info().Msg("connected to browser device")
	return nil
}

// Disconnect closes the connection
func (d *Device) Disconnect(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	d.connected = false
	log.Info().Msg("disconnected from browser device")
	return nil
}

// Screenshot captures the current page
func (d *Device) Screenshot(ctx context.Context) (*image.RGBA, error) {
	if !d.connected {
		return nil, fmt.Errorf("device not connected")
	}

	var imgData []byte
	err := chromedp.Run(d.ctx,
		chromedp.FullScreenshot(&imgData, 90),
	)
	if err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Convert to RGBA if needed
	rgba, ok := img.(*image.RGBA)
	if !ok {
		bounds := img.Bounds()
		rgba = image.NewRGBA(bounds)
		draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	}

	log.Info().
		Int("width", rgba.Bounds().Dx()).
		Int("height", rgba.Bounds().Dy()).
		Msg("browser screenshot captured")

	return rgba, nil
}

// Tap clicks at coordinates
func (d *Device) Tap(ctx context.Context, x, y int) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Int("x", x).Int("y", y).Msg("browser mouse click")

	err := chromedp.Run(d.ctx,
		chromedp.MouseClickXY(float64(x), float64(y)),
	)
	if err != nil {
		return fmt.Errorf("click failed: %w", err)
	}

	return nil
}

// Swipe performs a scroll gesture
func (d *Device) Swipe(ctx context.Context, x1, y1, x2, y2 int, duration time.Duration) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().
		Int("x1", x1).Int("y1", y1).
		Int("x2", x2).Int("y2", y2).
		Dur("duration", duration).
		Msg("browser scroll/swipe")

	// Calculate scroll direction
	dx := x2 - x1
	dy := y2 - y1

	// Use JavaScript to scroll smoothly
	script := fmt.Sprintf(`
		(function() {
			const startX = window.scrollX || window.pageXOffset;
			const startY = window.scrollY || window.pageYOffset;
			const targetX = startX + %d;
			const targetY = startY + %d;
			const duration = %d;

			const startTime = performance.now();

			function scroll(currentTime) {
				const elapsed = currentTime - startTime;
				const progress = Math.min(elapsed / duration, 1);

				// Ease in-out function
				const ease = progress < 0.5
					? 2 * progress * progress
					: 1 - Math.pow(-2 * progress + 2, 2) / 2;

				window.scrollTo(
					startX + (targetX - startX) * ease,
					startY + (targetY - startY) * ease
				);

				if (progress < 1) {
					requestAnimationFrame(scroll);
				}
			}

			requestAnimationFrame(scroll);
		})();
	`, dx, dy, duration.Milliseconds())

	err := chromedp.Run(d.ctx,
		chromedp.Evaluate(script, nil),
	)
	if err != nil {
		return fmt.Errorf("scroll failed: %w", err)
	}

	return nil
}

// InputText types text
func (d *Device) InputText(ctx context.Context, text string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("text", text).Msg("browser input text")

	err := chromedp.Run(d.ctx,
		chromedp.SendKeys(`body`, text),
	)
	if err != nil {
		return fmt.Errorf("input text failed: %w", err)
	}

	return nil
}

// KeyPress simulates a key press
func (d *Device) KeyPress(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("key", key).Msg("browser key press")

	k := d.mapKey(key)

	err := chromedp.Run(d.ctx,
		chromedp.KeyEvent(k),
	)
	if err != nil {
		return fmt.Errorf("key press failed: %w", err)
	}

	return nil
}

// KeyDown holds a key down
func (d *Device) KeyDown(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	k := d.mapKey(key)
	err := chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchKeyEvent(input.KeyDown).
				WithKey(k).
				Do(ctx)
		}),
	)
	if err != nil {
		return fmt.Errorf("key down failed: %w", err)
	}

	return nil
}

// KeyUp releases a key
func (d *Device) KeyUp(ctx context.Context, key string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	k := d.mapKey(key)
	err := chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchKeyEvent(input.KeyUp).
				WithKey(k).
				Do(ctx)
		}),
	)
	if err != nil {
		return fmt.Errorf("key up failed: %w", err)
	}

	return nil
}

// LaunchApp opens a URL in the browser
func (d *Device) LaunchApp(ctx context.Context, appName string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Info().Str("url", appName).Msg("browser navigate")

	return d.Navigate(ctx, appName)
}

// CloseApp closes the current tab (not well supported in chromedp)
func (d *Device) CloseApp(ctx context.Context, appName string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Info().Msg("browser close page - navigating to blank")

	// Navigate to blank page as "close"
	err := chromedp.Run(d.ctx,
		chromedp.Navigate("about:blank"),
	)
	if err != nil {
		return fmt.Errorf("close page failed: %w", err)
	}

	return nil
}

// GetScreenSize returns the page viewport size
func (d *Device) GetScreenSize(ctx context.Context) (width, height int, err error) {
	if !d.connected {
		return 0, 0, fmt.Errorf("device not connected")
	}

	var size struct {
		Width  int64 `json:"width"`
		Height int64 `json:"height"`
	}

	err = chromedp.Run(d.ctx,
		chromedp.Evaluate(`({ width: window.innerWidth, height: window.innerHeight })`, &size),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get viewport size: %w", err)
	}

	return int(size.Width), int(size.Height), nil
}

// HealthCheck verifies the browser connection
func (d *Device) HealthCheck(ctx context.Context) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	// Try to get current URL
	var url string
	err := chromedp.Run(d.ctx,
		chromedp.Location(&url),
	)
	if err != nil {
		return fmt.Errorf("browser health check failed: %w", err)
	}

	log.Debug().Str("url", url).Msg("browser health check ok")
	return nil
}

// Platform returns the device platform
func (d *Device) Platform() types.Platform {
	return types.PlatformBrowser
}

// Navigate navigates to a URL
func (d *Device) Navigate(ctx context.Context, url string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("url", url).Msg("navigating")

	// Use Navigate action without waiting
	err := chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Navigate(url).Do(ctx)
		}),
	)
	if err != nil {
		return fmt.Errorf("navigate failed: %w", err)
	}

	return nil
}

// ClickElement clicks an element by CSS selector
func (d *Device) ClickElement(ctx context.Context, selector string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("selector", selector).Msg("clicking element")

	err := chromedp.Run(d.ctx,
		chromedp.Click(selector),
	)
	if err != nil {
		return fmt.Errorf("click element failed: %w", err)
	}

	return nil
}

// InputTo inputs text into an element
func (d *Device) InputTo(ctx context.Context, selector, text string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("selector", selector).Str("text", text).Msg("inputting to element")

	err := chromedp.Run(d.ctx,
		chromedp.Click(selector),
		chromedp.SendKeys(selector, text),
	)
	if err != nil {
		return fmt.Errorf("input to element failed: %w", err)
	}

	return nil
}

// GetText retrieves text from an element
func (d *Device) GetText(ctx context.Context, selector string) (string, error) {
	if !d.connected {
		return "", fmt.Errorf("device not connected")
	}

	var text string
	err := chromedp.Run(d.ctx,
		chromedp.Text(selector, &text),
	)
	if err != nil {
		return "", fmt.Errorf("get text failed: %w", err)
	}

	return text, nil
}

// GetHTML retrieves HTML from an element
func (d *Device) GetHTML(ctx context.Context, selector string) (string, error) {
	if !d.connected {
		return "", fmt.Errorf("device not connected")
	}

	var html string
	err := chromedp.Run(d.ctx,
		chromedp.InnerHTML(selector, &html),
	)
	if err != nil {
		return "", fmt.Errorf("get html failed: %w", err)
	}

	return html, nil
}

// GetURL returns the current page URL
func (d *Device) GetURL(ctx context.Context) (string, error) {
	if !d.connected {
		return "", fmt.Errorf("device not connected")
	}

	var url string
	err := chromedp.Run(d.ctx,
		chromedp.Location(&url),
	)
	if err != nil {
		return "", fmt.Errorf("get url failed: %w", err)
	}

	return url, nil
}

// GetTitle returns the current page title
func (d *Device) GetTitle(ctx context.Context) (string, error) {
	if !d.connected {
		return "", fmt.Errorf("device not connected")
	}

	var title string
	err := chromedp.Run(d.ctx,
		chromedp.Title(&title),
	)
	if err != nil {
		return "", fmt.Errorf("get title failed: %w", err)
	}

	return title, nil
}

// Eval executes JavaScript
func (d *Device) Eval(ctx context.Context, script string) (interface{}, error) {
	if !d.connected {
		return nil, fmt.Errorf("device not connected")
	}

	log.Debug().Str("script", script).Msg("evaluating script")

	var result interface{}
	err := chromedp.Run(d.ctx,
		chromedp.Evaluate(script, &result),
	)
	if err != nil {
		return nil, fmt.Errorf("eval failed: %w", err)
	}

	return result, nil
}

// WaitFor waits for an element
func (d *Device) WaitFor(ctx context.Context, selector string, timeout time.Duration) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("selector", selector).Msg("waiting for element")

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(d.ctx, timeout)
	defer cancel()

	err := chromedp.Run(timeoutCtx,
		chromedp.WaitVisible(selector),
	)
	if err != nil {
		return fmt.Errorf("wait for element failed: %w", err)
	}

	return nil
}

// SelectOption selects an option from a select element
func (d *Device) SelectOption(ctx context.Context, selector, value string) error {
	if !d.connected {
		return fmt.Errorf("device not connected")
	}

	log.Debug().Str("selector", selector).Str("value", value).Msg("selecting option")

	err := chromedp.Run(d.ctx,
		chromedp.SetValue(selector, value),
	)
	if err != nil {
		return fmt.Errorf("select option failed: %w", err)
	}

	return nil
}

// mapKey maps key names to chromedp key names
func (d *Device) mapKey(key string) string {
	// chromedp uses single character for simple keys
	keyMap := map[string]string{
		"enter":     "Enter",
		"return":    "Enter",
		"space":     " ",
		"tab":       "Tab",
		"escape":    "Escape",
		"esc":       "Escape",
		"backspace": "Backspace",
		"delete":    "Delete",
		"home":      "Home",
		"end":       "End",
		"pageup":    "PageUp",
		"pagedown":  "PageDown",
		"up":        "ArrowUp",
		"down":      "ArrowDown",
		"left":      "ArrowLeft",
		"right":     "ArrowRight",
		"f1":        "F1",
		"f2":        "F2",
		"f3":        "F3",
		"f4":        "F4",
		"f5":        "F5",
		"f6":        "F6",
		"f7":        "F7",
		"f8":        "F8",
		"f9":        "F9",
		"f10":       "F10",
		"f11":       "F11",
		"f12":       "F12",
		"ctrl":      "Control",
		"alt":       "Alt",
		"shift":     "Shift",
		"meta":      "Meta",
	}

	if k, ok := keyMap[key]; ok {
		return k
	}
	return key
}
