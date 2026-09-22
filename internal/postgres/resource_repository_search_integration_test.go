package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync/atomic"
	"testing"
)

type searchQueryCounter struct{ count atomic.Int64 }

func (c *searchQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}
func (c *searchQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestResourceRepositorySearchSetHydrationIntegration(t *testing.T) {
	dsn, adminDSN := os.Getenv("GARFEX_TEST_DSN"), os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx, counter := context.Background(), &searchQueryCounter{}
	pool := openSearchTestPool(t, dsn, counter)
	repo := NewResourceRepository(pool)
	t.Cleanup(func() { cleanupResources(ctx, t, pool) })
	t.Run("1/10/50 pages use two queries", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		for i := 0; i < 50; i++ {
			createSearchResource(t, ctx, repo, fmt.Sprintf("v1|search-count-%02d", i), fullSearchValues)
		}
		counter.count.Store(0)
		for _, limit := range []int{1, 10, 50} {
			page, err := repo.(domain.ResourcePageRepository).SearchPage(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Limit: limit})
			if err != nil || len(page.Resources) != limit || page.HasPrevious || page.HasNext != (limit < 50) || counter.count.Swap(0) != 2 {
				t.Fatalf("Search(%d) = %d rows, previous:%v next:%v, %d queries; error %v", limit, len(page.Resources), page.HasPrevious, page.HasNext, counter.count.Load(), err)
			}
		}
		boundary, err := repo.(domain.ResourcePageRepository).SearchPage(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Limit: 10, Offset: 40})
		if err != nil || len(boundary.Resources) != 10 || !boundary.HasPrevious || boundary.HasNext {
			t.Fatalf("exact-boundary page = %+v, %v; want full final page with previous only", boundary, err)
		}
		empty, err := repo.(domain.ResourcePageRepository).SearchPage(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Limit: 10, Offset: 50})
		if err != nil || len(empty.Resources) != 0 || !empty.HasPrevious || empty.HasNext {
			t.Fatalf("empty final page = %+v, %v; want empty previous-only boundary", empty, err)
		}
		counter.count.Store(0)
		if got := mustSearch(t, ctx, repo, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Text: "no-such-resource"}); len(got) != 0 || counter.count.Load() != 1 {
			t.Fatalf("empty Search() = %d rows, %d queries", len(got), counter.count.Load())
		}
	})
	t.Run("type-only values hydrate once", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		resource := createSearchResource(t, ctx, repo, "v1|search-type-only", fullSearchValues)
		assertOption(t, searchGet(t, repo, resource), "gauge", "10 AWG")
		assertSearchOption(t, mustSearch(t, ctx, repo, domain.SearchCriteria{FamilyCode: "CONDUCTORES"}), "gauge", "10 AWG")
	})
	t.Run("family-only fallback, type override, and hidden filter", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		resource := createSearchResource(t, ctx, repo, "v1|search-precedence", fullSearchValues)
		stored := searchGet(t, repo, resource)
		wildcard := insertFamilyGauge(t, ctx, pool, stored.ID, "12 AWG")
		defer func() {
			_, _ = pool.Exec(ctx, `DELETE FROM public.resource_attribute_values WHERE resource_attribute_id = $1`, wildcard)
			_, _ = pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id = $1`, wildcard)
		}()
		assertSearchOption(t, mustSearch(t, ctx, repo, domain.SearchCriteria{FamilyCode: "CONDUCTORES"}), "gauge", "10 AWG")
		for _, tc := range []struct {
			option string
			want   int
		}{{"10 AWG", 1}, {"12 AWG", 0}} {
			got, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Filters: []domain.ResourceAttributeValue{domain.OptionValue("gauge", tc.option)}})
			if err != nil || len(got) != tc.want {
				t.Fatalf("filter %s = %d rows, %v; want %d", tc.option, len(got), err, tc.want)
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM public.resource_attribute_values v USING public.resource_attributes ra, public.attribute_definitions d WHERE v.resource_id = $1 AND v.resource_attribute_id = ra.id AND ra.type_id IS NOT NULL AND d.id = ra.definition_id AND d.code = 'gauge'`, stored.ID); err != nil {
			t.Fatal(err)
		}
		assertSearchOption(t, mustSearch(t, ctx, repo, domain.SearchCriteria{FamilyCode: "CONDUCTORES"}), "gauge", "12 AWG")
	})
	t.Run("sparse Search matches direct Get", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		resource := createSearchResource(t, ctx, repo, "v1|search-sparse", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("insulation", "DESNUDO"), {AttributeCode: "color", Type: domain.ValueTypeControlledOption, Text: notApplicableState}, {AttributeCode: "voltage", Type: domain.ValueTypeControlledOption, Text: notApplicableState}})
		stored := searchGet(t, repo, resource)
		if _, err := pool.Exec(ctx, `DELETE FROM public.resource_attribute_values v USING public.resource_attributes ra, public.attribute_definitions d WHERE v.resource_id = $1 AND v.resource_attribute_id = ra.id AND d.id = ra.definition_id AND d.code IN ('color', 'voltage')`, stored.ID); err != nil {
			t.Fatal(err)
		}
		want, got := searchGet(t, repo, resource), mustSearch(t, ctx, repo, domain.SearchCriteria{FamilyCode: "CONDUCTORES"})
		if len(want.Attributes) != 3 || len(got) != 1 {
			t.Fatalf("sparse parity = Get %d attributes, Search %d rows; want 3 and 1", len(want.Attributes), len(got))
		}
		assertResourceEqual(t, got[0], want)
	})
	t.Run("duplicate winner has no partial page", func(t *testing.T) {
		if adminDSN == "" {
			t.Skip("GARFEX_ADMIN_TEST_DSN not set")
		}
		cleanupResources(ctx, t, pool)
		resource := createSearchResource(t, ctx, repo, "v1|search-duplicate", fullSearchValues)
		admin := openSearchTestPool(t, adminDSN, nil)
		if _, err := admin.Exec(ctx, `DROP INDEX public.resource_attributes_type_definition_key`); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO public.resource_attributes (class_id,family_id,type_id,definition_id,mode,identity_participates,display_order) SELECT class_id,family_id,type_id,definition_id,mode,identity_participates,display_order+100 FROM public.resource_attributes WHERE type_id IS NOT NULL AND definition_id=(SELECT id FROM public.attribute_definitions WHERE code='gauge') LIMIT 1`); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO public.resource_attribute_values (resource_id,family_id,resource_attribute_id,attribute_definition_id,option_code) SELECT r.id,r.family_id,ra.id,ra.definition_id,'10 AWG' FROM public.recursos r JOIN public.resource_attributes ra ON ra.display_order>=100 AND ra.type_id=r.type_id WHERE r.identity_key=$1 AND ra.definition_id=(SELECT id FROM public.attribute_definitions WHERE code='gauge')`, resource.IdentityKey); err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanupResources(ctx, t, pool)
			_, _ = admin.Exec(ctx, `DELETE FROM public.resource_attributes WHERE display_order >= 100`)
			_, _ = admin.Exec(ctx, `CREATE UNIQUE INDEX resource_attributes_type_definition_key ON public.resource_attributes (family_id, type_id, definition_id) WHERE type_id IS NOT NULL`)
		}()
		if got, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey); !errors.Is(err, domain.ErrResourceIntegrity) || got.ID != 0 {
			t.Fatalf("Get() = %#v, %v", got, err)
		}
		if got, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES"}); !errors.Is(err, domain.ErrResourceIntegrity) || got != nil {
			t.Fatalf("Search() = %#v, %v", got, err)
		}
		for _, option := range []string{"10 AWG", "14 AWG"} {
			if got, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Filters: []domain.ResourceAttributeValue{domain.OptionValue("gauge", option)}}); !errors.Is(err, domain.ErrResourceIntegrity) || got != nil {
				t.Fatalf("filtered Search(%s) = %#v, %v", option, got, err)
			}
		}
	})
}
func openSearchTestPool(t *testing.T, dsn string, tracer pgx.QueryTracer) *pgxpool.Pool {
	config, _ := pgxpool.ParseConfig(dsn)
	config.ConnConfig.Tracer = tracer
	pool, _ := pgxpool.NewWithConfig(context.Background(), config)
	t.Cleanup(pool.Close)
	return pool
}
func createSearchResource(t *testing.T, ctx context.Context, repo domain.ResourceRepository, identity string, values []domain.ResourceAttributeValue) domain.Resource {
	resource := mustCreateResource(t, domain.SeedResourceCatalog(), conductoresScope, "M", fullSearchValues)
	resource.Attributes = values
	resource.IdentityKey = identity
	if err := repo.Create(ctx, resource); err != nil {
		t.Fatal(err)
	}
	return resource
}
func mustSearch(t *testing.T, ctx context.Context, repo domain.ResourceRepository, criteria domain.SearchCriteria) []domain.Resource {
	got, err := repo.Search(ctx, criteria)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func searchGet(t *testing.T, repo domain.ResourceRepository, resource domain.Resource) domain.Resource {
	got, err := repo.Get(context.Background(), resource.ClassCode, resource.IdentityKey)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func insertFamilyGauge(t *testing.T, ctx context.Context, pool *pgxpool.Pool, resourceID int64, option string) int64 {
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.resource_attributes (class_id, family_id, definition_id, mode, identity_participates, display_order) SELECT f.class_id, f.id, d.id, 'OPTIONAL', TRUE, 99 FROM public.resource_families f JOIN public.attribute_definitions d ON d.code = 'gauge' WHERE f.code = 'CONDUCTORES' RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_attribute_values (resource_id, family_id, resource_attribute_id, attribute_definition_id, option_code) SELECT r.id, r.family_id, $2, d.id, $3 FROM public.recursos r JOIN public.attribute_definitions d ON d.code = 'gauge' WHERE r.id = $1`, resourceID, id, option); err != nil {
		t.Fatal(err)
	}
	return id
}
func assertSearchOption(t *testing.T, resources []domain.Resource, code, option string) {
	if len(resources) != 1 {
		t.Fatalf("Search() = %d rows, want one", len(resources))
	}
	assertOption(t, resources[0], code, option)
}
func assertOption(t *testing.T, resource domain.Resource, code, option string) {
	var got string
	for _, value := range resource.Attributes {
		if value.AttributeCode == code {
			if got != "" {
				t.Fatalf("attribute %q has duplicate values", code)
			}
			got = value.OptionCode
		}
	}
	if got != option {
		t.Fatalf("attribute %q = %q, want %q", code, got, option)
	}
}

var fullSearchValues = []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")}
