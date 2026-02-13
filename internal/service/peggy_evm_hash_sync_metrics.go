package service

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PeggyEvmSyncMetrics holds metrics for Peggy EVM hash sync service.
type PeggyEvmSyncMetrics struct {
	// queueLength tracks the number of tasks in the queue
	queueLength prometheus.Gauge

	// taskDuration tracks the duration of task processing
	taskDuration *prometheus.HistogramVec

	// taskCount tracks the total number of tasks processed
	taskCount *prometheus.CounterVec

	// rpcDuration tracks the duration of RPC calls
	rpcDuration *prometheus.HistogramVec
}

// NewPeggyEvmSyncMetrics creates a new metrics instance.
// If registerer is nil, uses the default global registry.
func NewPeggyEvmSyncMetrics(registerer prometheus.Registerer) *PeggyEvmSyncMetrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	queueLength := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "peggy_evm_sync_queue_length",
		Help: "Number of tasks waiting for EVM hash sync",
	})
	registerer.MustRegister(queueLength)

	taskDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "peggy_evm_sync_task_duration_seconds",
		Help:    "Duration of EVM hash sync task processing",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"type", "status"})
	registerer.MustRegister(taskDuration)

	taskCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "peggy_evm_sync_task_total",
		Help: "Total number of sync tasks processed",
	}, []string{"type", "status"})
	registerer.MustRegister(taskCount)

	rpcDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "peggy_evm_rpc_duration_seconds",
		Help:    "Duration of RPC calls to EVM chain",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
	}, []string{"method", "status"})
	registerer.MustRegister(rpcDuration)

	return &PeggyEvmSyncMetrics{
		queueLength: queueLength,
		taskDuration: taskDuration,
		taskCount: taskCount,
		rpcDuration: rpcDuration,
	}
}

// RecordTask records a task processing result.
func (m *PeggyEvmSyncMetrics) RecordTask(taskType, status string, duration time.Duration) {
	m.taskDuration.WithLabelValues(taskType, status).Observe(duration.Seconds())
	m.taskCount.WithLabelValues(taskType, status).Inc()
}

// UpdateQueueLength updates the queue length metric.
func (m *PeggyEvmSyncMetrics) UpdateQueueLength(length int) {
	m.queueLength.Set(float64(length))
}

// RecordRPC records an RPC call result.
func (m *PeggyEvmSyncMetrics) RecordRPC(method, status string, duration time.Duration) {
	m.rpcDuration.WithLabelValues(method, status).Observe(duration.Seconds())
}
