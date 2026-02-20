package tracing

import (
	"testing"

	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

func TestCreateTraceableResource(t *testing.T) {
	ctx := t.Context()
	resourceOpts := defaultResourceOpts("descheduler")
	_, err := sdkresource.New(ctx, resourceOpts...)
	if err != nil {
		t.Errorf("error initialising tracer provider: %!", err)
	}
}
