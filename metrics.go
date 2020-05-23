package radish

import (
	"fmt"
	"net/http"

	"github.com/kansaslabs/x/out"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	pmWorkers     prometheus.Gauge // number of available workers
	pmQueueSize   prometheus.Gauge // number of tasks in the queue awaiting handling
	pmPercentFull prometheus.Gauge // the percent of the queue that is full * 100
	// pmPercentSuccess *prometheus.GaugeVec     // the percent of tasks successfully completed, labeled by task
	pmTasksSucceeded *prometheus.CounterVec   // the count of successfully completed tasks, labeled by task type
	pmTasksFailed    *prometheus.CounterVec   // the count of failed tasks, labeled by task type
	pmTaskLatency    *prometheus.HistogramVec // the time it is taking for tasks to complete, labeled by task type, success, and failure
)

const (
	pmNamespace = "radish"
)

func initMetrics() {
	pmWorkers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: pmNamespace,
		Name:      "workers",
		Help:      "The number of available workers",
	})

	pmQueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: pmNamespace,
		Name:      "queue_size",
		Help:      "number of tasks in the queue awaiting handling",
	})

	pmPercentFull = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: pmNamespace,
		Name:      "percent_full",
		Help:      "the percent of the queue that is already full",
	})

	// TODO: Come back to this; would need to keep track of global tasks?
	// pmPercentSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	// 	Namespace: pmNamespace,
	// 	Name:      "percent_success",
	// 	Help:      "the percent of tasks successfully completed, labeled by task",
	// }, []string{"task"})

	pmTasksSucceeded = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: pmNamespace,
		Name:      "tasks_succeeded",
		Help:      "the count of tasks successfully completed, labeled by task type",
	}, []string{"task"})

	pmTasksFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: pmNamespace,
		Name:      "tasks_failed",
		Help:      "the count of failed tasks, labeled by task type",
	}, []string{"task"})

	pmTaskLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: pmNamespace,
		Name:      "task_latency",
		Help:      "time to task completion, labeled by task type, success, and failure",
	}, []string{"task", "result"})
}

func serveMetrics(metricsAddr string) {
	out.Status("serving prometheus metrics at http://%s/metrics", metricsAddr)
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(metricsAddr, nil); err != nil {
		out.Warne(err)
	}
}

func registerMetrics() error {
	if err := prometheus.Register(pmWorkers); err != nil {
		return fmt.Errorf("did not register %s: %s", pmWorkers, err)
	}
	if err := prometheus.Register(pmQueueSize); err != nil {
		return fmt.Errorf("did not register %s: %s", pmQueueSize, err)
	}
	if err := prometheus.Register(pmPercentFull); err != nil {
		return fmt.Errorf("did not register %s: %s", pmPercentFull, err)
	}
	// if err := prometheus.Register(pmPercentSuccess); err != nil {
	// 	return fmt.Errorf("did not register %v: %s", pmPercentSuccess, err)
	// }
	if err := prometheus.Register(pmTasksSucceeded); err != nil {
		return fmt.Errorf("did not register %v: %s", pmTasksSucceeded, err)
	}
	if err := prometheus.Register(pmTasksFailed); err != nil {
		return fmt.Errorf("did not register %v: %s", pmTasksFailed, err)
	}
	if err := prometheus.Register(pmTaskLatency); err != nil {
		return fmt.Errorf("did not register %v: %s", pmTaskLatency, err)
	}

	return nil
}
