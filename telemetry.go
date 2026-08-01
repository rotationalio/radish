package radish

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.rtnl.ai/radish/status"
)

const (
	instrumentation = "go.rtnl.ai/radish"
	messagingSystem = "radish"
)

type Metrics struct {
	meter        metric.Meter
	queueSize    metric.Int64ObservableUpDownCounter
	completed    metric.Int64Counter
	taskDuration metric.Float64Histogram
	sent         metric.Int64Counter
	consumed     metric.Int64Counter
}

func NewMetrics(mp metric.MeterProvider) (m *Metrics, err error) {
	if mp == nil {
		mp = otel.GetMeterProvider()
	}

	m = &Metrics{
		meter: mp.Meter(instrumentation, MetricInstrumentationVersion()),
	}

	if m.queueSize, err = m.meter.Int64ObservableUpDownCounter(
		"radish.queue_size",
		metric.WithUnit("{task}"),
		metric.WithDescription("The current number of tasks pending in the queue."),
	); err != nil {
		return nil, err
	}

	if m.completed, err = m.meter.Int64Counter(
		"radish.completed",
		metric.WithUnit("{task}"),
		metric.WithDescription("The number of tasks completed by radish governed by their state."),
	); err != nil {
		return nil, err
	}

	// See: https://opentelemetry.io/docs/specs/semconv/messaging/messaging-metrics/
	if m.taskDuration, err = m.meter.Float64Histogram(
		"messaging.process.duration",
		metric.WithDescription("The duration of a task execution operation."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10),
	); err != nil {
		return nil, err
	}

	if m.sent, err = m.meter.Int64Counter(
		"messaging.client.sent.messages",
		metric.WithUnit("{message}"),
		metric.WithDescription("The number of tasks sent to the broker including retries."),
	); err != nil {
		return nil, err
	}

	if m.consumed, err = m.meter.Int64Counter(
		"messaging.client.consumed.messages",
		metric.WithUnit("{message}"),
		metric.WithDescription("The number of tasks consumed (dequeued) from the broker."),
	); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Metrics) RegisterQueueSizeCallback(f metric.Callback) error {
	_, err := m.meter.RegisterCallback(f, m.queueSize)
	return err
}

func (r *Radish) QueueSize(ctx context.Context, observer metric.Observer) error {
	return nil
}

func (m *Metrics) recordCompletedTask(ctx context.Context, kind string, status status.Status) {
	m.completed.Add(
		ctx, 1,
		metric.WithAttributes(
			attribute.String("task.kind", kind),
			attribute.String("task.status", status.String()),
		),
	)
}

func (m *Metrics) recordTaskDuration(ctx context.Context, duration time.Duration, kind string) {
	m.taskDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("messaging.operation.name", "execute"),
		attribute.String("messaging.system", messagingSystem),
		attribute.String("messaging.destination.name", kind),
	))
}

func (m *Metrics) recordTaskDurationFailed(ctx context.Context, duration time.Duration, kind string, errtype string) {
	m.taskDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("messaging.operation.name", "execute"),
		attribute.String("messaging.system", messagingSystem),
		attribute.String("messaging.destination.name", kind),
		attribute.String("error.type", errtype),
	))
}

func (m *Metrics) incrSentMessages(ctx context.Context, kind string) {
	m.sent.Add(
		ctx, 1,
		metric.WithAttributes(
			attribute.String("messaging.operation.name", "enqueue"),
			attribute.String("messaging.system", messagingSystem),
			attribute.String("messaging.destination.name", kind),
		),
	)
}

func (m *Metrics) incrConsumedMessages(ctx context.Context, kind string) {
	m.consumed.Add(
		ctx, 1,
		metric.WithAttributes(
			attribute.String("messaging.operation.name", "dequeue"),
			attribute.String("messaging.system", messagingSystem),
			attribute.String("messaging.destination.name", kind),
		),
	)
}

func (m *Metrics) consumeCancelledMessages(ctx context.Context, kind string) {
	m.consumed.Add(
		ctx, 1,
		metric.WithAttributes(
			attribute.String("messaging.operation.name", "cancelled"),
			attribute.String("messaging.system", messagingSystem),
			attribute.String("messaging.destination.name", kind),
		),
	)
}
