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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	testPendingDeletionTotalMetric    = "hyperfleet_api_resource_pending_deletion_total"
	testPendingDeletionDurationMetric = "hyperfleet_api_resource_pending_deletion_duration_seconds"
	testPendingDeletionStuckMetric    = "hyperfleet_api_resource_pending_deletion_stuck"
	testResourceCluster               = "cluster"
	testResourceNodepool              = "nodepool"
)

func TestMetricsSubsystem(t *testing.T) {
	RegisterTestingT(t)
	Expect(metricsSubsystem).To(Equal("hyperfleet_api"))
}

func TestPendingDeletionTotalIsRegistered(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	RecordPendingDeletion(testResourceCluster)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionTotalMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
			break
		}
	}
	Expect(found).To(BeTrue(), testPendingDeletionTotalMetric+" metric should be registered")
}

func TestRecordPendingDeletion_IncrementsCounter(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	RecordPendingDeletion(testResourceCluster)
	RecordPendingDeletion(testResourceCluster)
	RecordPendingDeletion(testResourceNodepool)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var clusterCount, nodepoolCount float64
	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["resource_type"] == testResourceCluster {
					clusterCount = metric.GetCounter().GetValue()
				}
				if labels["resource_type"] == testResourceNodepool {
					nodepoolCount = metric.GetCounter().GetValue()
				}
			}
			break
		}
	}
	Expect(clusterCount).To(Equal(2.0))
	Expect(nodepoolCount).To(Equal(1.0))
}

func TestRecordPendingDeletion_Labels(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	RecordPendingDeletion(testResourceCluster)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionTotalMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["resource_type"] == testResourceCluster {
					found = true
					Expect(labels["component"]).To(Equal("api"))
					Expect(labels["version"]).To(Equal(api.Version))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), "pending deletion total metric with expected labels should exist")
}

func TestPendingDeletionDurationIsRegistered(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	ObservePendingDeletionDuration(testResourceCluster, time.Now().Add(-5*time.Second))

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionDurationMetric {
			found = true
			Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))
			break
		}
	}
	Expect(found).To(BeTrue(), testPendingDeletionDurationMetric+" metric should be registered")
}

func TestObservePendingDeletionDuration_RecordsValue(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	deletedAt := time.Now().Add(-10 * time.Second)
	ObservePendingDeletionDuration(testResourceCluster, deletedAt)

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionDurationMetric {
			for _, metric := range mf.GetMetric() {
				labels := labelsToMap(metric)
				if labels["resource_type"] == testResourceCluster {
					found = true
					Expect(metric.GetHistogram().GetSampleCount()).To(BeEquivalentTo(1))
					Expect(metric.GetHistogram().GetSampleSum()).To(BeNumerically(">=", 10.0))
				}
			}
			break
		}
	}
	Expect(found).To(BeTrue(), "pending deletion duration metric with cluster label should exist")
}

func TestPendingDeletionDurationBuckets(t *testing.T) {
	RegisterTestingT(t)
	ResetMetrics()

	expectedBuckets := []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600}

	ObservePendingDeletionDuration(testResourceCluster, time.Now().Add(-1*time.Second))

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionDurationMetric {
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
	Expect(found).To(BeTrue(), testPendingDeletionDurationMetric+" metric should be registered")
}

func TestResetMetrics_ClearsAllDeletionMetrics(t *testing.T) {
	RegisterTestingT(t)

	RecordPendingDeletion(testResourceCluster)
	ObservePendingDeletionDuration(testResourceCluster, time.Now().Add(-5*time.Second))

	ResetMetrics()

	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	Expect(err).To(BeNil())

	for _, mf := range metricFamilies {
		if mf.GetName() == testPendingDeletionTotalMetric {
			Expect(mf.GetMetric()).To(BeEmpty(), "pending deletion total should be empty after reset")
		}
		if mf.GetName() == testPendingDeletionDurationMetric {
			Expect(mf.GetMetric()).To(BeEmpty(), "pending deletion duration should be empty after reset")
		}
	}
}

func TestPendingDeletionCollector_Describe(t *testing.T) {
	RegisterTestingT(t)

	collector := NewPendingDeletionCollector(nil, 30*time.Minute)
	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}

	Expect(descs).To(HaveLen(1))
	Expect(descs[0].String()).To(ContainSubstring("resource_pending_deletion_stuck"))
}

func TestPendingDeletionCollector_CollectWithNilDB(t *testing.T) {
	RegisterTestingT(t)

	collector := NewPendingDeletionCollector(nil, 30*time.Minute)
	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	var collectedMetrics []prometheus.Metric
	for m := range ch {
		collectedMetrics = append(collectedMetrics, m)
	}

	Expect(collectedMetrics).To(BeEmpty())
}

func TestStuckDescriptor(t *testing.T) {
	RegisterTestingT(t)

	collector := NewPendingDeletionCollector(nil, 30*time.Minute)
	descStr := collector.stuckDesc.String()

	Expect(descStr).To(ContainSubstring("hyperfleet_api_resource_pending_deletion_stuck"))
	Expect(descStr).To(ContainSubstring("resource_type"))
	Expect(descStr).To(ContainSubstring("component"))
	Expect(descStr).To(ContainSubstring("version"))
}

func labelsToMap(metric *dto.Metric) map[string]string {
	labels := make(map[string]string)
	for _, lp := range metric.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	return labels
}
