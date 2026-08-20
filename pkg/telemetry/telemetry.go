package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string
	SampleRate     float64
}

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	httpRequestsInFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Current number of in-flight HTTP requests",
	}, []string{"method", "path"})
)

func Init(cfg Config) (func(context.Context) error, error) {
	ctx := context.Background()

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	// Trace exporter
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	// Metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithTLSCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	// Tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))),
	)
	otel.SetTracerProvider(tp)

	// Meter provider
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// Propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info().Str("service", cfg.ServiceName).Msg("Telemetry initialized")

	// Return shutdown function
	return func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracer provider: %w", err))
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown meter provider: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("%v", errs)
		}
		return nil
	}, nil
}

func GinMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		httpRequestsInFlight.WithLabelValues(c.Request.Method, path).Inc()
		defer httpRequestsInFlight.WithLabelValues(c.Request.Method, path).Dec()

		// Add trace context to response headers
		traceID := otel.Tracer(serviceName).Start(c.Request.Context(), "").SpanContext().TraceID()
		if traceID.IsValid() {
			c.Header("X-Trace-ID", traceID.String())
		}

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer.Status()

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, fmt.Sprintf("%d", status)).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

// Custom metrics
var (
	DeploymentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "deployments_total",
		Help: "Total number of deployments",
	}, []string{"project", "environment", "status"})

	DeploymentDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "deployment_duration_seconds",
		Help:    "Deployment duration in seconds",
		Buckets: []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
	}, []string{"project", "environment"})

	AIJobsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_jobs_total",
		Help: "Total number of AI jobs",
	}, []string{"project", "type", "status"})

	AIJobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_job_duration_seconds",
		Help:    "AI job duration in seconds",
		Buckets: []float64{5, 10, 30, 60, 120, 300, 600},
	}, []string{"project", "type"})

	ActiveEnvironments = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "active_environments",
		Help: "Number of active environments per project",
	}, []string{"project"})

	WebSocketConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_connections",
		Help: "Current number of WebSocket connections",
	})
)

func RecordDeployment(project, environment, status string, duration float64) {
	DeploymentsTotal.WithLabelValues(project, environment, status).Inc()
	DeploymentDuration.WithLabelValues(project, environment).Observe(duration)
}

func RecordAIJob(project, jobType, status string, duration float64) {
	AIJobsTotal.WithLabelValues(project, jobType, status).Inc()
	AIJobDuration.WithLabelValues(project, jobType).Observe(duration)
}

func SetActiveEnvironments(project string, count int) {
	ActiveEnvironments.WithLabelValues(project).Set(float64(count))
}

func IncWebSocketConnections() {
	WebSocketConnections.Inc()
}

func DecWebSocketConnections() {
	WebSocketConnections.Dec()
}

// Metric helper for business metrics
type BusinessMetrics struct {
	meter metric.Meter
}

func NewBusinessMetrics(serviceName string) *BusinessMetrics {
	return &BusinessMetrics{
		meter: otel.Meter(serviceName),
	}
}

func (b *BusinessMetrics) RecordProjectCreated(ctx context.Context, projectID string) {
	counter, _ := b.meter.Int64Counter("projects.created",
		metric.WithDescription("Number of projects created"))
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("project_id", projectID)))
}

func (b *BusinessMetrics) RecordEnvironmentCreated(ctx context.Context, projectID, envID string) {
	counter, _ := b.meter.Int64Counter("environments.created",
		metric.WithDescription("Number of environments created"))
	counter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("project_id", projectID),
			attribute.String("environment_id", envID),
		))
}

func (b *BusinessMetrics) RecordDeploymentTriggered(ctx context.Context, projectID, envID, trigger string) {
	counter, _ := b.meter.Int64Counter("deployments.triggered",
		metric.WithDescription("Number of deployments triggered"))
	counter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("project_id", projectID),
			attribute.String("environment_id", envID),
			attribute.String("trigger", trigger),
		))
}

func (b *BusinessMetrics) RecordAIJobStarted(ctx context.Context, projectID, jobType string) {
	counter, _ := b.meter.Int64Counter("ai.jobs.started",
		metric.WithDescription("Number of AI jobs started"))
	counter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("project_id", projectID),
			attribute.String("job_type", jobType),
		))
}

func (b *BusinessMetrics) RecordAIJobCompleted(ctx context.Context, projectID, jobType string, success bool) {
	counter, _ := b.meter.Int64Counter("ai.jobs.completed",
		metric.WithDescription("Number of AI jobs completed"))
	counter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("project_id", projectID),
			attribute.String("job_type", jobType),
			attribute.Bool("success", success),
		))
}