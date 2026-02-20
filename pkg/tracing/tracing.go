/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tracing

import (
	"context"
	"crypto/x509"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"
	"k8s.io/klog/v2"
)

const (
	// DefaultServiceName is the default service name used for tracing.
	DefaultServiceName = "descheduler"

	// DescheduleOperation is the operation name used for Deschedule() functions.
	DescheduleOperation = "deschedule"

	// BalanceOperation is the operation name used for Balance() functions.
	BalanceOperation = "balance"

	// EvictOperation is the operation name used for Evict() functions.
	EvictOperation = "evict"

	// TracerName is used to setup the named tracer
	TracerName = "sigs.k8s.io/descheduler"
)

var (
	tracer   trace.Tracer
	provider trace.TracerProvider
)

func init() {
	provider = noop.NewTracerProvider()
	tracer = provider.Tracer(TracerName)
}

func Tracer() trace.Tracer {
	return tracer
}

// NewTracerProvider creates a new trace provider with the given options.
func NewTracerProvider(ctx context.Context, endpoint, caCert, name, namespace string, sampleRate float64, fallbackToNoOpTracer bool) (err error) {
	defer func(p trace.TracerProvider) {
		if err != nil && !fallbackToNoOpTracer {
			return
		}
		if err != nil && fallbackToNoOpTracer {
			klog.ErrorS(err, "ran into an error trying to setup a trace provider. Falling back to NoOp provider")
			err = nil
			provider = noop.NewTracerProvider()
		}
		otel.SetTextMapPropagator(propagation.TraceContext{})
		otel.SetTracerProvider(provider)
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			klog.ErrorS(err, "got error from opentelemetry")
		}))
		tracer = otel.GetTracerProvider().Tracer(TracerName)
	}(provider)

	if endpoint == "" {
		klog.V(2).Info("Did not find a trace collector endpoint defined. Switching to NoopTraceProvider")
		provider = noop.NewTracerProvider()
		return nil
	}

	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
	var data []byte
	if caCert != "" {
		data, err = os.ReadFile(caCert)
		if err != nil {
			klog.ErrorS(err, "failed to extract ca certificate required to generate the transport credentials")
			return err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(data) {
			klog.Error("failed to create cert pool using the ca certificate provided for generating the transport credentials")
			return err
		}
		klog.Info("Enabling trace GRPC client in secure TLS mode")
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(pool, "")))
	} else {
		klog.Info("Enabling trace GRPC client in insecure mode")
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if os.Getenv("USER") == "" {
		if err := os.Setenv("USER", "descheduler"); err != nil {
			klog.ErrorS(err, "failed to set USER environment variable")
		}
	}

	client := otlptracegrpc.NewClient(opts...)

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		klog.ErrorS(err, "failed to create an instance of the trace exporter")
		return err
	}
	if name == "" {
		klog.V(5).InfoS("no name provided, using default service name for tracing", "name", DefaultServiceName)
		name = DefaultServiceName
	}
	resourceOpts := defaultResourceOpts(name)
	if namespace != "" {
		resourceOpts = append(resourceOpts, sdkresource.WithAttributes(semconv.ServiceNamespaceKey.String(namespace)))
	}
	resource, err := sdkresource.New(ctx, resourceOpts...)
	if err != nil {
		klog.ErrorS(err, "failed to create traceable resource")
		return err
	}

	spanProcessor := sdktrace.NewBatchSpanProcessor(exporter)
	provider = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))),
		sdktrace.WithSpanProcessor(spanProcessor),
		sdktrace.WithResource(resource),
	)
	klog.V(2).Info("Successfully setup trace provider")
	return nil
}

func defaultResourceOpts(name string) []sdkresource.Option {
	return []sdkresource.Option{sdkresource.WithAttributes(semconv.ServiceNameKey.String(name)), sdkresource.WithSchemaURL(semconv.SchemaURL), sdkresource.WithProcess()}
}

// Shutdown shuts down the global trace exporter.
func Shutdown(ctx context.Context) error {
	tp, ok := provider.(*sdktrace.TracerProvider)
	if !ok {
		return nil
	}
	if err := tp.Shutdown(ctx); err != nil {
		otel.Handle(err)
		klog.ErrorS(err, "failed to shutdown the trace exporter")
		return err
	}
	return nil
}
