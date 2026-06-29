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
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const metricsSubsystem = "hyperfleet_api"

const (
	labelResourceType = "resource_type"
	labelComponent    = "component"
	labelVersion      = "version"
	labelIsDelete     = "is_delete"
)

const componentValue = "api"

const defaultQueryTimeout = 30 * time.Second

var reconciliationLabels = []string{labelResourceType, labelIsDelete}

var reconciliationStartedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: metricsSubsystem,
		Name:      "reconciliation_started_total",
		Help: "Total number of resources that entered the unreconciled state " +
			"(Reconciled condition transitioned to False).",
		ConstLabels: prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version},
	},
	reconciliationLabels,
)

var reconciliationRegisterOnce sync.Once

func RegisterReconciliationMetrics() {
	reconciliationRegisterOnce.Do(func() {
		prometheus.MustRegister(reconciliationStartedTotal)
	})
}

func init() {
	RegisterReconciliationMetrics()
}

func RecordReconciliationStarted(resourceType string, isDelete bool) {
	reconciliationStartedTotal.With(prometheus.Labels{
		labelResourceType: resourceType,
		labelIsDelete:     fmt.Sprintf("%t", isDelete),
	}).Inc()
}

func ResetReconciliationMetrics() {
	reconciliationStartedTotal.Reset()
}

type ReconciliationCollector struct {
	pendingDesc  *prometheus.Desc
	stuckDesc    *prometheus.Desc
	durationDesc *prometheus.Desc

	db             *sql.DB
	stuckThreshold time.Duration
	queryTimeout   time.Duration
}

func NewReconciliationCollector(db *sql.DB, stuckThreshold time.Duration) *ReconciliationCollector {
	constLabels := prometheus.Labels{labelComponent: componentValue, labelVersion: api.Version}
	variableLabels := []string{labelResourceType, labelIsDelete}

	return &ReconciliationCollector{
		db:             db,
		stuckThreshold: stuckThreshold,
		queryTimeout:   defaultQueryTimeout,
		pendingDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_reconciliation",
			"Number of resources currently pending reconciliation (Reconciled=False).",
			variableLabels,
			constLabels,
		),
		stuckDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_reconciliation_stuck",
			"Number of resources pending reconciliation beyond the stuck threshold.",
			variableLabels,
			constLabels,
		),
		durationDesc: prometheus.NewDesc(
			metricsSubsystem+"_resource_pending_reconciliation_stuck_duration_seconds",
			"Maximum duration in seconds that any resource has been stuck pending reconciliation.",
			variableLabels,
			constLabels,
		),
	}
}

func (c *ReconciliationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.pendingDesc
	ch <- c.stuckDesc
	ch <- c.durationDesc
}

// reconciliationQuery uses a CTE to parse JSONB once per row, then computes
// all three metrics (pending count, stuck count, max stuck duration) in a
// single query — 1 round-trip instead of 3.
//
//nolint:lll // SQL readability — breaking these lines across Go string boundaries would harm clarity
const reconciliationQuery = `
WITH unreconciled AS (
    SELECT 'cluster' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete,
           (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz AS transition_time
    FROM clusters
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
    UNION ALL
    SELECT 'nodepool' AS resource_type,
           CASE WHEN deleted_time IS NOT NULL THEN 'true' ELSE 'false' END AS is_delete,
           (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'last_transition_time')::timestamptz AS transition_time
    FROM node_pools
    WHERE (jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")') ->> 'status') = 'False'
)
SELECT resource_type,
       is_delete,
       COUNT(*) AS pending,
       COUNT(*) FILTER (WHERE transition_time < $1) AS stuck,
       COALESCE(MAX(EXTRACT(EPOCH FROM (NOW() - transition_time))) FILTER (WHERE transition_time < $1), 0) AS max_duration
FROM unreconciled
GROUP BY resource_type, is_delete`

func (c *ReconciliationCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()

	threshold := time.Now().UTC().Add(-c.stuckThreshold)

	rows, err := c.db.QueryContext(ctx, reconciliationQuery, threshold) //nolint:gosec // compile-time SQL
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to query reconciliation metrics")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType, isDelete string
		var pending, stuck int64
		var maxDuration float64

		if err := rows.Scan(&resourceType, &isDelete, &pending, &stuck, &maxDuration); err != nil {
			logger.WithError(ctx, err).Error("Failed to scan reconciliation metric row")
			continue
		}

		labels := []string{resourceType, isDelete}
		ch <- prometheus.MustNewConstMetric(c.pendingDesc, prometheus.GaugeValue, float64(pending), labels...)
		ch <- prometheus.MustNewConstMetric(c.stuckDesc, prometheus.GaugeValue, float64(stuck), labels...)
		ch <- prometheus.MustNewConstMetric(c.durationDesc, prometheus.GaugeValue, maxDuration, labels...)
	}

	if err := rows.Err(); err != nil {
		logger.WithError(ctx, err).Error("Error iterating reconciliation metric rows")
	}
}

func RegisterReconciliationCollector(db *sql.DB, stuckThreshold time.Duration) error {
	collector := NewReconciliationCollector(db, stuckThreshold)
	return prometheus.Register(collector)
}
