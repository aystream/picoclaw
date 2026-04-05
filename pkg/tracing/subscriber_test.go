package tracing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sipeed/picoclaw/pkg/agent"
)

func newTestTP() (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
	)
	return tp, exp
}

func TestSubscriber_TurnLifecycle(t *testing.T) {
	tp, exp := newTestTP()
	defer tp.Shutdown(context.Background())

	ch := make(chan agent.Event, 16)
	sub := agent.EventSubscription{ID: 1, C: ch}
	s := NewSubscriber(tp, sub)
	defer s.Close()

	turnID := "test-turn-1"
	meta := agent.EventMeta{
		AgentID:    "main",
		TurnID:     turnID,
		SessionKey: "session-1",
		Iteration:  0,
	}

	// Emit TurnStart
	ch <- agent.Event{
		Kind: agent.EventKindTurnStart,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.TurnStartPayload{
			Channel:     "telegram",
			ChatID:      "123",
			UserMessage: "hello",
		},
	}

	// Emit LLMRequest
	meta.Iteration = 1
	ch <- agent.Event{
		Kind: agent.EventKindLLMRequest,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.LLMRequestPayload{
			Model:         "kimi-k2.5",
			MessagesCount: 3,
			ToolsCount:    5,
			MaxTokens:     4096,
		},
	}

	// Emit LLMResponse
	ch <- agent.Event{
		Kind: agent.EventKindLLMResponse,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.LLMResponsePayload{
			ContentLen: 100,
			ToolCalls:  1,
		},
	}

	// Emit ToolExecStart
	ch <- agent.Event{
		Kind: agent.EventKindToolExecStart,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.ToolExecStartPayload{
			Tool: "read_file",
		},
	}

	// Emit ToolExecEnd
	ch <- agent.Event{
		Kind: agent.EventKindToolExecEnd,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.ToolExecEndPayload{
			Tool:      "read_file",
			Duration:  50 * time.Millisecond,
			ForLLMLen: 200,
		},
	}

	// Emit TurnEnd
	ch <- agent.Event{
		Kind: agent.EventKindTurnEnd,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.TurnEndPayload{
			Status:          agent.TurnEndStatusCompleted,
			Iterations:      1,
			Duration:        500 * time.Millisecond,
			FinalContentLen: 100,
		},
	}

	// Let the subscriber process events
	time.Sleep(100 * time.Millisecond)

	spans := exp.GetSpans()
	require.GreaterOrEqual(t, len(spans), 3, "should have at least 3 spans (turn + llm + tool)")

	// Verify span names
	spanNames := make(map[string]bool)
	for _, span := range spans {
		spanNames[span.Name] = true
	}

	assert.True(t, spanNames["agent.turn"], "should have root turn span")
	assert.True(t, spanNames["llm.chat"], "should have LLM generation span")
	assert.True(t, spanNames["tool.read_file"], "should have tool span")
}

func TestSubscriber_LLMGenerationHasModelAttribute(t *testing.T) {
	tp, exp := newTestTP()
	defer tp.Shutdown(context.Background())

	ch := make(chan agent.Event, 16)
	sub := agent.EventSubscription{ID: 1, C: ch}
	s := NewSubscriber(tp, sub)
	defer s.Close()

	turnID := "test-turn-2"
	meta := agent.EventMeta{TurnID: turnID, AgentID: "main", Iteration: 1}

	ch <- agent.Event{
		Kind:    agent.EventKindTurnStart,
		Time:    time.Now(),
		Meta:    meta,
		Payload: agent.TurnStartPayload{Channel: "telegram", ChatID: "123"},
	}
	ch <- agent.Event{
		Kind: agent.EventKindLLMRequest,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.LLMRequestPayload{
			Model:     "kimi-k2.5",
			MaxTokens: 4096,
		},
	}
	ch <- agent.Event{
		Kind:    agent.EventKindLLMResponse,
		Time:    time.Now(),
		Meta:    meta,
		Payload: agent.LLMResponsePayload{ContentLen: 50},
	}
	ch <- agent.Event{
		Kind: agent.EventKindTurnEnd,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.TurnEndPayload{Status: agent.TurnEndStatusCompleted},
	}

	time.Sleep(100 * time.Millisecond)

	spans := exp.GetSpans()
	var llmSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "llm.chat" {
			llmSpan = &spans[i]
			break
		}
	}
	require.NotNil(t, llmSpan, "llm.chat span should exist")

	// Check gen_ai.request.model attribute (triggers Langfuse Generation mapping)
	found := false
	for _, attr := range llmSpan.Attributes {
		if string(attr.Key) == "gen_ai.request.model" {
			assert.Equal(t, "kimi-k2.5", attr.Value.AsString())
			found = true
		}
	}
	assert.True(t, found, "gen_ai.request.model attribute should be set on LLM span")
}

func TestSubscriber_ContextCompressEvent(t *testing.T) {
	tp, exp := newTestTP()
	defer tp.Shutdown(context.Background())

	ch := make(chan agent.Event, 16)
	sub := agent.EventSubscription{ID: 1, C: ch}
	s := NewSubscriber(tp, sub)
	defer s.Close()

	turnID := "test-turn-3"
	meta := agent.EventMeta{TurnID: turnID, AgentID: "main"}

	ch <- agent.Event{
		Kind:    agent.EventKindTurnStart,
		Time:    time.Now(),
		Meta:    meta,
		Payload: agent.TurnStartPayload{Channel: "telegram"},
	}
	ch <- agent.Event{
		Kind: agent.EventKindContextCompress,
		Time: time.Now(),
		Meta: meta,
		Payload: agent.ContextCompressPayload{
			Reason:            agent.ContextCompressReasonProactive,
			DroppedMessages:   15,
			RemainingMessages: 10,
		},
	}
	ch <- agent.Event{
		Kind:    agent.EventKindTurnEnd,
		Time:    time.Now(),
		Meta:    meta,
		Payload: agent.TurnEndPayload{Status: agent.TurnEndStatusCompleted},
	}

	time.Sleep(100 * time.Millisecond)

	spans := exp.GetSpans()
	var turnSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "agent.turn" {
			turnSpan = &spans[i]
			break
		}
	}
	require.NotNil(t, turnSpan)
	assert.Len(t, turnSpan.Events, 1, "should have 1 event (context.compress)")
	assert.Equal(t, "context.compress", turnSpan.Events[0].Name)
}

func TestSubscriber_NoOpOnUnknownTurnID(t *testing.T) {
	tp, exp := newTestTP()
	defer tp.Shutdown(context.Background())

	ch := make(chan agent.Event, 16)
	sub := agent.EventSubscription{ID: 1, C: ch}
	s := NewSubscriber(tp, sub)
	defer s.Close()

	// Send events with unknown turn ID — should not panic
	meta := agent.EventMeta{TurnID: "unknown-turn"}
	ch <- agent.Event{
		Kind:    agent.EventKindLLMRequest,
		Time:    time.Now(),
		Meta:    meta,
		Payload: agent.LLMRequestPayload{Model: "test"},
	}
	ch <- agent.Event{
		Kind:    agent.EventKindToolExecEnd,
		Time:    time.Now(),
		Meta:    meta,
		Payload: agent.ToolExecEndPayload{Tool: "test"},
	}

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, exp.GetSpans(), "should produce no spans for unknown turn")
}
