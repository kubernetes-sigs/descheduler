package tracing

import (
	"context"
	"testing"

	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

func TestCreateTraceableResource(t *testing.T) {
	ctx := context.TODO()
	resourceOpts := defaultResourceOpts("descheduler")
	_, err := sdkresource.New(ctx, resourceOpts...)
	if err != nil {
		t.Errorf("error initialising tracer provider: %!", err)
	}
}
