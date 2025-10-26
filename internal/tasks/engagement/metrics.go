package engagement

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "lingo-services-catalog.engagement"

type metrics struct {
	applyCounter metric.Int64Counter
	lagHistogram metric.Int64Histogram
}

func newMetrics() *metrics {
	provider := otel.GetMeterProvider()
	m := provider.Meter(meterName)
	applyCounter, _ := m.Int64Counter("catalog_engagement_apply_total")
	lagHistogram, _ := m.Int64Histogram("catalog_engagement_event_lag_ms")
	return &metrics{applyCounter: applyCounter, lagHistogram: lagHistogram}
}

func (m *metrics) recordSuccess(ctx context.Context, occurred time.Time, now time.Time) {
	if m == nil || m.applyCounter == nil {
		return
	}
	m.applyCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "success")))
	if !occurred.IsZero() && !now.IsZero() && occurred.Before(now) {
		lag := now.Sub(occurred).Milliseconds()
		m.lagHistogram.Record(ctx, lag, metric.WithAttributes(attribute.String("result", "success")))
	}
}

func (m *metrics) recordFailure(ctx context.Context) {
	if m == nil || m.applyCounter == nil {
		return
	}
	m.applyCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "failure")))
}
