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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
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

	// EvictionOption is the operation name used for Evict() functions.
	EvictionOption = "evict"
)

// NewTracerProvider creates a new trace provider with the given options.
func NewTracerProvider(endpoint, caCert, name, namespace string, sampleRate float64) (provider trace.TracerProvider, err error) {
	if endpoint != "" {
		ctx := context.Background()

		var opts []otlptracegrpc.Option

		opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))

		if caCert != "" {
			data, err := os.ReadFile(caCert)
			if err != nil {
				klog.ErrorS(err, "failed to extract ca certificate required to generate the transport credentials")
				return nil, err
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(data) {
				klog.Error("failed to create cert pool using the ca certificate provided for generating the transport credentials")
				return nil, err
			}
			klog.Info("Enabling trace GRPC client in secure TLS mode")
			opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(pool, "")))
		} else {
			klog.Info("Enabling trace GRPC client in insecure mode")
			opts = append(opts, otlptracegrpc.WithInsecure())
		}

		client := otlptracegrpc.NewClient(opts...)

		exporter, err := otlptrace.New(ctx, client)
		if err != nil {
			klog.ErrorS(err, "failed to create an instance of the trace exporter")
			return nil, err
		}
		if name == "" {
			klog.V(5).InfoS("no name provided, using default service name for tracing", "name", DefaultServiceName)
			name = DefaultServiceName
		}
		resourceOpts := []sdkresource.Option{sdkresource.WithAttributes(semconv.ServiceNameKey.String(name)), sdkresource.WithSchemaURL(semconv.SchemaURL)}
		if namespace != "" {
			klog.V(5).InfoS("no namespace provided, using default namespace")
			resourceOpts = append(resourceOpts, sdkresource.WithAttributes(semconv.ServiceNamespaceKey.String(namespace)))
		}
		resource, err := sdkresource.New(ctx, resourceOpts...)
		if err != nil {
			klog.ErrorS(err, "failed to create traceable resource")
			return nil, err
		}

		spanProcessor := sdktrace.NewBatchSpanProcessor(exporter)
		provider = sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))),
			sdktrace.WithSpanProcessor(spanProcessor),
			sdktrace.WithResource(resource),
		)
	} else {
		klog.Info("Did not find a trace collector endpoint defined. Switching to NoopTraceProvider")
		provider = trace.NewNoopTracerProvider()
	}

	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(provider)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		klog.ErrorS(err, "got error from opentelemetry")
	}))
	klog.Info("Successfully setup trace provider")
	return
}

// StartSpan starts a new span with the given operation name and attributes.
func StartSpan(ctx context.Context, tracerName, operationName string) (context.Context, trace.Span, func()) {
	return StartSpanWithOptions(ctx, tracerName, []trace.TracerOption{}, operationName, []attribute.KeyValue{})
}

func StartSpanWithOptions(ctx context.Context, tracerName string, traceOpts []trace.TracerOption, operationName string, attributes []attribute.KeyValue) (context.Context, trace.Span, func()) {
	spanCtx, span := otel.Tracer(tracerName, traceOpts...).Start(ctx, operationName, trace.WithAttributes(attributes...))
	return spanCtx, span, func() {
		span.End()
	}
}

// Shutdown shuts down the global trace exporter.
func Shutdown(ctx context.Context, tpRaw trace.TracerProvider) error {
	tp, ok := tpRaw.(*sdktrace.TracerProvider)
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
