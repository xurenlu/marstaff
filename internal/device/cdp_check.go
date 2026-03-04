package device

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// CDPCheckResult holds the result of checking if Chrome CDP is running on a port
type CDPCheckResult struct {
	PortAvailable bool   `json:"port_available"` // Port is listening (something is using it)
	IsChromeCDP   bool   `json:"is_chrome_cdp"`  // Response from /json/version looks like Chrome DevTools
	Message       string `json:"message"`
}

// CheckCDPPort checks if the given port has Chrome running with remote debugging.
// 1. Tries to connect to localhost:port - if connection fails, port is not in use
// 2. If port is in use, GET http://localhost:port/json/version - if it returns JSON with "Browser" field, it's Chrome CDP
func CheckCDPPort(ctx context.Context, port int) CDPCheckResult {
	if port <= 0 || port > 65535 {
		return CDPCheckResult{
			PortAvailable: false,
			IsChromeCDP:  false,
			Message:      "端口号无效",
		}
	}

	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return CDPCheckResult{
			PortAvailable: false,
			IsChromeCDP:  false,
			Message:      fmt.Sprintf("端口 %d 未被占用，请先启动带远程调试的 Chrome", port),
		}
	}
	conn.Close()

	// Port is in use - check if it's Chrome CDP
	url := fmt.Sprintf("http://%s/json/version", addr)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return CDPCheckResult{
			PortAvailable: true,
			IsChromeCDP:   false,
			Message:       "无法创建请求",
		}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return CDPCheckResult{
			PortAvailable: true,
			IsChromeCDP:   false,
			Message:       fmt.Sprintf("端口 %d 有进程占用，但非 Chrome 远程调试（无法访问 /json/version）", port),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CDPCheckResult{
			PortAvailable: true,
			IsChromeCDP:   false,
			Message:       fmt.Sprintf("端口 %d 有进程占用，但返回状态 %d，非 Chrome CDP", port, resp.StatusCode),
		}
	}

	var version struct {
		Browser string `json:"Browser"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return CDPCheckResult{
			PortAvailable: true,
			IsChromeCDP:   false,
			Message:       fmt.Sprintf("端口 %d 有进程占用，但响应不是 Chrome CDP 格式", port),
		}
	}

	if version.Browser == "" {
		return CDPCheckResult{
			PortAvailable: true,
			IsChromeCDP:   false,
			Message:       "响应中缺少 Browser 字段，可能不是 Chrome",
		}
	}

	return CDPCheckResult{
		PortAvailable: true,
		IsChromeCDP:   true,
		Message:       fmt.Sprintf("端口 %d 检测到 Chrome 远程调试（%s）", port, version.Browser),
	}
}
