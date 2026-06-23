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

	g := dao.NewGenericDao(dbFactory)
	genericService := sqlGenericService{genericDao: g}

	// ill-formatted search should be rejected
	tests := []map[string]interface{}{
		{
			"search": "garbage",
			"error":  errors.CodeBadRequest + ": Failed to parse search query: garbage",
		},
	}
	for _, test := range tests {
		var list []api.Cluster
		search := test["search"].(string)
		errorMsg := test["error"].(string)
		listCtx, model, serviceErr := genericService.newListContext(
			context.Background(), &ListArguments{Search: search}, &list,
		)
		Expect(serviceErr).ToNot(HaveOccurred())
		d := g.GetInstanceDao(context.Background(), model)
		_, serviceErr = genericService.buildSearch(listCtx, &d)
		Expect(serviceErr).To(HaveOccurred())
		Expect(serviceErr.Type).To(Equal(errors.ErrorTypeBadRequest))
		Expect(serviceErr.Error()).To(Equal(errorMsg))
	}

	// tests for sql parsing
	tests = []map[string]interface{}{
		{
			"search": "username in ('ooo.openshift')",
			"sql":    "username IN (?)",
			"values": ConsistOf("ooo.openshift"),
		},
		// Test status.conditions field mapping (use status.conditions.<Type>='<Status>' syntax for condition queries)
		// Test labels.xxx field mapping
		{
			"search": "labels.environment = 'production'",
			"sql":    "labels->>'environment' = ?",
			"values": ConsistOf("production"),
		},
		// Test spec.xxx field mapping (shallow, string value — no CAST)
		{
			"search": "spec.region = 'us-east-1'",
			"sql":    "spec->>'region' = ?",
			"values": ConsistOf("us-east-1"),
		},
		// Test spec.xxx.yyy field mapping (2-level nested, string value — no CAST)
		{
			"search": "spec.release.version = '2'",
			"sql":    "spec->'release'->>'version' = ?",
			"values": ConsistOf("2"),
		},
		// Test spec.xxx.yyy.zzz field mapping (3-level nested, string value — no CAST)
		{
			"search": "spec.release.notes.url = 'https://example.com'",
			"sql":    "spec->'release'->'notes'->>'url' = ?",
			"values": ConsistOf("https://example.com"),
		},
		// Test spec field with unquoted number — CAST applied for correct numeric ordering
		{
			"search": "spec.replicas > 9",
			"sql":    "CAST(spec->>'replicas' AS numeric) > ?",
			"values": ConsistOf(float64(9)),
		},
		// Test nested spec field with unquoted number — CAST applied
		{
			"search": "spec.release.version > 9",
			"sql":    "CAST(spec->'release'->>'version' AS numeric) > ?",
			"values": ConsistOf(float64(9)),
		},
		// Test 3-level nested spec field with unquoted number — CAST applied
		{
			"search": "spec.release.config.replicas > 9",
			"sql":    "CAST(spec->'release'->'config'->>'replicas' AS numeric) > ?",
			"values": ConsistOf(float64(9)),
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
		listCtx, _, serviceErr := genericService.newListContext(
			context.Background(), &ListArguments{Search: search}, &list,
		)
		Expect(serviceErr).ToNot(HaveOccurred())
		// Mirror the production pipeline: pre-process spec deep paths before TSL parsing
		preprocessed := db.PreprocessSpecSubfields(search)
		tslTree, err := tsl.ParseTSL(preprocessed)
		Expect(err).ToNot(HaveOccurred())
		// Apply field name mapping (status.xxx -> status_xxx, labels.xxx -> labels->>'xxx')
		// This must happen before converting to sqlizer
		tslTree, serviceErr = db.FieldNameWalk(tslTree, *listCtx.disallowedFields)
		Expect(serviceErr).ToNot(HaveOccurred())
		// Wrap spec fields in CAST(... AS numeric) when compared against a number
		tslTree = db.WrapSpecNumericCasts(tslTree)
		sqlizer, serviceErr := genericService.treeWalkForSqlizer(listCtx, tslTree)
		Expect(serviceErr).ToNot(HaveOccurred())
		sql, values, err := sqlizer.ToSql()
		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(Equal(sqlReal))
		Expect(values).To(valuesReal)
	}
}
