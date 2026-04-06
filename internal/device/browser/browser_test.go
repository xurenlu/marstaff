package browser

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestBrowserConnectAndNavigate 测试浏览器连接和导航功能
func TestBrowserConnectAndNavigate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser test in short mode (requires real browser)")
	}
	// 创建浏览器设备 (headless 模式)
	device := NewDevice("", true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试连接
	t.Run("Connect", func(t *testing.T) {
		if err := device.Connect(ctx); err != nil {
			t.Fatalf("Failed to connect to browser: %v", err)
		}
		if !device.connected {
			t.Fatal("Device should be connected after successful Connect()")
		}
		t.Log("✓ Browser connected successfully")
	})

	// 使用 example.com 作为冒烟导航（部分 Chromium 版本会拒绝 about:blank）
	t.Run("Navigate to example.com", func(t *testing.T) {
		if err := device.Navigate(ctx, "https://example.com"); err != nil {
			t.Fatalf("Failed to navigate to example.com: %v", err)
		}
		t.Log("✓ Navigated to example.com")
	})

	// 测试获取页面标题
	t.Run("Get Title", func(t *testing.T) {
		title, err := device.GetTitle(ctx)
		if err != nil {
			t.Fatalf("Failed to get page title: %v", err)
		}
		if title == "" {
			t.Fatal("Page title should not be empty")
		}
		t.Logf("✓ Page title: %s", title)
	})

	// 测试获取当前 URL
	t.Run("Get URL", func(t *testing.T) {
		url, err := device.GetURL(ctx)
		if err != nil {
			t.Fatalf("Failed to get current URL: %v", err)
		}
		if !strings.Contains(url, "example.com") {
			t.Errorf("Expected URL to contain 'example.com', got: %s", url)
		}
		t.Logf("✓ Current URL: %s", url)
	})

	// 测试获取页面文本内容
	t.Run("Get Body Text", func(t *testing.T) {
		text, err := device.GetText(ctx, "body")
		if err != nil {
			t.Fatalf("Failed to get body text: %v", err)
		}
		if text == "" {
			t.Fatal("Body text should not be empty")
		}
		// 验证是否包含 example.com 相关内容
		if !strings.Contains(text, "Example") && !strings.Contains(text, "example") {
			t.Logf("Warning: Body text doesn't contain expected keywords. Got: %s", text[:100])
		}
		t.Logf("✓ Body text length: %d characters", len(text))
	})

	// 测试执行 JavaScript
	t.Run("Evaluate JavaScript", func(t *testing.T) {
		result, err := device.Eval(ctx, "document.title")
		if err != nil {
			t.Fatalf("Failed to evaluate JavaScript: %v", err)
		}
		if result == nil {
			t.Fatal("JavaScript result should not be nil")
		}
		t.Logf("✓ JavaScript result: %v", result)
	})

	// 测试截图功能
	t.Run("Screenshot", func(t *testing.T) {
		img, err := device.Screenshot(ctx)
		if err != nil {
			t.Fatalf("Failed to take screenshot: %v", err)
		}
		if img == nil {
			t.Fatal("Screenshot should not be nil")
		}
		bounds := img.Bounds()
		if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
			t.Fatalf("Invalid screenshot dimensions: %dx%d", bounds.Dx(), bounds.Dy())
		}
		t.Logf("✓ Screenshot captured: %dx%d pixels", bounds.Dx(), bounds.Dy())
	})

	// 清理：断开连接
	t.Run("Disconnect", func(t *testing.T) {
		if err := device.Disconnect(ctx); err != nil {
			t.Fatalf("Failed to disconnect: %v", err)
		}
		if device.connected {
			t.Fatal("Device should not be connected after Disconnect()")
		}
		t.Log("✓ Browser disconnected successfully")
	})
}

// TestExampleDomain 测试访问 example.com 页面
func TestExampleDomain(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser test in short mode (requires real browser)")
	}
	device := NewDevice("", true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 连接并导航
	if err := device.Connect(ctx); err != nil {
		t.Skipf("Skipping test: failed to connect to browser: %v", err)
	}
	defer device.Disconnect(ctx)

	if err := device.Navigate(ctx, "https://example.com"); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// 等待页面加载
	time.Sleep(2 * time.Second)

	// 尝试获取热点新闻链接
	t.Run("Get News Links", func(t *testing.T) {
		// 使用 JavaScript 获取所有新闻链接
		script := `
			(function() {
				var links = [];
				var elements = document.querySelectorAll('a[href]');
				for (var i = 0; i < Math.min(elements.length, 50); i++) {
					var href = elements[i].href;
					var text = elements[i].textContent.trim();
					if (text && text.length > 0 && text.length < 100) {
						links.push({text: text, href: href});
					}
				}
				return links;
			})();
		`

		result, err := device.Eval(ctx, script)
		if err != nil {
			t.Fatalf("Failed to execute JavaScript: %v", err)
		}

		t.Logf("✓ Found links on page: %v", result)

		// 尝试获取特定新闻区域的文本
		bodyText, err := device.GetText(ctx, "body")
		if err != nil {
			t.Fatalf("Failed to get body text: %v", err)
		}

		// 打印前 500 个字符作为示例
		preview := bodyText
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		t.Logf("✓ Page content preview:\n%s", preview)
	})
}

// TestBrowserHealthCheck 测试健康检查
func TestBrowserHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser test in short mode (requires real browser)")
	}
	device := NewDevice("", true)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := device.Connect(ctx); err != nil {
		t.Skipf("Skipping test: failed to connect: %v", err)
	}
	defer device.Disconnect(ctx)

	if err := device.HealthCheck(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}
	t.Log("✓ Health check passed")
}

// BenchmarkExampleNavigation 性能基准测试
func BenchmarkExampleNavigation(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping browser benchmark in short mode (requires real browser)")
	}
	device := NewDevice("", true)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	if err := device.Connect(ctx); err != nil {
		b.Skipf("Skipping benchmark: failed to connect: %v", err)
	}
	defer device.Disconnect(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := device.Navigate(ctx, "https://example.com"); err != nil {
			b.Errorf("Navigation failed: %v", err)
		}
	}
}

// 运行示例：go test -v ./internal/device/browser/
// 或运行单个测试：go test -v ./internal/device/browser/ -run TestBrowserConnectAndNavigate
// 或运行基准测试：go test -bench=. ./internal/device/browser/
