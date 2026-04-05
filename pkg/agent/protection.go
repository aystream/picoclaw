package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// protectedBasenames lists workspace-root files that the agent must never modify.
var protectedBasenames = map[string]bool{
	"AGENT.md":    true,
	"AGENTS.md":   true,
	"SOUL.md":     true,
	"USER.md":     true,
	"IDENTITY.md": true,
}

// PromptProtectionHook is a ToolInterceptor that denies write operations
// targeting protected workspace definition files.
type PromptProtectionHook struct {
	workspace string
}

func NewPromptProtectionHook(workspace string) *PromptProtectionHook {
	return &PromptProtectionHook{workspace: workspace}
}

func (h *PromptProtectionHook) BeforeTool(
	_ context.Context,
	call *ToolCallHookRequest,
) (*ToolCallHookRequest, HookDecision, error) {
	if !isWriteTool(call.Tool) {
		return call, HookDecision{Action: HookActionContinue}, nil
	}

	path, _ := call.Arguments["path"].(string)
	if path == "" {
		path, _ = call.Arguments["file_path"].(string)
	}
	if path == "" {
		return call, HookDecision{Action: HookActionContinue}, nil
	}

	if h.isProtectedPath(path) {
		return call, HookDecision{
			Action: HookActionDenyTool,
			Reason: fmt.Sprintf("writing to protected file %q is not allowed", filepath.Base(path)),
		}, nil
	}

	return call, HookDecision{Action: HookActionContinue}, nil
}

func (h *PromptProtectionHook) AfterTool(
	_ context.Context,
	result *ToolResultHookResponse,
) (*ToolResultHookResponse, HookDecision, error) {
	return result, HookDecision{Action: HookActionContinue}, nil
}

func isWriteTool(name string) bool {
	switch name {
	case "write_file", "edit_file", "append_file":
		return true
	}
	return false
}

func (h *PromptProtectionHook) isProtectedPath(path string) bool {
	// Resolve to absolute for comparison
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absWorkspace, err := filepath.Abs(h.workspace)
	if err != nil {
		absWorkspace = h.workspace
	}

	// Check if the file is directly in the workspace root and matches a protected name
	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)

	if strings.EqualFold(dir, absWorkspace) && protectedBasenames[base] {
		return true
	}

	return false
}
