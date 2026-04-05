# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PicoClaw is an ultra-lightweight personal AI assistant written in Go, designed to run on $10 hardware with <10MB RAM. It connects LLM providers to messaging channels (Telegram, Discord, Slack, WhatsApp, etc.) through a gateway architecture with agent-based processing.

## Build & Development Commands

```bash
make build              # Build binary for current platform (runs go generate first)
make build-launcher     # Build the web UI launcher (requires pnpm for frontend)
make test               # Run all Go tests + web tests
make lint               # Run golangci-lint
make fmt                # Format code (gci, gofmt, gofumpt, goimports, golines)
make vet                # Static analysis
make check              # Full pre-commit: deps + fmt + vet + test

# Single test
go test -run TestName -v -tags goolm,stdjson ./pkg/agent/

# Web UI development
cd web && make dev      # Start frontend (Vite) + backend dev servers
cd web && make test     # Run web backend tests + frontend lint
```

Build tags: always use `-tags goolm,stdjson` when running Go commands directly. The Makefile sets these via `GOFLAGS`.

CGO is disabled by default (`CGO_ENABLED=0`) except on Darwin for the web launcher (`CGO_ENABLED=1`).

## Architecture

### Core Data Flow

```
Channel (inbound msg) -> MessageBus -> Gateway -> Agent Loop -> LLM Provider
                                                    |
                                               Tool Registry
                                                    |
                                         MessageBus (outbound) -> Channel (reply)
```

### Key Packages

- **`pkg/gateway`** — Orchestrates the full system: initializes agents, channels, providers, cron, tools, and the message bus. Entry point for the `picoclaw gateway` command.
- **`pkg/agent`** — Agent loop implementation. `AgentInstance` holds config + dependencies. `turn.go` runs the main LLM conversation loop with tool calls. `subturn.go` handles sub-agent spawning. `steering.go` applies system prompt steering. `hooks.go`/`hook_process.go` run lifecycle hooks. `eventbus.go` emits agent events. `context.go`/`context_budget.go` manage context window pruning and summarization.
- **`pkg/providers`** — `LLMProvider` interface (Chat method) with implementations: OpenAI-compatible (`openai_compat/`), Anthropic (`anthropic/`, `anthropic_messages/`), Azure (`azure/`), AWS Bedrock (`bedrock/`), GitHub Copilot, Claude CLI. `fallback.go` handles multi-key and multi-provider fallback. `factory.go` resolves providers from `model_list` config.
- **`pkg/channels`** — `Channel` implementations for messaging platforms. Each subdirectory (telegram, discord, slack, matrix, irc, wecom, whatsapp, etc.) is a self-registering plugin via `init()`. `manager.go` handles message routing, typing indicators, placeholders, streaming, and split logic.
- **`pkg/bus`** — `MessageBus` with typed inbound/outbound/media channels. Decouples channels from the agent loop.
- **`pkg/tools`** — Tool registry and built-in tools (shell, filesystem, edit, web search, MCP, cron, spawn/subagent, skills, i2c/spi for hardware).
- **`pkg/config`** — JSON config schema (version 2). `Config` struct maps to `config.json`. `model_list` is the provider configuration mechanism.
- **`pkg/routing`** — Model routing: scores messages to decide between main and light (cheaper) models.
- **`pkg/mcp`** — Model Context Protocol client integration.
- **`pkg/session`** — Conversation session storage (SQLite-backed).
- **`pkg/memory`** — JSONL-based persistent memory store for agents.

### Web UI (Launcher)

- **`web/frontend/`** — Vite + React + TypeScript SPA. Uses pnpm.
- **`web/backend/`** — Go HTTP server that embeds the built frontend (`dist/`) and exposes REST APIs for config management and gateway control. Built as `picoclaw-launcher`.

### CLI Structure

`cmd/picoclaw/main.go` uses cobra with subcommands: `gateway`, `agent`, `auth`, `cron`, `migrate`, `skills`, `model`, `status`, `version`, `onboard`.

### Channel Plugin Pattern

Each channel in `pkg/channels/<name>/` registers itself via `init()` calling `channels.Register(...)`. The gateway imports them as blank imports (`_ ".../<name>"`). To add a new channel, create the subdirectory, implement the `Channel` interface, and add a blank import in `pkg/gateway/gateway.go`.

### Provider Resolution

Providers are resolved from `model_list` in config via `pkg/providers/factory.go`. Each model entry specifies a provider type and API key. The factory creates provider instances. `fallback.go` wraps multiple keys/providers for automatic failover.

## Workflow

### BDD-First Development

When implementing features or fixing bugs, follow these steps in order. Do not skip steps.

#### Step 0: Read the Spec
Read all available context before writing any code:
- Jira ticket (acceptance criteria, description, comments) — use `mcp__claude_ai_Atlassian__*` tools
- Linked Confluence pages — use `mcp__confluence__*` tools
- Existing tests in the affected package
- Existing code in the affected area

#### Step 1: Write Failing Tests
Write tests that formalize the expected behavior. Tests are the definition of done.
- Place tests in the same package as the code being changed
- Follow existing test patterns (testify assert/require)

#### Step 2: Run Tests — Confirm Failure
Run only the new/modified tests to verify they fail for the right reason:
```bash
go test -run TestName -v -tags goolm,stdjson ./pkg/<package>/
```
If a test passes before implementation — re-examine it, it's not testing new behavior.

#### Step 3: Write Implementation
Write minimal code to make the failing tests pass.
- Follow existing patterns in the package
- Keep functions small and focused
- Never delete or weaken existing tests to make new code pass — fix the code, not the test

#### Step 4: Run Tests — Confirm Pass
Run the same tests from Step 2. All must pass.

#### Step 5: Run Broader Tests — Check Regressions
Run the full test suite for the affected area:
```bash
go test -v -tags goolm,stdjson ./pkg/<package>/...
```
If changes touch shared packages (`pkg/config`, `pkg/bus`, `pkg/providers/protocoltypes`) — also run tests for direct dependents.

#### Step 6: Compile Check
Verify the project compiles cleanly:
```bash
make vet
```

### General Rules
- For multi-file refactoring, outline the complete plan first and wait for approval before executing.
- Run relevant tests after making changes to verify nothing is broken.

## Code Conventions

- When making git commits, do NOT add a `Co-Authored-By` line to the commit message.
- Conventional commits: `feat(scope):`, `fix(scope):`, `docs:`, etc.
- Linter: golangci-lint v2 with most linters enabled (see `.golangci.yaml`). Many are currently disabled pending fixes.
- Formatter config enforces: `interface{}` -> `any`, max line length 120, gci import ordering (stdlib, external, local module).
- Tests use `testify` (assert/require).
- Go 1.25+, module path: `github.com/sipeed/picoclaw`.
