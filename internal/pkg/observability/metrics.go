// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package observability provides metrics and health endpoints for the provider.
package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics records provider lifecycle and OpenNebula adapter telemetry.
type Metrics struct {
	registry               *prometheus.Registry
	provisionStepDuration  *prometheus.HistogramVec
	deprovisionDuration    *prometheus.HistogramVec
	opennebulaRequestTotal *prometheus.CounterVec
	opennebulaLatency      *prometheus.HistogramVec
	retryTotal             *prometheus.CounterVec
	imageOperationTotal    *prometheus.CounterVec
}

// NewMetrics creates the Prometheus collectors used by the provider.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)

	metrics := &Metrics{
		registry: registry,
		provisionStepDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "omni",
			Subsystem: "infra_provider_opennebula",
			Name:      "provision_step_duration_seconds",
			Help:      "Duration of provision steps by step name and outcome.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"step", "outcome"}),
		deprovisionDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "omni",
			Subsystem: "infra_provider_opennebula",
			Name:      "deprovision_duration_seconds",
			Help:      "Duration of deprovision operations by outcome.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"outcome"}),
		opennebulaRequestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "omni",
			Subsystem: "infra_provider_opennebula",
			Name:      "opennebula_requests_total",
			Help:      "Total OpenNebula API requests by operation and error class.",
		}, []string{"operation", "class"}),
		opennebulaLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "omni",
			Subsystem: "infra_provider_opennebula",
			Name:      "opennebula_request_duration_seconds",
			Help:      "Latency of OpenNebula API requests by operation and error class.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"operation", "class"}),
		retryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "omni",
			Subsystem: "infra_provider_opennebula",
			Name:      "retry_total",
			Help:      "Total provider retries by step and classification.",
		}, []string{"step", "class"}),
		imageOperationTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "omni",
			Subsystem: "infra_provider_opennebula",
			Name:      "image_operations_total",
			Help:      "Total image management operations by action and outcome.",
		}, []string{"action", "outcome"}),
	}

	registry.MustRegister(
		metrics.provisionStepDuration,
		metrics.deprovisionDuration,
		metrics.opennebulaRequestTotal,
		metrics.opennebulaLatency,
		metrics.retryTotal,
		metrics.imageOperationTotal,
	)

	return metrics
}

// Handler returns the Prometheus HTTP handler.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return promhttp.Handler()
	}

	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		Registry:          m.registry,
	})
}

// ObserveProvisionStep records a provision step duration.
func (m *Metrics) ObserveProvisionStep(step, outcome string, duration time.Duration) {
	if m == nil {
		return
	}

	m.provisionStepDuration.WithLabelValues(step, outcome).Observe(duration.Seconds())
}

// ObserveDeprovision records a deprovision duration.
func (m *Metrics) ObserveDeprovision(outcome string, duration time.Duration) {
	if m == nil {
		return
	}

	m.deprovisionDuration.WithLabelValues(outcome).Observe(duration.Seconds())
}

// ObserveOpenNebulaRequest records OpenNebula API traffic.
func (m *Metrics) ObserveOpenNebulaRequest(operation, class string, duration time.Duration) {
	if m == nil {
		return
	}

	m.opennebulaRequestTotal.WithLabelValues(operation, class).Inc()
	m.opennebulaLatency.WithLabelValues(operation, class).Observe(duration.Seconds())
}

// IncRetry records a retry decision.
func (m *Metrics) IncRetry(step, class string) {
	if m == nil {
		return
	}

	m.retryTotal.WithLabelValues(step, class).Inc()
}

// ObserveImageOperation records image manager actions.
func (m *Metrics) ObserveImageOperation(action, outcome string) {
	if m == nil {
		return
	}

	m.imageOperationTotal.WithLabelValues(action, outcome).Inc()
}
