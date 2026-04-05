package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	maxnetBaseURL         = "https://stat.maxnet.ru"
	maxnetLoginPath       = "/cgi-bin/index.cgi"
	maxnetPinPath         = "/cgi-bin/edit.cgi"
	maxnetSessionTimeout  = 30 * time.Minute
	maxnetPinCacheTTL     = 55 * time.Minute
	maxnetHTTPTimeout     = 10 * time.Second
)

var pinPatterns = []*regexp.Regexp{
	regexp.MustCompile(`PIN[:\s]*(\d{4})`),
	regexp.MustCompile(`(?i)ПИН[:\s]*(\d{4})`),
	regexp.MustCompile(`(?i)код[:\s]*(\d{4})`),
	regexp.MustCompile(`\b(\d{4})\b`),
}

type pinCache struct {
	Pin       string `json:"pin"`
	Timestamp int64  `json:"timestamp"`
}

type sessionCache struct {
	Timestamp int64 `json:"timestamp"`
}

// MaxnetTool retrieves WiFi PIN codes from Maxnet ISP (stat.maxnet.ru).
type MaxnetTool struct {
	username     string
	password     string
	cacheDir     string
	client       *http.Client
	pinCacheFile string
	sessionFile  string
}

// NewMaxnetTool creates a MaxnetTool if MAXNET_USERNAME and MAXNET_PASSWORD
// are set. Returns nil if credentials are not configured.
func NewMaxnetTool() *MaxnetTool {
	username := os.Getenv("MAXNET_USERNAME")
	password := os.Getenv("MAXNET_PASSWORD")
	if username == "" || password == "" {
		return nil
	}

	cacheDir := filepath.Join(config.GetHome(), "cache", "maxnet")
	os.MkdirAll(cacheDir, 0o755)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: maxnetHTTPTimeout,
	}

	return &MaxnetTool{
		username:     username,
		password:     password,
		cacheDir:     cacheDir,
		client:       client,
		pinCacheFile: filepath.Join(cacheDir, "pin_cache.json"),
		sessionFile:  filepath.Join(cacheDir, "session.json"),
	}
}

func (t *MaxnetTool) Name() string        { return "maxnet_wifi_pin" }
func (t *MaxnetTool) Description() string {
	return "Get WiFi PIN code from Maxnet ISP. Use action 'get_pin' to retrieve a PIN (cached for 55 min). Use 'clear_session' to reset."
}

func (t *MaxnetTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"get_pin", "check_status", "clear_session"},
				"description": "Action to perform",
			},
			"force_refresh": map[string]any{
				"type":        "boolean",
				"description": "Force refresh PIN even if cached",
			},
		},
		"required": []string{"action"},
	}
}

func (t *MaxnetTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	switch action {
	case "get_pin":
		forceRefresh, _ := args["force_refresh"].(bool)
		return t.getPin(ctx, forceRefresh)
	case "check_status":
		return t.checkStatus(ctx)
	case "clear_session":
		return t.clearSession()
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *MaxnetTool) getPin(ctx context.Context, forceRefresh bool) *ToolResult {
	// Check cache first
	if !forceRefresh {
		if pin := t.getCachedPin(); pin != "" {
			return SilentResult(fmt.Sprintf("WiFi PIN: %s (cached)", pin))
		}
	}

	// Try login and get PIN
	if err := t.login(ctx); err != nil {
		return ErrorResult(fmt.Sprintf("Login failed: %v", err))
	}

	pin, err := t.generatePin(ctx)
	if err != nil {
		return ErrorResult(fmt.Sprintf("PIN generation failed: %v", err))
	}

	t.cachePin(pin)
	return SilentResult(fmt.Sprintf("WiFi PIN: %s", pin))
}

func (t *MaxnetTool) checkStatus(ctx context.Context) *ToolResult {
	resp, err := t.client.Get(maxnetBaseURL + maxnetPinPath + "?subject=wifi_pincode_new")
	if err != nil {
		return SilentResult("Status: not authenticated (connection error)")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if strings.Contains(string(body), "wifi_pincode_new_do") && strings.Contains(string(body), "apassword") {
		return SilentResult("Status: authenticated")
	}
	return SilentResult("Status: not authenticated")
}

func (t *MaxnetTool) clearSession() *ToolResult {
	os.Remove(t.pinCacheFile)
	os.Remove(t.sessionFile)
	// Reset cookie jar
	jar, _ := cookiejar.New(nil)
	t.client.Jar = jar
	return SilentResult("Session and PIN cache cleared")
}

func (t *MaxnetTool) login(ctx context.Context) error {
	// GET login page to get cookies
	req, _ := http.NewRequestWithContext(ctx, "GET", maxnetBaseURL+maxnetLoginPath, nil)
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("get login page: %w", err)
	}
	resp.Body.Close()

	// POST login
	form := url.Values{
		"username": {t.username},
		"password": {t.password},
		"action":   {"login"},
		"storepw":  {"1"},
	}
	req, _ = http.NewRequestWithContext(ctx, "POST", maxnetBaseURL+maxnetLoginPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = t.client.Do(req)
	if err != nil {
		return fmt.Errorf("login POST: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := strings.ToLower(string(body))

	if strings.Contains(text, "превышено максимальное количество попыток") {
		return fmt.Errorf("max login attempts exceeded")
	}
	if !strings.Contains(text, "logout") && !strings.Contains(text, "выход") {
		return fmt.Errorf("login failed (no logout link in response)")
	}

	t.saveSession()
	logger.DebugC("maxnet", "Login successful")
	return nil
}

func (t *MaxnetTool) generatePin(ctx context.Context) (string, error) {
	form := url.Values{
		"subject":   {"wifi_pincode_new_do"},
		"apassword": {t.password},
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", maxnetBaseURL+maxnetPinPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("PIN request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	return extractPin(text)
}

// extractPin tries multiple regex patterns to find a 4-digit PIN in HTML response.
func extractPin(text string) (string, error) {
	for _, pattern := range pinPatterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) >= 2 {
			return matches[1], nil
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("PIN not found in response")
}

func (t *MaxnetTool) getCachedPin() string {
	data, err := os.ReadFile(t.pinCacheFile)
	if err != nil {
		return ""
	}
	var cache pinCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return ""
	}
	age := time.Since(time.UnixMilli(cache.Timestamp))
	if age > maxnetPinCacheTTL {
		os.Remove(t.pinCacheFile)
		return ""
	}
	logger.DebugCF("maxnet", "Returning cached PIN", map[string]any{"age_min": int(age.Minutes())})
	return cache.Pin
}

func (t *MaxnetTool) cachePin(pin string) {
	data, _ := json.Marshal(pinCache{Pin: pin, Timestamp: time.Now().UnixMilli()})
	os.WriteFile(t.pinCacheFile, data, 0o600)
}

func (t *MaxnetTool) saveSession() {
	data, _ := json.Marshal(sessionCache{Timestamp: time.Now().UnixMilli()})
	os.WriteFile(t.sessionFile, data, 0o600)
}
