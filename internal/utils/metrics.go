package utils

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus metrics for the generator
type Metrics struct {
	// Counters
	MessagesPublished   prometheus.Counter
	MessagesFailed      prometheus.Counter
	LoginAttempts       prometheus.Counter
	LoginFailures       prometheus.Counter
	SubscriptionSuccess prometheus.Counter
	SubscriptionFailure prometheus.Counter
	DeletionsAttempted  prometheus.Counter

	// Gauges
	ActiveConnections prometheus.Gauge
	ActiveScenarios   prometheus.Gauge

	// Histograms
	MessageLatency    prometheus.Histogram
	ScenarioDuration  prometheus.Histogram
	ConnectionLatency prometheus.Histogram
}

// NewMetrics creates and registers Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		MessagesPublished: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_messages_published_total",
			Help: "Total number of messages published",
		}),
		MessagesFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_messages_failed_total",
			Help: "Total number of message publication failures",
		}),
		LoginAttempts: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_login_attempts_total",
			Help: "Total number of login attempts",
		}),
		LoginFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_login_failures_total",
			Help: "Total number of login failures",
		}),
		SubscriptionSuccess: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_subscription_success_total",
			Help: "Total successful subscriptions",
		}),
		SubscriptionFailure: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_subscription_failure_total",
			Help: "Total failed subscriptions",
		}),
		DeletionsAttempted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "generator_deletions_attempted_total",
			Help: "Total number of message deletions attempted",
		}),
		ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "generator_active_connections",
			Help: "Number of active WebSocket connections",
		}),
		ActiveScenarios: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "generator_active_scenarios",
			Help: "Number of active scenarios",
		}),
		MessageLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "generator_message_latency_ms",
			Help:    "Message operation latency in milliseconds",
			Buckets: []float64{10, 50, 100, 500, 1000, 2000, 5000},
		}),
		ScenarioDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "generator_scenario_duration_seconds",
			Help:    "Scenario execution duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 300},
		}),
		ConnectionLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "generator_connection_latency_ms",
			Help:    "WebSocket connection latency in milliseconds",
			Buckets: []float64{10, 50, 100, 500, 1000, 2000},
		}),
	}
}

// RecordMessage records a message operation
func (m *Metrics) RecordMessage(success bool) {
	m.MessagesPublished.Inc()
	if !success {
		m.MessagesFailed.Inc()
	}
}

// RecordLogin records a login attempt
func (m *Metrics) RecordLogin(success bool) {
	m.LoginAttempts.Inc()
	if !success {
		m.LoginFailures.Inc()
	}
}

// RecordSubscription records a subscription attempt
func (m *Metrics) RecordSubscription(success bool) {
	if success {
		m.SubscriptionSuccess.Inc()
	} else {
		m.SubscriptionFailure.Inc()
	}
}

// RecordDeletion records a message deletion attempt
func (m *Metrics) RecordDeletion() {
	m.DeletionsAttempted.Inc()
}

// RecordMessageLatency records message operation latency
func (m *Metrics) RecordMessageLatency(ms float64) {
	m.MessageLatency.Observe(ms)
}

// RecordScenarioDuration records scenario execution duration
func (m *Metrics) RecordScenarioDuration(seconds float64) {
	m.ScenarioDuration.Observe(seconds)
}

// RecordConnectionLatency records WebSocket connection latency
func (m *Metrics) RecordConnectionLatency(ms float64) {
	m.ConnectionLatency.Observe(ms)
}

// SetActiveConnections sets gauge for active connections
func (m *Metrics) SetActiveConnections(count float64) {
	m.ActiveConnections.Set(count)
}

// SetActiveScenarios sets gauge for active scenarios
func (m *Metrics) SetActiveScenarios(count float64) {
	m.ActiveScenarios.Set(count)
}
