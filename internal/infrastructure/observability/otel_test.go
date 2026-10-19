// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSetupOTelSDK_NoSchemaURLConflict guards against the semconv import in
// otel.go drifting from the semconv version used by sdk/resource.Default(),
// which makes resource.Merge fail with a conflicting Schema URL error.
func TestSetupOTelSDK_NoSchemaURLConflict(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	ctx := context.Background()
	shutdown, err := SetupOTelSDK(ctx)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(ctx))
}
