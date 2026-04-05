package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectionHook_DeniesWriteToAgentMD(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	call := &ToolCallHookRequest{
		Tool:      "write_file",
		Arguments: map[string]any{"path": "/workspace/AGENT.md"},
	}
	_, decision, err := hook.BeforeTool(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, HookActionDenyTool, decision.Action)
	assert.Contains(t, decision.Reason, "AGENT.md")
}

func TestProtectionHook_DeniesEditToSOULMD(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	call := &ToolCallHookRequest{
		Tool:      "edit_file",
		Arguments: map[string]any{"file_path": "/workspace/SOUL.md"},
	}
	_, decision, err := hook.BeforeTool(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, HookActionDenyTool, decision.Action)
	assert.Contains(t, decision.Reason, "SOUL.md")
}

func TestProtectionHook_DeniesAppendToIdentityMD(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	call := &ToolCallHookRequest{
		Tool:      "append_file",
		Arguments: map[string]any{"path": "/workspace/IDENTITY.md"},
	}
	_, decision, err := hook.BeforeTool(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, HookActionDenyTool, decision.Action)
}

func TestProtectionHook_AllowsWriteToOtherFiles(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	call := &ToolCallHookRequest{
		Tool:      "write_file",
		Arguments: map[string]any{"path": "/workspace/notes.md"},
	}
	_, decision, err := hook.BeforeTool(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, HookActionContinue, decision.Action)
}

func TestProtectionHook_AllowsWriteToMemorySubdir(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	call := &ToolCallHookRequest{
		Tool:      "write_file",
		Arguments: map[string]any{"path": "/workspace/memory/MEMORY.md"},
	}
	_, decision, err := hook.BeforeTool(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, HookActionContinue, decision.Action)
}

func TestProtectionHook_AllowsNonWriteTools(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	call := &ToolCallHookRequest{
		Tool:      "read_file",
		Arguments: map[string]any{"path": "/workspace/AGENT.md"},
	}
	_, decision, err := hook.BeforeTool(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, HookActionContinue, decision.Action)
}

func TestProtectionHook_AfterToolPassthrough(t *testing.T) {
	hook := NewPromptProtectionHook("/workspace")
	resp := &ToolResultHookResponse{}
	_, decision, err := hook.AfterTool(context.Background(), resp)
	require.NoError(t, err)
	assert.Equal(t, HookActionContinue, decision.Action)
}

func TestCriticalRulesInPrompt(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	prompt := cb.BuildSystemPrompt()

	// Critical rules should be the very first section
	assert.True(t, strings.HasPrefix(prompt, "# CRITICAL SYSTEM RULES"),
		"System prompt should start with critical protection rules")

	// Should appear before identity section
	critIdx := strings.Index(prompt, "CRITICAL SYSTEM RULES")
	identIdx := strings.Index(prompt, "picoclaw")
	assert.Greater(t, identIdx, critIdx,
		"Critical rules should appear before identity section")
}
