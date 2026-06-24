package services

import (
	"context"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	dbmocks "github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/mocks"

	"github.com/onsi/gomega/types"
	"github.com/yaacov/tree-search-language/v6/pkg/tsl"

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
	// Note: v6 accepts bare identifiers like "garbage" as valid expressions,
	// so we use a truly unparseable input instead.
	errorTests := []map[string]interface{}{
		{
			"search": "= = =",
			"error":  errors.CodeBadRequest + ": Failed to parse search query: = = =",
		},
	}
	for _, test := range errorTests {
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
	sqlTests := []map[string]interface{}{
		{
			"search": "username in ['ooo.openshift']",
			"sql":    "username IN (?)",
			"values": ConsistOf("ooo.openshift"),
		},
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
	for _, test := range sqlTests {
		var list []api.Cluster
		search := test["search"].(string)
		sqlReal := test["sql"].(string)
		valuesReal := test["values"].(types.GomegaMatcher)
		listCtx, _, serviceErr := genericService.newListContext(
			context.Background(), &ListArguments{Search: search}, &list,
		)
		Expect(serviceErr).ToNot(HaveOccurred())
		// v6 handles deep identifiers natively — no preprocessing needed
		tslTreeWrapper, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		tslTree := tslTreeWrapper.Node
		// Apply field name mapping (includes numeric CAST for spec fields)
		tslTree, serviceErr = db.FieldNameWalk(tslTree, *listCtx.disallowedFields)
		Expect(serviceErr).ToNot(HaveOccurred())
		sqlizer, serviceErr := genericService.treeWalkForSqlizer(listCtx, &tsl.TSLNode{Node: tslTree})
		Expect(serviceErr).ToNot(HaveOccurred())
		sql, values, err := sqlizer.ToSql()
		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(Equal(sqlReal))
		Expect(values).To(valuesReal)
	}

	// Verify JSONB-mapped fields survive treeWalkForRelatedTables without
	// being misclassified as related-resource paths (the "->" substring
	// signals an already-mapped JSONB expression that should be skipped).
	jsonbRelatedTableTests := []map[string]interface{}{
		{
			"search": "labels.environment = 'production'",
			"sql":    "labels->>'environment' = ?",
			"values": ConsistOf("production"),
		},
		{
			"search": "spec.release.version = '2'",
			"sql":    "spec->'release'->>'version' = ?",
			"values": ConsistOf("2"),
		},
		{
			"search": "properties.owner = 'team_a'",
			"sql":    "properties ->> 'owner' = ?",
			"values": ConsistOf("team_a"),
		},
	}
	for _, test := range jsonbRelatedTableTests {
		var list []api.Cluster
		search := test["search"].(string)
		sqlReal := test["sql"].(string)
		valuesReal := test["values"].(types.GomegaMatcher)
		listCtx, _, serviceErr := genericService.newListContext(
			context.Background(), &ListArguments{Search: search}, &list,
		)
		Expect(serviceErr).ToNot(HaveOccurred())
		tslTreeWrapper, err := tsl.ParseTSL(search)
		Expect(err).ToNot(HaveOccurred())
		tslTree := tslTreeWrapper.Node
		tslTree, serviceErr = db.FieldNameWalk(tslTree, *listCtx.disallowedFields)
		Expect(serviceErr).ToNot(HaveOccurred())
		d := g.GetInstanceDao(context.Background(), &api.Cluster{})
		tslTree, serviceErr = genericService.treeWalkForRelatedTables(listCtx, tslTree, &d)
		Expect(serviceErr).ToNot(HaveOccurred(), "JSONB field should not be misclassified as related table: %s", search)
		tslTree, serviceErr = genericService.treeWalkForAddingTableName(listCtx, tslTree, &d)
		Expect(serviceErr).ToNot(HaveOccurred())
		sqlizer, serviceErr := genericService.treeWalkForSqlizer(listCtx, &tsl.TSLNode{Node: tslTree})
		Expect(serviceErr).ToNot(HaveOccurred())
		sql, values, err := sqlizer.ToSql()
		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(Equal(sqlReal))
		Expect(values).To(valuesReal)
	}
}
