package garfex_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	garfex "github.com/GARFEX33/garfex-backend"
	"github.com/GARFEX33/garfex-backend/resourcecore"
	"github.com/GARFEX33/garfex-backend/suppliercore"
	"github.com/jackc/pgx/v5/pgxpool"
)

const coreIntegrationAdvisoryLock int64 = 0x4741524645583351

func TestOpenIntegrationExposesSixLiveHandles(t *testing.T) {
	runtimeDSN := os.Getenv("GARFEX_TEST_DSN")
	adminDSN := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if runtimeDSN == "" || adminDSN == "" {
		t.Skip("GARFEX_TEST_DSN and GARFEX_ADMIN_TEST_DSN must target an isolated test database")
	}

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("open admin cleanup pool: %v", err)
	}
	defer adminPool.Close()
	lock, err := adminPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire integration advisory lock: %v", err)
	}
	defer lock.Release()
	if _, err := lock.Exec(ctx, `SELECT pg_advisory_lock($1)`, coreIntegrationAdvisoryLock); err != nil {
		t.Fatalf("acquire integration advisory lock: %v", err)
	}
	defer func() { _, _ = lock.Exec(ctx, `SELECT pg_advisory_unlock($1)`, coreIntegrationAdvisoryLock) }()

	app, err := garfex.Open(ctx, garfex.Config{DSN: runtimeDSN})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer app.Close()
	if app.ResourceReader == nil || app.ResourceWriter == nil || app.SupplierReader == nil || app.SupplierWriter == nil || app.PurchaseReader == nil || app.PurchaseWriter == nil {
		t.Fatal("Open() did not expose all six public handles")
	}

	classes, err := app.ResourceReader.ActiveClasses(ctx)
	if err != nil {
		t.Fatalf("ResourceReader.ActiveClasses() error = %v", err)
	}
	if len(classes) == 0 {
		t.Fatal("ResourceReader.ActiveClasses() returned no seeded classes")
	}
	if _, err := app.SupplierReader.SearchSuppliers(ctx, suppliercore.SupplierQuery{Limit: 1}); err != nil {
		t.Fatalf("SupplierReader.SearchSuppliers() error = %v", err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	classCode := "TEST_CORE_" + suffix
	classValues := func() map[string]resourcecore.Value {
		return map[string]resourcecore.Value{
			"code":   {Kind: resourcecore.ValueCode, Text: classCode},
			"name":   {Kind: resourcecore.ValueText, Text: "Core integration class"},
			"plural": {Kind: resourcecore.ValueText, Text: "Core integration classes"},
			"slug":   {Kind: resourcecore.ValueText, Text: "core-integration-" + suffix},
		}
	}
	assertEmptyStringList := func(t *testing.T, rec resourcecore.CatalogRecord, field string) {
		t.Helper()
		value, ok := rec.Values[field]
		if !ok || value.Kind != resourcecore.ValueStringList || value.Strings == nil || len(value.Strings) != 0 {
			t.Fatalf("%s = %+v, want a non-nil empty STRING_LIST", field, value)
		}
	}
	getClass := func(t *testing.T, id int64) resourcecore.CatalogRecord {
		t.Helper()
		rec, err := app.ResourceReader.GetCatalog(ctx, resourcecore.CatalogKey{Kind: resourcecore.KindClass, ID: id})
		if err != nil {
			t.Fatalf("ResourceReader.GetCatalog() error = %v", err)
		}
		return rec
	}

	class, err := app.ResourceWriter.CreateCatalog(ctx, resourcecore.CatalogWriteRequest{
		Actor:  "core-integration-test",
		Kind:   resourcecore.KindClass,
		Active: true,
		Values: classValues(),
	})
	if err != nil {
		t.Fatalf("ResourceWriter.CreateCatalog() error = %v", err)
	}
	defer func() {
		deactivatedClass, err := app.ResourceWriter.DeactivateCatalog(ctx, resourcecore.CatalogLifecycleRequest{
			Actor:            "core-integration-test",
			Kind:             resourcecore.KindClass,
			ID:               class.ID,
			ExpectedRevision: class.Revision,
		})
		if err != nil {
			t.Errorf("deactivate resource class for cleanup: %v", err)
			return
		}
		if err := app.ResourceWriter.HardDeleteCatalog(ctx, resourcecore.CatalogLifecycleRequest{
			Actor:            "core-integration-test",
			Kind:             resourcecore.KindClass,
			ID:               class.ID,
			ExpectedRevision: deactivatedClass.Revision,
		}); err != nil {
			t.Errorf("cleanup resource class: %v", err)
		}
	}()

	storedClass := getClass(t, class.ID)
	assertEmptyStringList(t, storedClass, "aliases")
	assertEmptyStringList(t, storedClass, "keywords")

	nonemptyValues := classValues()
	nonemptyValues["aliases"] = resourcecore.Value{Kind: resourcecore.ValueStringList, Strings: []string{"core integration"}}
	nonemptyValues["keywords"] = resourcecore.Value{Kind: resourcecore.ValueStringList, Strings: []string{"core", "integration"}}
	class, err = app.ResourceWriter.UpdateCatalog(ctx, resourcecore.CatalogUpdateRequest{
		Actor:            "core-integration-test",
		Kind:             resourcecore.KindClass,
		ID:               class.ID,
		ExpectedRevision: class.Revision,
		Active:           true,
		Values:           nonemptyValues,
	})
	if err != nil {
		t.Fatalf("ResourceWriter.UpdateCatalog() with nonempty lists error = %v", err)
	}
	storedClass = getClass(t, class.ID)
	if len(storedClass.Values["aliases"].Strings) != 1 || storedClass.Values["aliases"].Strings[0] != "core integration" || len(storedClass.Values["keywords"].Strings) != 2 || storedClass.Values["keywords"].Strings[1] != "integration" {
		t.Fatalf("nonempty aliases/keywords were not persisted: %+v", storedClass.Values)
	}

	clearedValues := classValues()
	clearedValues["aliases"] = resourcecore.Value{Kind: resourcecore.ValueStringList, Strings: nil}
	clearedValues["keywords"] = resourcecore.Value{Kind: resourcecore.ValueStringList, Strings: []string{}}
	class, err = app.ResourceWriter.UpdateCatalog(ctx, resourcecore.CatalogUpdateRequest{
		Actor:            "core-integration-test",
		Kind:             resourcecore.KindClass,
		ID:               class.ID,
		ExpectedRevision: class.Revision,
		Active:           true,
		Values:           clearedValues,
	})
	if err != nil {
		t.Fatalf("ResourceWriter.UpdateCatalog() clearing lists error = %v", err)
	}
	storedClass = getClass(t, class.ID)
	assertEmptyStringList(t, storedClass, "aliases")
	assertEmptyStringList(t, storedClass, "keywords")

	class, err = app.ResourceWriter.UpdateCatalog(ctx, resourcecore.CatalogUpdateRequest{
		Actor:            "core-integration-test",
		Kind:             resourcecore.KindClass,
		ID:               class.ID,
		ExpectedRevision: class.Revision,
		Active:           true,
		Values:           classValues(),
	})
	if err != nil {
		t.Fatalf("ResourceWriter.UpdateCatalog() omitting lists error = %v", err)
	}
	storedClass = getClass(t, class.ID)
	assertEmptyStringList(t, storedClass, "aliases")
	assertEmptyStringList(t, storedClass, "keywords")

	activeClasses, err := app.ResourceReader.ActiveClasses(ctx)
	if err != nil {
		t.Fatalf("ResourceReader.ActiveClasses() after create error = %v", err)
	}
	classPublished := false
	for _, activeClass := range activeClasses {
		if activeClass.Values["code"].Text == classCode {
			classPublished = true
			break
		}
	}
	if !classPublished {
		t.Fatalf("ResourceReader.ActiveClasses() after create did not include class code %q", classCode)
	}

	supplier, err := app.SupplierWriter.CreateSupplier(ctx, suppliercore.SupplierWriteRequest{
		Actor:     "core-integration-test",
		TradeName: fmt.Sprintf("Core integration supplier %s", suffix),
	})
	if err != nil {
		t.Fatalf("SupplierWriter.CreateSupplier() error = %v", err)
	}
	defer func() {
		if _, err := adminPool.Exec(ctx, `DELETE FROM public.suppliers WHERE id=$1`, supplier.ID); err != nil {
			t.Errorf("cleanup supplier: %v", err)
		}
	}()
}
