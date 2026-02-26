/*
Copyright (c) 2025 Red Hat, Inc.

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

// This file contains Prometheus metrics for database operations:
//
//	hyperfleet_db_query_duration_seconds - Duration of database queries in seconds.
//	hyperfleet_db_connections_open - Number of open database connections.
//	hyperfleet_db_connections_in_use - Number of database connections currently in use.
//	hyperfleet_db_errors_total - Total number of database query errors.
//
// All metrics include `component` and `version` labels per HyperFleet Metrics Standard.
// Query metrics also include `operation` and `table` labels.
// Error metrics also include `operation` and `error_type` labels.

package db_metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// Subsystem used to define the database metrics:
const metricsSubsystem = "hyperfleet_db"

// Label names:
const (
	labelComponent = "component"
	labelVersion   = "version"
	labelOperation = "operation"
	labelTable     = "table"
	labelErrorType = "error_type"
)

// componentValue is the value for the component label.
const componentValue = "api"

// queryDurationBuckets are the histogram buckets for database query duration per standard.
var queryDurationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

// QueryDurationMetric is a histogram of database query durations in seconds.
var QueryDurationMetric = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Subsystem: metricsSubsystem,
		Name:      "query_duration_seconds",
		Help:      "Duration of database queries in seconds.",
		Buckets:   queryDurationBuckets,
	},
	[]string{labelOperation, labelTable, labelComponent, labelVersion},
)

// ErrorsMetric is a counter of database query errors.
var ErrorsMetric = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: metricsSubsystem,
		Name:      "errors_total",
		Help:      "Total number of database query errors.",
	},
	[]string{labelOperation, labelErrorType, labelComponent, labelVersion},
)

// ConnectionsOpenDesc is the descriptor for the connections_open gauge.
var ConnectionsOpenDesc = prometheus.NewDesc(
	metricsSubsystem+"_connections_open",
	"Number of open database connections.",
	nil,
	prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version},
)

// ConnectionsInUseDesc is the descriptor for the connections_in_use gauge.
var ConnectionsInUseDesc = prometheus.NewDesc(
	metricsSubsystem+"_connections_in_use",
	"Number of database connections currently in use.",
	nil,
	prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version},
)

// ResetMetrics resets all resettable database metrics.
func ResetMetrics() {
	QueryDurationMetric.Reset()
	ErrorsMetric.Reset()
}

var registerOnce sync.Once

// RegisterMetrics registers the database metrics with Prometheus.
// It is safe to call multiple times; registration happens only once.
func RegisterMetrics() {
	registerOnce.Do(func() {
		prometheus.MustRegister(QueryDurationMetric)
		prometheus.MustRegister(ErrorsMetric)
	})
}

func init() {
	RegisterMetrics()
}
