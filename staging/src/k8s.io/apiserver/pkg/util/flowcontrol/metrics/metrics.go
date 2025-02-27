/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	compbasemetrics "k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	basemetricstestutil "k8s.io/component-base/metrics/testutil"
	"k8s.io/utils/clock"
)

const (
	namespace = "apiserver"
	subsystem = "flowcontrol"
)

const (
	requestKind   = "request_kind"
	priorityLevel = "priority_level"
	flowSchema    = "flow_schema"
	phase         = "phase"
	mark          = "mark"
)

var (
	queueLengthBuckets            = []float64{0, 10, 25, 50, 100, 250, 500, 1000}
	requestDurationSecondsBuckets = []float64{0, 0.005, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30}
)

var registerMetrics sync.Once

// Register all metrics.
func Register() {
	registerMetrics.Do(func() {
		for _, metric := range metrics {
			legacyregistry.MustRegister(metric)
		}
	})
}

type resettable interface {
	Reset()
}

// Reset all metrics to zero
func Reset() {
	for _, metric := range metrics {
		rm := metric.(resettable)
		rm.Reset()
	}
}

// GatherAndCompare the given metrics with the given Prometheus syntax expected value
func GatherAndCompare(expected string, metricNames ...string) error {
	return basemetricstestutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(expected), metricNames...)
}

// Registerables is a slice of Registerable
type Registerables []compbasemetrics.Registerable

// Append adds more
func (rs Registerables) Append(more ...compbasemetrics.Registerable) Registerables {
	return append(rs, more...)
}

var (
	apiserverRejectedRequestsTotal = compbasemetrics.NewCounterVec(
		&compbasemetrics.CounterOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "rejected_requests_total",
			Help:           "Number of requests rejected by API Priority and Fairness system",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema, "reason"},
	)
	apiserverDispatchedRequestsTotal = compbasemetrics.NewCounterVec(
		&compbasemetrics.CounterOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "dispatched_requests_total",
			Help:           "Number of requests released by API Priority and Fairness system for service",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)
	// PriorityLevelExecutionSeatsObserverGenerator creates observers of seats occupied throughout execution for priority levels
	PriorityLevelExecutionSeatsObserverGenerator = NewSampleAndWaterMarkHistogramsGenerator(clock.RealClock{}, time.Millisecond,
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "priority_level_seat_count_samples",
			Help:           "Periodic observations of the number of requests",
			Buckets:        []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
			ConstLabels:    map[string]string{phase: "executing"},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "priority_level_seat_count_watermarks",
			Help:           "Watermarks of the number of requests",
			Buckets:        []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
			ConstLabels:    map[string]string{phase: "executing"},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel},
	)
	// PriorityLevelConcurrencyObserverPairGenerator creates pairs that observe concurrency for priority levels
	PriorityLevelConcurrencyObserverPairGenerator = NewSampleAndWaterMarkHistogramsPairGenerator(clock.RealClock{}, time.Millisecond,
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "priority_level_request_count_samples",
			Help:           "Periodic observations of the number of requests",
			Buckets:        []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "priority_level_request_count_watermarks",
			Help:           "Watermarks of the number of requests",
			Buckets:        []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel},
	)
	// ReadWriteConcurrencyObserverPairGenerator creates pairs that observe concurrency broken down by mutating vs readonly
	ReadWriteConcurrencyObserverPairGenerator = NewSampleAndWaterMarkHistogramsPairGenerator(clock.RealClock{}, time.Millisecond,
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "read_vs_write_request_count_samples",
			Help:           "Periodic observations of the number of requests",
			Buckets:        []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "read_vs_write_request_count_watermarks",
			Help:           "Watermarks of the number of requests",
			Buckets:        []float64{0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{requestKind},
	)
	apiserverCurrentR = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "current_r",
			Help:           "R(time of last change)",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel},
	)
	apiserverDispatchR = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "dispatch_r",
			Help:           "R(time of last dispatch)",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel},
	)
	apiserverLatestS = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "latest_s",
			Help:           "S(most recently dispatched request)",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel},
	)
	apiserverNextSBounds = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "next_s_bounds",
			Help:           "min and max, over queues, of S(oldest waiting request in queue)",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, "bound"},
	)
	apiserverNextDiscountedSBounds = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "next_discounted_s_bounds",
			Help:           "min and max, over queues, of S(oldest waiting request in queue) - estimated work in progress",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, "bound"},
	)
	apiserverCurrentInqueueRequests = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "current_inqueue_requests",
			Help:           "Number of requests currently pending in queues of the API Priority and Fairness system",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)
	apiserverRequestQueueLength = compbasemetrics.NewHistogramVec(
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "request_queue_length_after_enqueue",
			Help:           "Length of queue in the API Priority and Fairness system, as seen by each request after it is enqueued",
			Buckets:        queueLengthBuckets,
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)
	apiserverRequestConcurrencyLimit = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "request_concurrency_limit",
			Help:           "Shared concurrency limit in the API Priority and Fairness system",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel},
	)
	apiserverCurrentExecutingRequests = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "current_executing_requests",
			Help:           "Number of requests in regular execution phase in the API Priority and Fairness system",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)
	apiserverRequestConcurrencyInUse = compbasemetrics.NewGaugeVec(
		&compbasemetrics.GaugeOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "request_concurrency_in_use",
			Help:           "Concurrency (number of seats) occupided by the currently executing (all phases count) requests in the API Priority and Fairness system",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)
	apiserverRequestWaitingSeconds = compbasemetrics.NewHistogramVec(
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "request_wait_duration_seconds",
			Help:           "Length of time a request spent waiting in its queue",
			Buckets:        requestDurationSecondsBuckets,
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema, "execute"},
	)
	apiserverRequestExecutionSeconds = compbasemetrics.NewHistogramVec(
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "request_execution_seconds",
			Help:           "Duration of regular phase of request execution in the API Priority and Fairness system",
			Buckets:        requestDurationSecondsBuckets,
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema, "type"},
	)
	watchCountSamples = compbasemetrics.NewHistogramVec(
		&compbasemetrics.HistogramOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "watch_count_samples",
			Help:           "count of watchers for mutating requests in API Priority and Fairness",
			Buckets:        []float64{0, 1, 10, 100, 1000, 10000},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)
	apiserverEpochAdvances = compbasemetrics.NewCounterVec(
		&compbasemetrics.CounterOpts{
			Namespace:      namespace,
			Subsystem:      subsystem,
			Name:           "epoch_advance_total",
			Help:           "Number of times the queueset's progress meter jumped backward",
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, "success"},
	)
	apiserverWorkEstimatedSeats = compbasemetrics.NewHistogramVec(
		&compbasemetrics.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "work_estimated_seats",
			Help:      "Number of estimated seats (maximum of initial and final seats) associated with requests in API Priority and Fairness",
			// the upper bound comes from the maximum number of seats a request
			// can occupy which is currently set at 10.
			Buckets:        []float64{1, 2, 4, 10},
			StabilityLevel: compbasemetrics.ALPHA,
		},
		[]string{priorityLevel, flowSchema},
	)

	metrics = Registerables{
		apiserverRejectedRequestsTotal,
		apiserverDispatchedRequestsTotal,
		apiserverCurrentR,
		apiserverDispatchR,
		apiserverLatestS,
		apiserverNextSBounds,
		apiserverNextDiscountedSBounds,
		apiserverCurrentInqueueRequests,
		apiserverRequestQueueLength,
		apiserverRequestConcurrencyLimit,
		apiserverRequestConcurrencyInUse,
		apiserverCurrentExecutingRequests,
		apiserverRequestWaitingSeconds,
		apiserverRequestExecutionSeconds,
		watchCountSamples,
		apiserverEpochAdvances,
		apiserverWorkEstimatedSeats,
	}.
		Append(PriorityLevelExecutionSeatsObserverGenerator.metrics()...).
		Append(PriorityLevelConcurrencyObserverPairGenerator.metrics()...).
		Append(ReadWriteConcurrencyObserverPairGenerator.metrics()...)
)

// AddRequestsInQueues adds the given delta to the gauge of the # of requests in the queues of the specified flowSchema and priorityLevel
func AddRequestsInQueues(ctx context.Context, priorityLevel, flowSchema string, delta int) {
	apiserverCurrentInqueueRequests.WithLabelValues(priorityLevel, flowSchema).Add(float64(delta))
}

// AddRequestsExecuting adds the given delta to the gauge of executing requests of the given flowSchema and priorityLevel
func AddRequestsExecuting(ctx context.Context, priorityLevel, flowSchema string, delta int) {
	apiserverCurrentExecutingRequests.WithLabelValues(priorityLevel, flowSchema).Add(float64(delta))
}

// SetCurrentR sets the current-R (virtualTime) gauge for the given priority level
func SetCurrentR(priorityLevel string, r float64) {
	apiserverCurrentR.WithLabelValues(priorityLevel).Set(r)
}

// SetLatestS sets the latest-S (virtual time of dispatched request) gauge for the given priority level
func SetDispatchMetrics(priorityLevel string, r, s, sMin, sMax, discountedSMin, discountedSMax float64) {
	apiserverDispatchR.WithLabelValues(priorityLevel).Set(r)
	apiserverLatestS.WithLabelValues(priorityLevel).Set(s)
	apiserverNextSBounds.WithLabelValues(priorityLevel, "min").Set(sMin)
	apiserverNextSBounds.WithLabelValues(priorityLevel, "max").Set(sMax)
	apiserverNextDiscountedSBounds.WithLabelValues(priorityLevel, "min").Set(discountedSMin)
	apiserverNextDiscountedSBounds.WithLabelValues(priorityLevel, "max").Set(discountedSMax)
}

// AddRequestConcurrencyInUse adds the given delta to the gauge of concurrency in use by
// the currently executing requests of the given flowSchema and priorityLevel
func AddRequestConcurrencyInUse(priorityLevel, flowSchema string, delta int) {
	apiserverRequestConcurrencyInUse.WithLabelValues(priorityLevel, flowSchema).Add(float64(delta))
}

// UpdateSharedConcurrencyLimit updates the value for the concurrency limit in flow control
func UpdateSharedConcurrencyLimit(priorityLevel string, limit int) {
	apiserverRequestConcurrencyLimit.WithLabelValues(priorityLevel).Set(float64(limit))
}

// AddReject increments the # of rejected requests for flow control
func AddReject(ctx context.Context, priorityLevel, flowSchema, reason string) {
	apiserverRejectedRequestsTotal.WithContext(ctx).WithLabelValues(priorityLevel, flowSchema, reason).Add(1)
}

// AddDispatch increments the # of dispatched requests for flow control
func AddDispatch(ctx context.Context, priorityLevel, flowSchema string) {
	apiserverDispatchedRequestsTotal.WithContext(ctx).WithLabelValues(priorityLevel, flowSchema).Add(1)
}

// ObserveQueueLength observes the queue length for flow control
func ObserveQueueLength(ctx context.Context, priorityLevel, flowSchema string, length int) {
	apiserverRequestQueueLength.WithContext(ctx).WithLabelValues(priorityLevel, flowSchema).Observe(float64(length))
}

// ObserveWaitingDuration observes the queue length for flow control
func ObserveWaitingDuration(ctx context.Context, priorityLevel, flowSchema, execute string, waitTime time.Duration) {
	apiserverRequestWaitingSeconds.WithContext(ctx).WithLabelValues(priorityLevel, flowSchema, execute).Observe(waitTime.Seconds())
}

// ObserveExecutionDuration observes the execution duration for flow control
func ObserveExecutionDuration(ctx context.Context, priorityLevel, flowSchema string, executionTime time.Duration) {
	reqType := "regular"
	if requestInfo, ok := apirequest.RequestInfoFrom(ctx); ok && requestInfo.Verb == "watch" {
		reqType = requestInfo.Verb
	}
	apiserverRequestExecutionSeconds.WithContext(ctx).WithLabelValues(priorityLevel, flowSchema, reqType).Observe(executionTime.Seconds())
}

// ObserveWatchCount notes a sampling of a watch count
func ObserveWatchCount(ctx context.Context, priorityLevel, flowSchema string, count int) {
	watchCountSamples.WithLabelValues(priorityLevel, flowSchema).Observe(float64(count))
}

// AddEpochAdvance notes an advance of the progress meter baseline for a given priority level
func AddEpochAdvance(ctx context.Context, priorityLevel string, success bool) {
	apiserverEpochAdvances.WithContext(ctx).WithLabelValues(priorityLevel, strconv.FormatBool(success)).Inc()
}

// ObserveWorkEstimatedSeats notes a sampling of estimated seats associated with a request
func ObserveWorkEstimatedSeats(priorityLevel, flowSchema string, seats int) {
	apiserverWorkEstimatedSeats.WithLabelValues(priorityLevel, flowSchema).Observe(float64(seats))
}
