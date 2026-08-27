package tenant

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

// dummyDB returns a *gorm.DB that builds SQL without a real connection.
func dummyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy db: %v", err)
	}
	return db
}

// statementFor runs a dry-run Find and returns the built statement.
func statementFor(t *testing.T, scoped *gorm.DB) *gorm.Statement {
	t.Helper()
	type resource struct {
		ID string
	}
	dryRun := scoped.Session(&gorm.Session{DryRun: true}).Find(&[]resource{})
	return dryRun.Statement
}

// scopeCases returns the shared test cases for ScopeDB and ScopeClause.
func scopeCases() []struct {
	name          string
	ctx           context.Context
	wantClause    string
	wantContained string
	wantScoped    bool
} {
	return []struct {
		name          string
		ctx           context.Context
		wantClause    string
		wantContained string
		wantScoped    bool
	}{
		{
			name:          "tenant context applies containment filter",
			ctx:           WithTenant(context.Background(), &ResolvedTenant{Dimensions: map[string]string{"org": "acme"}}),
			wantClause:    "tenancy @> ?",
			wantContained: `{"org":"acme"}`,
			wantScoped:    true,
		},
		{
			name: "multi-dimension tenant produces deterministic json",
			ctx: WithTenant(context.Background(), &ResolvedTenant{
				Dimensions: map[string]string{"org": "acme", "project": "widgets"},
			}),
			wantClause:    "tenancy @> ?",
			wantContained: `{"org":"acme","project":"widgets"}`,
			wantScoped:    true,
		},
		{
			name: "system caller is unscoped",
			ctx:  WithTenant(context.Background(), &ResolvedTenant{System: true, Dimensions: map[string]string{"org": "acme"}}),
		},
		{
			name:       "empty dimensions fails closed - matches nothing",
			ctx:        WithTenant(context.Background(), &ResolvedTenant{Dimensions: map[string]string{}}),
			wantClause: "1 = 0",
			wantScoped: true,
		},
		{
			name: "no tenant in context is unscoped",
			ctx:  context.Background(),
		},
	}
}

func TestScopeDB(t *testing.T) {
	for _, tt := range scopeCases() {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			stmt := statementFor(t, ScopeDB(dummyDB(t), tt.ctx))

			if tt.wantScoped {
				Expect(stmt.SQL.String()).To(ContainSubstring(tt.wantClause))
				if tt.wantContained != "" {
					Expect(stmt.Vars).To(HaveLen(1))
					Expect(stmt.Vars[0]).To(MatchJSON(tt.wantContained))
				} else {
					Expect(stmt.Vars).To(BeEmpty(), "deny-all clause must not carry an unconsumed bound argument")
				}
			} else {
				Expect(stmt.SQL.String()).ToNot(ContainSubstring("WHERE"))
			}
		})
	}
}

// TestScopeClause exercises ScopeClause directly for raw-SQL DAO callers.
func TestScopeClause(t *testing.T) {
	for _, tt := range scopeCases() {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			clause, args := ScopeClause(tt.ctx)

			if tt.wantScoped {
				Expect(clause).To(Equal(tt.wantClause))
				if tt.wantContained != "" {
					Expect(args).To(HaveLen(1))
					Expect(args[0]).To(MatchJSON(tt.wantContained))
				} else {
					Expect(args).To(BeEmpty(), "deny-all clause must return no args for callers to spread")
				}
			} else {
				Expect(clause).To(BeEmpty())
				Expect(args).To(BeEmpty())
			}
		})
	}
}
