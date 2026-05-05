/*
Copyright (c) 2026 Red Hat, Inc.

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
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const metricsSubsystem = "hyperfleet_api"

const (
	labelResourceType = "resource_type"
	labelComponent    = "component"
	labelVersion      = "version"
)

const componentValue = "api"

var deletionLabels = []string{labelResourceType, labelComponent, labelVersion}

var pendingDeletionDurationBuckets = []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600}

var pendingDeletionTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: metricsSubsystem,
		Name:      "resource_pending_deletion_total",
		Help:      "Total number of resources that entered the Pending Deletion state (deleted_time set).",
	},
	deletionLabels,
)

var pendingDeletionDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Subsystem: metricsSubsystem,
		Name:      "resource_pending_deletion_duration_seconds",
		Help:      "Duration from pending deletion (deleted_time set) to hard-delete completion in seconds.",
		Buckets:   pendingDeletionDurationBuckets,
	},
	deletionLabels,
)

var registerOnce sync.Once

func RegisterMetrics() {
	registerOnce.Do(func() {
		prometheus.MustRegister(pendingDeletionTotal)
		prometheus.MustRegister(pendingDeletionDuration)
	})
}

func init() {
	RegisterMetrics()
}

func RecordPendingDeletion(resourceType string) {
	pendingDeletionTotal.With(prometheus.Labels{
		labelResourceType: resourceType,
		labelComponent:    componentValue,
		labelVersion:      api.Version,
	}).Inc()
}

func ObservePendingDeletionDuration(resourceType string, deletedAt time.Time) {
	duration := time.Since(deletedAt).Seconds()
	pendingDeletionDuration.With(prometheus.Labels{
		labelResourceType: resourceType,
		labelComponent:    componentValue,
		labelVersion:      api.Version,
	}).Observe(duration)
}

func ResetMetrics() {
	pendingDeletionTotal.Reset()
	pendingDeletionDuration.Reset()
}

// PendingDeletionCollector implements prometheus.Collector to report the number of
// resources stuck in Pending Deletion state beyond a configurable threshold.
// It queries the database on each Prometheus scrape.
const defaultQueryTimeout = 5 * time.Second

type PendingDeletionCollector struct {
	stuckDesc      *prometheus.Desc
	db             *sql.DB
	stuckThreshold time.Duration
	queryTimeout   time.Duration
}

func NewPendingDeletionCollector(db *sql.DB, stuckThreshold time.Duration) *PendingDeletionCollector {
	return &PendingDeletionCollector{
		db:             db,
		stuckThreshold: stuckThreshold,
		queryTimeout:   defaultQueryTimeout,
		stuckDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_deletion_stuck",
			"Number of resources in Pending Deletion state beyond the stuck threshold.",
			[]string{labelResourceType},
			prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version},
		),
	}
}

func (c *PendingDeletionCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.stuckDesc
}

// stuckQueries maps resource types to their pre-built SQL queries.
// Table names are compile-time constants — no user input in SQL strings.
var stuckQueries = []struct {
	query        string
	resourceType string
}{
	{"SELECT COUNT(*) FROM clusters WHERE deleted_time IS NOT NULL AND deleted_time < $1", "cluster"},
	{"SELECT COUNT(*) FROM node_pools WHERE deleted_time IS NOT NULL AND deleted_time < $1", "nodepool"},
}

func (c *PendingDeletionCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()

	threshold := time.Now().UTC().Add(-c.stuckThreshold)

	for _, q := range stuckQueries {
		var count int64
		row := c.db.QueryRowContext(ctx, q.query, threshold) //nolint:gosec // table names are compile-time constants
		if err := row.Scan(&count); err != nil {
			slog.Error("Failed to query pending deletion resources",
				"resource_type", q.resourceType,
				"error", err,
			)
			continue
		}

		ch <- prometheus.MustNewConstMetric(
			c.stuckDesc,
			prometheus.GaugeValue,
			float64(count),
			q.resourceType,
		)
	}
}

func RegisterCollector(db *sql.DB, stuckThreshold time.Duration) error {
	collector := NewPendingDeletionCollector(db, stuckThreshold)
	return prometheus.Register(collector)
}
