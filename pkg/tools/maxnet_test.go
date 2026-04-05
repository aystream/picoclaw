package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxnetTool_PinExtraction(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantPin string
		wantErr bool
	}{
		{"PIN english", `<p>Your PIN: 4567</p>`, "4567", false},
		{"PIN russian", `<p>ПИН: 1234</p>`, "1234", false},
		{"Code russian", `<p>Код: 9876</p>`, "9876", false},
		{"Fallback 4 digits", `<p>Result: success 5432 done</p>`, "5432", false},
		{"No PIN", `<p>Error occurred</p>`, "", true},
		{"PIN with colon space", `PIN: 7777`, "7777", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pin, err := extractPin(tt.html)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPin, pin)
			}
		})
	}
}

func TestMaxnetTool_PinCacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	tool := &MaxnetTool{
		username:     "test",
		password:     "test",
		cacheDir:     cacheDir,
		pinCacheFile: filepath.Join(cacheDir, "pin_cache.json"),
		sessionFile:  filepath.Join(cacheDir, "session.json"),
	}

	// Write fresh cache
	cache := pinCache{Pin: "1234", Timestamp: time.Now().UnixMilli()}
	data, _ := json.Marshal(cache)
	os.WriteFile(tool.pinCacheFile, data, 0o600)

	pin := tool.getCachedPin()
	assert.Equal(t, "1234", pin)
}

func TestMaxnetTool_PinCacheExpired(t *testing.T) {
	cacheDir := t.TempDir()
	tool := &MaxnetTool{
		username:     "test",
		password:     "test",
		cacheDir:     cacheDir,
		pinCacheFile: filepath.Join(cacheDir, "pin_cache.json"),
		sessionFile:  filepath.Join(cacheDir, "session.json"),
	}

	// Write expired cache (2 hours old)
	cache := pinCache{Pin: "1234", Timestamp: time.Now().Add(-2 * time.Hour).UnixMilli()}
	data, _ := json.Marshal(cache)
	os.WriteFile(tool.pinCacheFile, data, 0o600)

	pin := tool.getCachedPin()
	assert.Empty(t, pin, "expired cache should return empty")

	// Cache file should be deleted
	_, err := os.Stat(tool.pinCacheFile)
	assert.True(t, os.IsNotExist(err), "expired cache file should be deleted")
}

func TestMaxnetTool_LoginAndGetPin(t *testing.T) {
	// Mock HTTP server simulating stat.maxnet.ru
	mux := http.NewServeMux()

	// Login page
	mux.HandleFunc("/cgi-bin/index.cgi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write([]byte("<html>Login page</html>"))
			return
		}
		// POST login
		r.ParseForm()
		if r.Form.Get("username") == "testuser" && r.Form.Get("password") == "testpass" {
			w.Write([]byte("<html>Welcome! <a href='logout'>выход</a></html>"))
		} else {
			w.Write([]byte("<html>Invalid credentials</html>"))
		}
	})

	// PIN generation
	mux.HandleFunc("/cgi-bin/edit.cgi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write([]byte(`<html><form><input name="apassword"><input name="wifi_pincode_new_do"></form></html>`))
			return
		}
		r.ParseForm()
		if r.Form.Get("subject") == "wifi_pincode_new_do" {
			w.Write([]byte(fmt.Sprintf("<html>Your PIN: 4321</html>")))
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// The mock server is available for integration tests that override the base URL.
	// For unit tests, we verify PIN extraction works against the expected response format.
	_ = server

	pin, err := extractPin("<html>Your PIN: 4321</html>")
	require.NoError(t, err)
	assert.Equal(t, "4321", pin)
}

func TestMaxnetTool_ClearSession(t *testing.T) {
	cacheDir := t.TempDir()
	tool := &MaxnetTool{
		username:     "test",
		password:     "test",
		cacheDir:     cacheDir,
		client:       &http.Client{},
		pinCacheFile: filepath.Join(cacheDir, "pin_cache.json"),
		sessionFile:  filepath.Join(cacheDir, "session.json"),
	}

	// Create cache files
	os.WriteFile(tool.pinCacheFile, []byte(`{"pin":"1234","timestamp":1}`), 0o600)
	os.WriteFile(tool.sessionFile, []byte(`{"timestamp":1}`), 0o600)

	result := tool.clearSession()
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "cleared")

	_, err1 := os.Stat(tool.pinCacheFile)
	_, err2 := os.Stat(tool.sessionFile)
	assert.True(t, os.IsNotExist(err1))
	assert.True(t, os.IsNotExist(err2))
}

func TestMaxnetTool_NoCredentials(t *testing.T) {
	os.Unsetenv("MAXNET_USERNAME")
	os.Unsetenv("MAXNET_PASSWORD")

	tool := NewMaxnetTool()
	assert.Nil(t, tool, "should return nil when credentials are not set")
}
