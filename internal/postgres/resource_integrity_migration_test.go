package postgres

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationResetRestoresAfterBodyFailure(t *testing.T) {
	if os.Getenv("GARFEX_ADMIN_TEST_DSN") == "" {
		t.Skip("GARFEX_ADMIN_TEST_DSN not set")
	}
	if os.Getenv("GARFEX_MIGRATION_FAILURE_CHILD") == "1" {
		pool := openUnitTestPool(t, os.Getenv("GARFEX_ADMIN_TEST_DSN"))
		window := beginMigrationTestWindow(t, pool)
		window.TrackResource(insertIntegrityResource(t, pool, "TEST_RESOURCE_INTEGRITY_FAILURE"))
		t.Fatal("injected migration test body failure")
	}

	pool := openUnitTestPool(t, os.Getenv("GARFEX_ADMIN_TEST_DSN"))
	ctx := context.Background()
	seed := createCanonicalMigrationSeed(t, pool)
	before, err := captureMigrationTestState(ctx, pool)
	migrationFatal(t, "capture initial state", err)
	if seed.ID != 0 {
		t.Cleanup(func() { deleteIntegrityResources(t, pool, seed.ID) })
	}
	if before.resourceCount == 0 || before.mapExists && before.mapCount == 0 {
		t.Fatalf("non-empty restoration seed missing: resources=%d map=%d", before.resourceCount, before.mapCount)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMigrationResetRestoresAfterBodyFailure$")
	cmd.Env = append(os.Environ(), "GARFEX_MIGRATION_FAILURE_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("injected child unexpectedly passed")
	}
	if !strings.Contains(string(output), "injected migration test body failure") {
		t.Fatalf("child failure output missing injected marker: %s", output)
	}
	after, err := captureMigrationTestState(ctx, pool)
	migrationFatal(t, "capture restored state", err)
	for _, err := range compareMigrationTestState(before, after) {
		t.Errorf("body-failure restoration: %v", err)
	}
	if seed.ID != 0 {
		restored, err := NewResourceRepository(pool).Get(ctx, seed.ClassCode, seed.IdentityKey)
		if err != nil {
			t.Fatalf("load restored canonical seed: %v", err)
		}
		if !reflect.DeepEqual(restored, seed) {
			t.Fatalf("restored canonical seed = %#v, want %#v", restored, seed)
		}
		deleteIntegrityResources(t, pool, seed.ID)
	}
}

func createCanonicalMigrationSeed(t *testing.T, pool *pgxpool.Pool) domain.Resource {
	ctx := context.Background()
	state, err := captureMigrationTestState(ctx, pool)
	migrationFatal(t, "inspect migration seed state", err)
	if !state.constraintExists {
		return domain.Resource{}
	}
	catalog, err := LoadResourceCatalog(ctx, pool)
	migrationFatal(t, "load persisted catalog for migration seed", err)
	seed := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "ALUMINIO"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
	migrationFatal(t, "create canonical migration seed", NewResourceRepository(pool).Create(ctx, seed))
	seed, err = NewResourceRepository(pool).Get(ctx, seed.ClassCode, seed.IdentityKey)
	migrationFatal(t, "read canonical migration seed", err)
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_integrity_identity_map (resource_id, class_id, legacy_identity_key, v1_identity_key) SELECT $1, id, $2, $3 FROM public.resource_classes WHERE code=$4`, seed.ID, "TEST_MIGRATION_CANONICAL_LEGACY", seed.IdentityKey, seed.ClassCode); err != nil {
		t.Fatalf("map canonical migration seed: %v", err)
	}
	return seed
}

func TestResourceIntegrityMigrationAuditRelation(t *testing.T) {
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000005_resource_integrity.up.sql"))
	if err != nil {
		t.Fatalf("read resource integrity migration: %v", err)
	}
	migration := string(sql)
	relation := `resource_integrity_identity_audit`

	t.Run("does not require temporary table privilege", func(t *testing.T) {
		if regexp.MustCompile(`(?i)\bTEMP(?:ORARY)?\b`).MatchString(migration) {
			t.Fatal("migration requests TEMP or TEMPORARY privilege")
		}
	})

	t.Run("fully qualifies every audit relation reference", func(t *testing.T) {
		references := regexp.MustCompile(`(?i)([a-z_][a-z0-9_]*\s*\.\s*)?`+relation).FindAllStringSubmatch(migration, -1)
		whitespace := regexp.MustCompile(`\s+`)
		if len(references) == 0 {
			t.Fatal("migration does not reference the audit relation")
		}
		for _, reference := range references {
			qualifier := whitespace.ReplaceAllString(reference[1], "")
			if !strings.EqualFold(qualifier, "public.") {
				t.Errorf("audit relation reference %q is not qualified with public", reference[0])
			}
		}
	})

	t.Run("explicitly drops audit relation", func(t *testing.T) {
		drop := regexp.MustCompile(`(?i)\bDROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?public\s*\.\s*` + relation + `\b`)
		if !drop.MatchString(migration) {
			t.Fatal("migration does not explicitly drop the public audit relation")
		}
	})
}

// revisionMigrationTables mirrors migrations/000008_resource_revisions.*.sql
// exactly: public.recursos plus the 11 catalog-definition parent tables.
// resource_attribute_rules is deliberately excluded — it gets no independent
// revision column.
var revisionMigrationTables = []string{
	"recursos", "resource_classes", "resource_families", "resource_types",
	"attribute_definitions", "resource_option_sets", "attribute_options",
	"attribute_option_relations", "unit_definitions", "resource_unit_policies",
	"resource_attributes", "resource_type_presentation_fields",
}

// TestMigrationRevisionColumnsIntegration proves migration 8 is additive over
// an already-migrated (through 000007) database: applying it backfills every
// existing row's revision to 1 on recursos and the 11 catalog-definition
// parent tables, leaves resource_attribute_rules with no independent
// revision, is safely reversible, and never disturbs identity_key
// byte-for-byte — the single most important invariant (design "Resource
// revisions and identity"). Serialized with every other schema-destructive
// integration test via the shared migrationTestAdvisoryLock.
func TestMigrationRevisionColumnsIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_ADMIN_TEST_DSN not set")
	}
	ctx := context.Background()
	pool := openUnitTestPool(t, dsn)

	lock, err := pool.Acquire(ctx)
	migrationFatal(t, "acquire migration advisory-lock connection", err)
	t.Cleanup(lock.Release)
	_, err = lock.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationTestAdvisoryLock)
	migrationFatal(t, "acquire migration advisory lock", err)
	t.Cleanup(func() {
		_, _ = lock.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationTestAdvisoryLock)
	})

	hasRevisionColumn := func(table string) bool {
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='revision')`, table).Scan(&exists)
		migrationFatal(t, "inspect "+table+".revision", err)
		return exists
	}
	identityHash := func() string {
		var hash string
		err := pool.QueryRow(ctx, `SELECT COALESCE(md5(string_agg(id::text || ':' || identity_key, '|' ORDER BY id)), md5('')) FROM public.recursos`).Scan(&hash)
		migrationFatal(t, "hash recursos identity_key", err)
		return hash
	}

	startedWithRevision := hasRevisionColumn("recursos")
	if startedWithRevision {
		migrationFatal(t, "down migration 8 to reach a clean starting point", applySchemaMigration(pool, "000008_resource_revisions.down.sql"))
	}
	t.Cleanup(func() {
		if startedWithRevision && !hasRevisionColumn("recursos") {
			migrationFatal(t, "restore migration 8", applySchemaMigration(pool, "000008_resource_revisions.up.sql"))
		}
	})
	for _, table := range revisionMigrationTables {
		if hasRevisionColumn(table) {
			t.Fatalf("%s already has a revision column before migration 8 up", table)
		}
	}
	if hasRevisionColumn("resource_attribute_rules") {
		t.Fatal("resource_attribute_rules unexpectedly has a revision column before migration 8")
	}

	seed := mustCreateResource(t, domain.SeedResourceCatalog(), conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
	migrationFatal(t, "create canonical revision-migration seed", NewResourceRepository(pool).Create(ctx, seed))
	var seededID int64
	migrationFatal(t, "read canonical revision-migration seed id", pool.QueryRow(ctx, `SELECT id FROM public.recursos WHERE identity_key=$1`, seed.IdentityKey).Scan(&seededID))
	t.Cleanup(func() { deleteIntegrityResources(t, pool, seededID) })
	beforeUp := identityHash()

	migrationFatal(t, "apply migration 8 up", applySchemaMigration(pool, "000008_resource_revisions.up.sql"))
	if got := identityHash(); got != beforeUp {
		t.Fatalf("identity_key hash changed after migration 8 up: before=%s after=%s", beforeUp, got)
	}
	for _, table := range revisionMigrationTables {
		if !hasRevisionColumn(table) {
			t.Fatalf("%s missing revision column after migration 8 up", table)
		}
		var count, badRevisions int64
		err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*), count(*) FILTER (WHERE revision <> 1) FROM public.%s`, table)).Scan(&count, &badRevisions)
		migrationFatal(t, "read "+table+" revision backfill", err)
		if count > 0 && badRevisions != 0 {
			t.Fatalf("%s revision backfill has %d row(s) != 1 (of %d)", table, badRevisions, count)
		}
	}
	if hasRevisionColumn("resource_attribute_rules") {
		t.Fatal("resource_attribute_rules must not gain an independent revision column")
	}
	var seededRevision int64
	migrationFatal(t, "read seeded recursos revision", pool.QueryRow(ctx, `SELECT revision FROM public.recursos WHERE id=$1`, seededID).Scan(&seededRevision))
	if seededRevision != 1 {
		t.Fatalf("pre-existing recursos row backfilled revision = %d, want 1", seededRevision)
	}

	migrationFatal(t, "apply migration 8 down", applySchemaMigration(pool, "000008_resource_revisions.down.sql"))
	if got := identityHash(); got != beforeUp {
		t.Fatalf("identity_key hash changed after migration 8 down: before=%s after=%s", beforeUp, got)
	}
	for _, table := range revisionMigrationTables {
		if hasRevisionColumn(table) {
			t.Fatalf("%s still has revision column after migration 8 down", table)
		}
	}

	migrationFatal(t, "reapply migration 8 up", applySchemaMigration(pool, "000008_resource_revisions.up.sql"))
}
