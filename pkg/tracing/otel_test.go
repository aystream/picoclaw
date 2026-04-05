package tracing

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTracerProvider_NilWhenKeysUnset(t *testing.T) {
	os.Unsetenv("LANGFUSE_PUBLIC_KEY")
	os.Unsetenv("LANGFUSE_SECRET_KEY")

	tp, err := InitTracerProvider(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tp, "should return nil when keys are not set")
}

func TestInitTracerProvider_NilWhenOnlyPublicKeySet(t *testing.T) {
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test")
	os.Unsetenv("LANGFUSE_SECRET_KEY")

	tp, err := InitTracerProvider(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tp)
}

func TestInitTracerProvider_CreatesProviderWhenKeysSet(t *testing.T) {
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test-key")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-test-key")
	t.Setenv("LANGFUSE_BASE_URL", "http://localhost:3000")

	tp, err := InitTracerProvider(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tp, "should create TracerProvider when keys are set")

	// Cleanup
	err = tp.Shutdown(context.Background())
	require.NoError(t, err)
}
