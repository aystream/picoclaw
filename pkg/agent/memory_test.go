package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeTopicKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"-1001234/42", "-1001234_42"},
		{"-1001234567890/99", "-1001234567890_99"},
		{"simple", "simple"},
		{"a:b/c\\d", "a_b_c_d"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, sanitizeTopicKey(tt.input), "input: %s", tt.input)
	}
}

func TestMemoryStore_TopicMemoryPath(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	path := ms.TopicMemoryPath("-1001234_42")
	assert.Contains(t, path, filepath.Join("memory", "topics", "-1001234_42", "MEMORY.md"))
}

func TestMemoryStore_EnsureTopicDir(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	topicKey := "-1001234_42"
	ms.EnsureTopicDir(topicKey)

	topicDir := ms.TopicMemoryDir(topicKey)
	info, err := os.Stat(topicDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestMemoryStore_GetTopicMemoryContext_Empty(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	topicKey := "-1001234_42"
	ms.EnsureTopicDir(topicKey)

	ctx := ms.GetTopicMemoryContext(topicKey)
	assert.Empty(t, ctx, "empty topic dir should return empty context")
}

func TestMemoryStore_GetTopicMemoryContext_WithContent(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	topicKey := "-1001234_42"
	ms.EnsureTopicDir(topicKey)

	// Write topic-specific MEMORY.md
	memPath := ms.TopicMemoryPath(topicKey)
	err := os.WriteFile(memPath, []byte("# Health Topic\n\nUser tracks sleep quality."), 0o600)
	require.NoError(t, err)

	ctx := ms.GetTopicMemoryContext(topicKey)
	assert.Contains(t, ctx, "Health Topic")
	assert.Contains(t, ctx, "sleep quality")
}

func TestMemoryStore_GetTopicMemoryContext_NonexistentTopic(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	ctx := ms.GetTopicMemoryContext("nonexistent_topic")
	assert.Empty(t, ctx, "nonexistent topic should return empty context")
}

func TestMemoryStore_GlobalMemoryUnchanged(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	// Write global memory
	err := ms.WriteLongTerm("Global memory content")
	require.NoError(t, err)

	// Global memory should still work
	ctx := ms.GetMemoryContext()
	assert.Contains(t, ctx, "Global memory content")
}
