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

package db_metrics

import (
	"database/sql"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	testQueryDurationMetric    = "hyperfleet_db_query_duration_seconds"
	testErrorsMetric           = "hyperfleet_db_errors_total"
	testConnectionsOpenMetric  = "hyperfleet_db_connections_open"
	testConnectionsInUseMetric = "hyperfleet_db_connections_in_use"
)

func TestMetricsSubsystem(t *testing.T) {
	RegisterTestingT(t)
	Expect(metricsSubsystem).To(Equal("hyperfleet_db"))
}

func TestQueryDurationMetricIsRegistered(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	// Observe a value so the metric appears in Gather()
	QueryDurationMetric.With(prometheus.Labels{
		labelOperation: "query",
		labelTable:     "test_table",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Observe(0.01)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testQueryDurationMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))
			break
		}
	}
	Expect(found).To(BeTrue(), testQueryDurationMetric+" metric should be registered")
}

func TestQueryDurationHistogramBuckets(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	expectedBuckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

	// Observe a value to generate metric data
	QueryDurationMetric.With(prometheus.Labels{
		labelOperation: "query",
		labelTable:     "test_table",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Observe(0.05)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testQueryDurationMetric {
			found = true
			for _, metric := range mf.GetMetric() {
				histogram := metric.GetHistogram()
				buckets := histogram.GetBucket()
				Expect(buckets).To(HaveLen(len(expectedBuckets)))
				for i, b := range buckets {
					Expect(b.GetUpperBound()).To(Equal(expectedBuckets[i]))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), testQueryDurationMetric+" metric should be registered")
}

func TestQueryDurationMetricLabels(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	QueryDurationMetric.With(prometheus.Labels{
		labelOperation: "create",
		labelTable:     "clusters",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Observe(0.01)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testQueryDurationMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["operation"] == "create" && labels["table"] == "clusters" {
					found = true
					Expect(labels["component"]).To(Equal(componentValue))
					Expect(labels["version"]).To(Equal(api.Version))
					Expect(metric.GetHistogram().GetSampleCount()).To(BeEquivalentTo(1))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), "query duration metric with expected labels should exist")
}

func TestErrorsMetricIsRegistered(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	// Increment a value so the metric appears in Gather()
	ErrorsMetric.With(prometheus.Labels{
		labelOperation: "query",
		labelErrorType: "other",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Inc()

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testErrorsMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
			break
		}
	}
	Expect(found).To(BeTrue(), testErrorsMetric+" metric should be registered")
}

func TestErrorsMetricLabels(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	ErrorsMetric.With(prometheus.Labels{
		labelOperation: "create",
		labelErrorType: "constraint_violation",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Inc()

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testErrorsMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["operation"] == "create" && labels["error_type"] == "constraint_violation" {
					found = true
					Expect(labels["component"]).To(Equal(componentValue))
					Expect(labels["version"]).To(Equal(api.Version))
					Expect(metric.GetCounter().GetValue()).To(Equal(1.0))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), "errors metric with expected labels should exist")
}

func TestResetMetrics(t *testing.T) {
	RegisterTestingT(t)

	// Add some data
	QueryDurationMetric.With(prometheus.Labels{
		labelOperation: "query",
		labelTable:     "clusters",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Observe(0.1)

	ErrorsMetric.With(prometheus.Labels{
		labelOperation: "query",
		labelErrorType: "other",
		labelComponent: componentValue,
		labelVersion:   api.Version,
	}).Inc()

	// Reset
	ResetMetrics()

	// Verify metrics are reset - gather should not have any data points for our specific labels
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testQueryDurationMetric {
			Expect(mf.GetMetric()).To(BeEmpty(), "query duration metrics should be empty after reset")
		}
		if mf.GetName() == testErrorsMetric {
			Expect(mf.GetMetric()).To(BeEmpty(), "errors metrics should be empty after reset")
		}
	}
}

func TestClassifyError_ConstraintViolation(t *testing.T) {
	RegisterTestingT(t)

	testCases := []string{
		"ERROR: duplicate key value violates unique constraint",
		"pq: duplicate key value violates unique constraint \"clusters_pkey\"",
		"ERROR: violates foreign key constraint",
		"UNIQUE constraint failed",
	}

	for _, tc := range testCases {
		Expect(classifyError(errors.New(tc))).To(Equal("constraint_violation"),
			"expected constraint_violation for: %s", tc)
	}
}

func TestClassifyError_ConnectionError(t *testing.T) {
	RegisterTestingT(t)

	testCases := []string{
		"connection refused",
		"connection reset by peer",
		"broken pipe",
	}

	for _, tc := range testCases {
		Expect(classifyError(errors.New(tc))).To(Equal("connection_error"),
			"expected connection_error for: %s", tc)
	}
}

func TestClassifyError_Timeout(t *testing.T) {
	RegisterTestingT(t)

	testCases := []string{
		"context deadline exceeded",
		"timeout waiting for response",
		"canceling statement due to statement timeout",
	}

	for _, tc := range testCases {
		Expect(classifyError(errors.New(tc))).To(Equal("timeout"),
			"expected timeout for: %s", tc)
	}
}

func TestClassifyError_Other(t *testing.T) {
	RegisterTestingT(t)

	Expect(classifyError(errors.New("some unknown error"))).To(Equal("other"))
}

func TestClassifyError_Nil(t *testing.T) {
	RegisterTestingT(t)

	Expect(classifyError(nil)).To(Equal(""))
}

func TestPoolCollector_Describe(t *testing.T) {
	RegisterTestingT(t)

	collector := NewPoolCollector(&sql.DB{})
	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}

	Expect(descs).To(HaveLen(2))
	Expect(descs).To(ContainElement(ConnectionsOpenDesc))
	Expect(descs).To(ContainElement(ConnectionsInUseDesc))
}

func TestPoolCollector_Collect(t *testing.T) {
	RegisterTestingT(t)

	// A zero-value sql.DB yields zero-valued sql.DBStats, which is valid for this test
	db := &sql.DB{}
	collector := NewPoolCollector(db)

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	Expect(metrics).To(HaveLen(2))

	// Verify the metric values are zero for a zero-value sql.DB
	for _, m := range metrics {
		var dtoMetric dto.Metric
		err := m.Write(&dtoMetric)
		Expect(err).To(BeNil())
		Expect(dtoMetric.GetGauge().GetValue()).To(Equal(0.0))
	}
}

func TestConnectionsOpenDescriptor(t *testing.T) {
	RegisterTestingT(t)

	Expect(ConnectionsOpenDesc.String()).To(ContainSubstring("hyperfleet_db_connections_open"))
	Expect(ConnectionsOpenDesc.String()).To(ContainSubstring("component"))
	Expect(ConnectionsOpenDesc.String()).To(ContainSubstring("version"))
}

func TestConnectionsInUseDescriptor(t *testing.T) {
	RegisterTestingT(t)

	Expect(ConnectionsInUseDesc.String()).To(ContainSubstring("hyperfleet_db_connections_in_use"))
	Expect(ConnectionsInUseDesc.String()).To(ContainSubstring("component"))
	Expect(ConnectionsInUseDesc.String()).To(ContainSubstring("version"))
}

// labelsToMap converts metric labels to a map for easier testing.
func labelsToMap(metric *dto.Metric) map[string]string {
	labels := make(map[string]string)
	for _, lp := range metric.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	return labels
}
