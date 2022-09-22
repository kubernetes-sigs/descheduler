package tracing

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func TestNewTracerProvider(t *testing.T) {
	type args struct {
		endpoint  string
		caCert    string
		name      string
		namespace string
	}
	tests := []struct {
		name         string
		args         args
		wantProvider trace.TracerProvider
		error        error
	}{
		{
			name: "test not exist ca cert",
			args: args{
				endpoint: "localhost",
				caCert:   "foo",
			},
			wantProvider: nil,
			error:        fmt.Errorf("open foo: no such file or directory"),
		},
		{
			name: "empty name",
			args: args{
				endpoint: "localhost",
				name:     "",
			},
			wantProvider: otel.GetTracerProvider(),
			error:        nil,
		},
		{
			name: "with namespace and empty endpoint",
			args: args{
				endpoint: "",
			},
			wantProvider: otel.GetTracerProvider(),
			error:        nil,
		},
		{
			name: "with name and namespace",
			args: args{
				endpoint:  "localhost",
				name:      "foo",
				namespace: "bar",
			},
			wantProvider: otel.GetTracerProvider(),
			error:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProvider, err := NewTracerProvider(tt.args.endpoint, tt.args.caCert, tt.args.name, tt.args.namespace, 1.0)
			if (err != nil) && err.Error() != tt.error.Error() {
				t.Errorf("NewTracerProvider() error = '%v', wantErr '%v'", err, tt.error)
				return
			}
			if tt.wantProvider != nil {
				if gotProvider == nil {
					t.Errorf("NewTracerProvider() nil provider got = %v, want %v", gotProvider, tt.wantProvider)
					return
				}
				if !reflect.DeepEqual(otel.GetTracerProvider(), gotProvider) {
					t.Errorf("NewTracerProvider() gotProvider = %v, want %v", gotProvider, otel.GetTracerProvider())
					return
				}
			}
		})
	}
}

func TestStartSpan(t *testing.T) {
	tests := []struct {
		name  string
		error error
	}{
		{
			name:  "test without error",
			error: nil,
		},
		{
			name:  "test with error",
			error: fmt.Errorf("test error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, _ := NewTracerProvider("localhost:80", "", "", "", 1.0)
			traceCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer Shutdown(traceCtx, tp)
			defer cancel()

			_, span, spanCloser := StartSpan(context.Background(), "test-tracer", "test-operation")
			defer func() {
				if tt.error != nil {
					span.RecordError(tt.error)
				}
				spanCloser()
			}()

			if !span.SpanContext().IsValid() {
				t.Errorf("StartSpan() spanContext is invalid. spanId = %v traceId = %v", span.SpanContext().SpanID(), span.SpanContext().TraceID())
				return
			}

			if !span.SpanContext().IsSampled() {
				t.Errorf("StartSpan() spanContext is not sampled. spanId = %v", span.SpanContext().SpanID())
				return
			}
		})
	}
}
