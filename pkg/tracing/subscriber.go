package tracing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// turnTrace tracks the OTel span hierarchy for a single agent turn.
type turnTrace struct {
	ctx        context.Context
	rootSpan   trace.Span
	llmSpan    trace.Span
	llmCtx     context.Context
	toolSpans  map[int]trace.Span // keyed by iteration
	toolCtxs   map[int]context.Context
}

// Subscriber listens to the agent EventBus and translates events into
// OTel spans with Langfuse-compatible attributes.
type Subscriber struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
	sub    agent.EventSubscription
	done   chan struct{}
	mu     sync.Mutex
	traces map[string]*turnTrace // keyed by TurnID
}

// NewSubscriber creates and starts a subscriber that bridges agent events
// to OTel spans. The subscriber runs a background goroutine that reads
// from the EventBus subscription channel. Call Close to stop it.
func NewSubscriber(tp *sdktrace.TracerProvider, sub agent.EventSubscription) *Subscriber {
	s := &Subscriber{
		tp:     tp,
		tracer: tp.Tracer(tracerName),
		sub:    sub,
		done:   make(chan struct{}),
		traces: make(map[string]*turnTrace),
	}
	go s.run()
	return s
}

// Close stops the subscriber goroutine.
func (s *Subscriber) Close() {
	close(s.done)
}

func (s *Subscriber) run() {
	for {
		select {
		case <-s.done:
			return
		case evt, ok := <-s.sub.C:
			if !ok {
				return
			}
			s.handleEvent(evt)
		}
	}
}

func (s *Subscriber) handleEvent(evt agent.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	turnID := evt.Meta.TurnID

	switch evt.Kind {
	case agent.EventKindTurnStart:
		s.onTurnStart(turnID, evt)
	case agent.EventKindTurnEnd:
		s.onTurnEnd(turnID, evt)
	case agent.EventKindLLMRequest:
		s.onLLMRequest(turnID, evt)
	case agent.EventKindLLMResponse:
		s.onLLMResponse(turnID, evt)
	case agent.EventKindToolExecStart:
		s.onToolExecStart(turnID, evt)
	case agent.EventKindToolExecEnd:
		s.onToolExecEnd(turnID, evt)
	case agent.EventKindContextCompress:
		s.onContextCompress(turnID, evt)
	case agent.EventKindSessionSummarize:
		s.onSessionSummarize(turnID, evt)
	}
}

func (s *Subscriber) onTurnStart(turnID string, evt agent.Event) {
	payload, ok := evt.Payload.(agent.TurnStartPayload)
	if !ok {
		return
	}

	ctx, span := s.tracer.Start(context.Background(), "agent.turn",
		trace.WithTimestamp(evt.Time),
		trace.WithAttributes(
			attribute.String("langfuse.trace.name", "agent.turn"),
			attribute.String("langfuse.trace.session_id", evt.Meta.SessionKey),
			attribute.String("langfuse.trace.user_id", evt.Meta.AgentID),
			attribute.String("picoclaw.turn_id", turnID),
			attribute.String("picoclaw.channel", payload.Channel),
			attribute.String("picoclaw.chat_id", payload.ChatID),
			attribute.Int("picoclaw.media_count", payload.MediaCount),
		),
	)

	s.traces[turnID] = &turnTrace{
		ctx:       ctx,
		rootSpan:  span,
		toolSpans: make(map[int]trace.Span),
		toolCtxs:  make(map[int]context.Context),
	}
}

func (s *Subscriber) onTurnEnd(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil {
		return
	}

	payload, ok := evt.Payload.(agent.TurnEndPayload)
	if !ok {
		return
	}

	tt.rootSpan.SetAttributes(
		attribute.String("picoclaw.status", string(payload.Status)),
		attribute.Int("picoclaw.iterations", payload.Iterations),
		attribute.Int("picoclaw.final_content_len", payload.FinalContentLen),
	)

	if payload.Status == agent.TurnEndStatusError {
		tt.rootSpan.SetStatus(codes.Error, "turn error")
	} else {
		tt.rootSpan.SetStatus(codes.Ok, "")
	}

	tt.rootSpan.End(trace.WithTimestamp(evt.Time))
	delete(s.traces, turnID)
}

func (s *Subscriber) onLLMRequest(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil {
		return
	}

	payload, ok := evt.Payload.(agent.LLMRequestPayload)
	if !ok {
		return
	}

	// End any previous LLM span (multi-iteration turns)
	if tt.llmSpan != nil {
		tt.llmSpan.End(trace.WithTimestamp(evt.Time))
	}

	ctx, span := s.tracer.Start(tt.ctx, "llm.chat",
		trace.WithTimestamp(evt.Time),
		trace.WithAttributes(
			// gen_ai.* attributes make Langfuse map this as a Generation
			attribute.String("gen_ai.request.model", payload.Model),
			attribute.Int("gen_ai.request.max_tokens", payload.MaxTokens),
			attribute.Float64("gen_ai.request.temperature", payload.Temperature),
			attribute.Int("picoclaw.messages_count", payload.MessagesCount),
			attribute.Int("picoclaw.tools_count", payload.ToolsCount),
			attribute.Int("picoclaw.iteration", evt.Meta.Iteration),
		),
	)

	tt.llmSpan = span
	tt.llmCtx = ctx
}

func (s *Subscriber) onLLMResponse(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil || tt.llmSpan == nil {
		return
	}

	payload, ok := evt.Payload.(agent.LLMResponsePayload)
	if !ok {
		return
	}

	tt.llmSpan.SetAttributes(
		attribute.Int("gen_ai.response.content_len", payload.ContentLen),
		attribute.Int("picoclaw.tool_calls", payload.ToolCalls),
		attribute.Bool("picoclaw.has_reasoning", payload.HasReasoning),
	)

	tt.llmSpan.End(trace.WithTimestamp(evt.Time))
	tt.llmSpan = nil
	tt.llmCtx = nil
}

func (s *Subscriber) onToolExecStart(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil {
		return
	}

	payload, ok := evt.Payload.(agent.ToolExecStartPayload)
	if !ok {
		return
	}

	spanName := fmt.Sprintf("tool.%s", payload.Tool)
	ctx, span := s.tracer.Start(tt.ctx, spanName,
		trace.WithTimestamp(evt.Time),
		trace.WithAttributes(
			attribute.String("picoclaw.tool", payload.Tool),
			attribute.Int("picoclaw.iteration", evt.Meta.Iteration),
		),
	)

	tt.toolSpans[evt.Meta.Iteration] = span
	tt.toolCtxs[evt.Meta.Iteration] = ctx
}

func (s *Subscriber) onToolExecEnd(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil {
		return
	}

	payload, ok := evt.Payload.(agent.ToolExecEndPayload)
	if !ok {
		return
	}

	span := tt.toolSpans[evt.Meta.Iteration]
	if span == nil {
		return
	}

	span.SetAttributes(
		attribute.Int("picoclaw.for_llm_len", payload.ForLLMLen),
		attribute.Bool("picoclaw.is_error", payload.IsError),
		attribute.Bool("picoclaw.async", payload.Async),
	)

	if payload.IsError {
		span.SetStatus(codes.Error, "tool error")
	}

	span.End(trace.WithTimestamp(evt.Time))
	delete(tt.toolSpans, evt.Meta.Iteration)
	delete(tt.toolCtxs, evt.Meta.Iteration)
}

func (s *Subscriber) onContextCompress(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil {
		return
	}

	payload, ok := evt.Payload.(agent.ContextCompressPayload)
	if !ok {
		return
	}

	tt.rootSpan.AddEvent("context.compress", trace.WithTimestamp(evt.Time),
		trace.WithAttributes(
			attribute.String("picoclaw.reason", string(payload.Reason)),
			attribute.Int("picoclaw.dropped_messages", payload.DroppedMessages),
			attribute.Int("picoclaw.remaining_messages", payload.RemainingMessages),
		),
	)
}

func (s *Subscriber) onSessionSummarize(turnID string, evt agent.Event) {
	tt := s.traces[turnID]
	if tt == nil {
		return
	}

	payload, ok := evt.Payload.(agent.SessionSummarizePayload)
	if !ok {
		return
	}

	tt.rootSpan.AddEvent("session.summarize", trace.WithTimestamp(evt.Time),
		trace.WithAttributes(
			attribute.Int("picoclaw.summarized_messages", payload.SummarizedMessages),
			attribute.Int("picoclaw.kept_messages", payload.KeptMessages),
			attribute.Int("picoclaw.summary_len", payload.SummaryLen),
		),
	)
}

// cleanupStaleTurns removes turn traces that have been open too long (likely leaked).
// This prevents memory leaks from turns that never emit a TurnEnd event.
func (s *Subscriber) cleanupStaleTurns(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for turnID, tt := range s.traces {
		if tt.rootSpan != nil {
			// Use a heuristic: if the span has been open too long, end it
			ro := tt.rootSpan.(sdktrace.ReadOnlySpan)
			if ro.StartTime().Before(cutoff) {
				logger.WarnCF("tracing", "Cleaning up stale turn trace", map[string]any{
					"turn_id": turnID,
				})
				tt.rootSpan.SetStatus(codes.Error, "stale turn cleanup")
				tt.rootSpan.End()
				delete(s.traces, turnID)
			}
		}
	}
}
