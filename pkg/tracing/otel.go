// Package tracing provides Langfuse observability via OpenTelemetry OTLP/HTTP.
package tracing

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	tracerName            = "picoclaw"
	langfuseIngestionVer  = "4"
	defaultLangfuseOTLPPath = "/api/public/otel/v1/traces"
)

// InitTracerProvider creates an OTel TracerProvider configured to export
// to a Langfuse instance via OTLP/HTTP. Returns nil if LANGFUSE_PUBLIC_KEY
// or LANGFUSE_SECRET_KEY are not set (no-op mode).
func InitTracerProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")
	if publicKey == "" || secretKey == "" {
		return nil, nil
	}

	baseURL := os.Getenv("LANGFUSE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://cloud.langfuse.com"
	}

	// Langfuse OTLP endpoint uses Basic auth: base64(publicKey:secretKey)
	authString := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", publicKey, secretKey)))

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(baseURL+defaultLangfuseOTLPPath),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization":                fmt.Sprintf("Basic %s", authString),
			"x-langfuse-ingestion-version": langfuseIngestionVer,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(tracerName),
		)),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}
