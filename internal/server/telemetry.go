package server

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	kmetrics "github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	promexp "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Telemetry bundles the shared metric instruments and registry.
type Telemetry struct {
	MeterProvider      *sdkmetric.MeterProvider
	RequestCounter     metric.Int64Counter
	SecondsHistogram   metric.Float64Histogram
	PrometheusRegistry *prometheus.Registry
}

// NewTelemetry prepares OpenTelemetry metrics instruments and a Prometheus exporter.
func NewTelemetry(logger log.Logger) (*Telemetry, func(), error) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
	)
	exporter, err := promexp.New(
		promexp.WithRegisterer(registry),
		promexp.WithoutUnits(),
	)
	if err != nil {
		return nil, nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithView(kmetrics.DefaultSecondsHistogramView(kmetrics.DefaultServerSecondsHistogramName)),
	)
	otel.SetMeterProvider(mp)

	meter := mp.Meter("kratos-template")

	requestCounter, err := kmetrics.DefaultRequestsCounter(meter, kmetrics.DefaultServerRequestsCounterName)
	if err != nil {
		return nil, nil, err
	}
	secondsHistogram, err := kmetrics.DefaultSecondsHistogram(meter, kmetrics.DefaultServerSecondsHistogramName)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mp.Shutdown(ctx); err != nil {
			log.NewHelper(logger).Warnf("shutdown meter provider: %v", err)
		}
	}

	return &Telemetry{
		MeterProvider:      mp,
		RequestCounter:     requestCounter,
		SecondsHistogram:   secondsHistogram,
		PrometheusRegistry: registry,
	}, cleanup, nil
}
