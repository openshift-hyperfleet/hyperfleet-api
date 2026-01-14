package services

import (
	"context"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	dbmocks "github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/mocks"

	"github.com/onsi/gomega/types"
	"github.com/yaacov/tree-search-language/pkg/tsl"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"

	. "github.com/onsi/gomega"
)

func TestSQLTranslation(t *testing.T) {
	RegisterTestingT(t)
	var dbFactory db.SessionFactory = dbmocks.NewMockSessionFactory()
	defer dbFactory.Close() //nolint:errcheck

	g := dao.NewGenericDao(&dbFactory)
	genericService := sqlGenericService{genericDao: g}

	// ill-formatted search or disallowed fields should be rejected
	tests := []map[string]interface{}{
		{
			"search": "garbage",
			"error":  "HYPERFLEET-VAL-005: Failed to parse search query: garbage",
		},
		{
			"search": "spec = '{}'",
			"error":  "HYPERFLEET-VAL-005: spec is not a valid field name",
		},
	}
	for _, test := range tests {
		var list []api.Cluster
		search := test["search"].(string)
		errorMsg := test["error"].(string)
		listCtx, model, serviceErr := genericService.newListContext(context.Background(), "", &ListArguments{Search: search}, &list)
		Expect(serviceErr).ToNot(HaveOccurred())
		d := g.GetInstanceDao(context.Background(), model)
		_, serviceErr = genericService.buildSearch(listCtx, &d)
		Expect(serviceErr).To(HaveOccurred())
		Expect(serviceErr.Code).To(Equal(errors.ErrorBadRequest))
		Expect(serviceErr.Error()).To(Equal(errorMsg))
	}

	// tests for sql parsing
	tests = []map[string]interface{}{
		{
			"search": "username in ('ooo.openshift')",
			"sql":    "username IN (?)",
			"values": ConsistOf("ooo.openshift"),
		},
		// Test status.xxx field mapping
		{
			"search": "status.phase = 'NotReady'",
			"sql":    "status_phase = ?",
			"values": ConsistOf("NotReady"),
		},
		{
			"search": "status.last_updated_time < '2025-01-01T00:00:00Z'",
			"sql":    "status_last_updated_time < ?",
			"values": ConsistOf("2025-01-01T00:00:00Z"),
		},
		// Test labels.xxx field mapping
		{
			"search": "labels.environment = 'production'",
			"sql":    "labels->>'environment' = ?",
			"values": ConsistOf("production"),
		},
		// Test ID query (should be allowed)
		{
			"search": "id = 'cls-123'",
			"sql":    "id = ?",
			"values": ConsistOf("cls-123"),
		},
	}
	for _, test := range tests {
		var list []api.Cluster
		search := test["search"].(string)
		sqlReal := test["sql"].(string)
		valuesReal := test["values"].(types.GomegaMatcher)
		listCtx, _, serviceErr := genericService.newListContext(context.Background(), "", &ListArguments{Search: search}, &list)
		Expect(serviceErr).ToNot(HaveOccurred())
		tslTree, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		// Apply field name mapping (status.xxx -> status_xxx, labels.xxx -> labels->>'xxx')
		// This must happen before converting to sqlizer
		tslTree, serviceErr = db.FieldNameWalk(tslTree, *listCtx.disallowedFields)
		Expect(serviceErr).ToNot(HaveOccurred())
		sqlizer, serviceErr := genericService.treeWalkForSqlizer(listCtx, tslTree)
		Expect(serviceErr).ToNot(HaveOccurred())
		sql, values, err := sqlizer.ToSql()
		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(Equal(sqlReal))
		Expect(values).To(valuesReal)
	}
}
