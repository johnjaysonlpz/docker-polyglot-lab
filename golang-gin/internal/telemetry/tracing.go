package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	exporterOTLP = "otlp"
	exporterNone = "none"
)

var (
	newOTLPHTTPExporter = func(ctx context.Context, endpointURL string, timeout time.Duration) (sdktrace.SpanExporter, error) {
		return otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(endpointURL),
			otlptracehttp.WithTimeout(timeout),
		)
	}

	newOTLPGRPCExporter = func(ctx context.Context, endpoint string, timeout time.Duration, insecure bool) (sdktrace.SpanExporter, error) {
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithTimeout(timeout),
		}
		if insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, opts...)
	}

	grpcEndpointResolver = grpcTracesEndpointAndInsecure
)

func InitTracing(ctx context.Context, logger *slog.Logger, serviceName, version, buildTime string) (func(context.Context) error, error) {
	if envTrue("OTEL_SDK_DISABLED") {
		return initDisabled(logger), nil
	}

	expName, err := tracesExporterFromEnv()
	if err != nil {
		return nil, err
	}

	serviceName = resolveServiceName(serviceName)

	res, err := buildResource(ctx, serviceName, version, buildTime)
	if err != nil {
		return nil, err
	}

	samplerName, samplerArg := readSamplerEnv(expName)
	sampler, err := samplerFromEnv(samplerName, samplerArg)
	if err != nil {
		return nil, err
	}

	setDefaultPropagators()

	if expName == exporterNone {
		return initNoExporter(logger, res, sampler, samplerName), nil
	}

	proto := strings.ToLower(resolveOTLPProtocol())
	timeout := parseOTLPTimeout()

	if proto == "http/protobuf" {
		return initOTLPHTTP(ctx, logger, res, sampler, samplerName, expName, proto, timeout)
	}

	if proto == "grpc" || proto == "grpc/protobuf" {
		return initOTLPGRPC(ctx, logger, res, sampler, samplerName, expName, proto, timeout)
	}

	return nil, fmt.Errorf("unsupported OTEL_EXPORTER_OTLP_*_PROTOCOL=%q (supported: http/protobuf, grpc)", proto)
}

func initDisabled(logger *slog.Logger) func(context.Context) error {
	otel.SetTracerProvider(noop.NewTracerProvider())
	setDefaultPropagators()
	if logger != nil {
		logger.Info("otel_sdk_disabled")
	}
	return func(context.Context) error { return nil }
}

func resolveServiceName(defaultName string) string {
	if v := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); v != "" {
		return v
	}
	return defaultName
}

func buildResource(ctx context.Context, serviceName, version, buildTime string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithContainer(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("build_time", buildTime),
		),
	)
}

func readSamplerEnv(expName string) (name, arg string) {
	name = strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))
	arg = strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))

	if name == "" && expName == exporterNone {
		name = "parentbased_always_off"
	}
	return name, arg
}

func initNoExporter(logger *slog.Logger, res *resource.Resource, sampler sdktrace.Sampler, samplerName string) func(context.Context) error {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)

	if logger != nil {
		logger.Info("otel_traces_exporter_disabled",
			"otel_traces_exporter", exporterNone,
			"otel_traces_sampler", samplerNameOrDefault(samplerName),
		)
	}

	return tp.Shutdown
}

func initOTLPHTTP(
	ctx context.Context,
	logger *slog.Logger,
	res *resource.Resource,
	sampler sdktrace.Sampler,
	samplerName string,
	expName string,
	proto string,
	timeout time.Duration,
) (func(context.Context) error, error) {
	endpointURL, err := httpTracesEndpointURL()
	if err != nil {
		return nil, err
	}

	exp, err := newOTLPHTTPExporter(ctx, endpointURL, timeout)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exp),
	)
	otel.SetTracerProvider(tp)

	if logger != nil {
		logger.Info("otel_tracing_enabled",
			"otel_traces_exporter", expName,
			"otel_exporter_otlp_protocol", proto,
			"otel_exporter_otlp_traces_endpoint", endpointURL,
			"otel_traces_sampler", samplerNameOrDefault(samplerName),
			"otel_exporter_otlp_timeout", timeout.String(),
		)
	}

	return tp.Shutdown, nil
}

func initOTLPGRPC(
	ctx context.Context,
	logger *slog.Logger,
	res *resource.Resource,
	sampler sdktrace.Sampler,
	samplerName string,
	expName string,
	proto string,
	timeout time.Duration,
) (func(context.Context) error, error) {
	endpoint, insecure, err := grpcEndpointResolver()
	if err != nil {
		return nil, err
	}

	exp, err := newOTLPGRPCExporter(ctx, endpoint, timeout, insecure)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exp),
	)
	otel.SetTracerProvider(tp)

	if logger != nil {
		logger.Info("otel_tracing_enabled",
			"otel_traces_exporter", expName,
			"otel_exporter_otlp_protocol", proto,
			"otel_exporter_otlp_traces_endpoint", endpoint,
			"otel_exporter_otlp_insecure", insecure,
			"otel_traces_sampler", samplerNameOrDefault(samplerName),
			"otel_exporter_otlp_timeout", timeout.String(),
		)
	}

	return tp.Shutdown, nil
}

func resolveOTLPProtocol() string {
	proto := firstNonEmpty(
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL")),
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")),
	)
	if proto == "" {
		return "http/protobuf"
	}
	return proto
}

func setDefaultPropagators() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

func tracesExporterFromEnv() (string, error) {
	raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER"))
	if raw == "" {
		return exporterOTLP, nil
	}

	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}

	for _, p := range parts {
		if p == exporterOTLP {
			return exporterOTLP, nil
		}
	}
	for _, p := range parts {
		if p == exporterNone {
			return exporterNone, nil
		}
	}

	return "", fmt.Errorf("unsupported OTEL_TRACES_EXPORTER=%q (supported: otlp, none)", raw)
}

func samplerNameOrDefault(s string) string {
	if strings.TrimSpace(s) == "" {
		return "parentbased_always_on"
	}
	return s
}

func samplerFromEnv(name, arg string) (sdktrace.Sampler, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "", "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case "always_on":
		return sdktrace.AlwaysSample(), nil
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case "always_off":
		return sdktrace.NeverSample(), nil
	case "traceidratio":
		r, err := parseRatio(arg)
		if err != nil {
			return nil, err
		}
		return sdktrace.TraceIDRatioBased(r), nil
	case "parentbased_traceidratio":
		r, err := parseRatio(arg)
		if err != nil {
			return nil, err
		}
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(r)), nil
	default:
		return nil, fmt.Errorf("unsupported OTEL_TRACES_SAMPLER=%q", name)
	}
}

func parseRatio(arg string) (float64, error) {
	a := strings.TrimSpace(arg)
	if a == "" {
		return 0, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG is required for ratio-based samplers")
	}
	r, err := strconv.ParseFloat(a, 64)
	if err != nil || r < 0 || r > 1 {
		return 0, fmt.Errorf("invalid OTEL_TRACES_SAMPLER_ARG=%q; must be a float in [0,1]", arg)
	}
	return r, nil
}

func parseOTLPTimeout() time.Duration {
	val := firstNonEmpty(
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT")),
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TIMEOUT")),
	)

	const def = 10 * time.Second
	if val == "" {
		return def
	}

	if ms, err := strconv.Atoi(val); err == nil {
		if ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
		return def
	}

	if d, err := time.ParseDuration(val); err == nil {
		if d > 0 {
			return d
		}
		return def
	}

	return def
}

func httpTracesEndpointURL() (string, error) {
	if ep := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")); ep != "" {
		return ep, nil
	}

	if base := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); base != "" {
		u, err := url.Parse(base)
		if err != nil {
			return "", fmt.Errorf("invalid OTEL_EXPORTER_OTLP_ENDPOINT=%q: %w", base, err)
		}
		if u.Path == "" {
			u.Path = "/"
		}
		if !strings.HasSuffix(u.Path, "/") {
			u.Path += "/"
		}
		u.Path += "v1/traces"
		return u.String(), nil
	}

	return "http://127.0.0.1:4318/v1/traces", nil
}

func grpcTracesEndpointAndInsecure() (endpoint string, insecure bool, err error) {
	ep := firstNonEmpty(
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")),
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
	)
	if ep == "" {
		return "127.0.0.1:4317", true, nil
	}

	if u, perr := url.Parse(ep); perr == nil && u.Host != "" {
		insecure = (u.Scheme == "" || u.Scheme == "http")
		return u.Host, insecure, nil
	}

	return ep, true, nil
}

func envTrue(key string) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(key)), "true")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
