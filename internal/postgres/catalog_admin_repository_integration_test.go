package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCatalogAdminRepositoryIntegration proves catalog_admin_repository.go's
// generic per-kind CRUD (List/Get/Insert/Update/SetActive/Delete),
// dependency probes (Dependents), and Código-immutability enforcement
// (design D11 layer 2: the repository omits ImmutableOnceReferenced columns
// from the generated UPDATE ... SET once referenced, rather than refusing
// the whole write — a real design decision, not an ambiguous one this test
// had to guess at) against a real PostgreSQL instance (design's Testing
// Strategy row for this file; task 4.3).
//
// GARFEX_TEST_DSN for this test MUST be a garfex_app-privilege DSN (same
// requirement as catalog_admin_grants_integration_test.go) — every write
// below succeeding without a 42501 permission-denied error is itself live
// proof that PR1's GRANT fix and this PR's repository code compose
// correctly end to end. Skips per this project's existing GARFEX_TEST_DSN
// convention; fixtures are TEST_-prefixed and cleaned up by the test itself
// (testing-data-boundary.md — never a blanket DELETE).
//
// Not every one of the 11 registered CatalogKinds gets its own round trip
// here (that would make this file unreasonably large) — per the coverage
// this test targets one representative kind from each structural shape:
// Class (top-level), Family/Type (child), AttributeDefinition (vocabulary),
// Option (option-set-scoped), OptionRelation (junction/relation, real
// serial id but no código field).
func TestCatalogAdminRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	repo := NewCatalogAdminRepository(pool)

	must := func(st *testing.T, label string, err error) {
		st.Helper()
		if err != nil {
			st.Fatalf("%s: %v", label, err)
		}
	}

	var classID, fam1ID, fam2ID, typeID int64
	var definitionID, optionSetID, opt1ID int64
	var caract2ID, opt2ID, relationID int64

	t.Run("class family type CRUD round trip and dependents", func(st *testing.T) {
		classID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindClass, Active: true,
			Values: map[string]domain.CatalogValue{
				"code": {Text: "TEST_REPO_CLASS"}, "name": {Text: "Test Repo Clase"}, "plural": {Text: "Test Repo Clases"},
				"slug": {Text: "test-repo-clase"}, "order": {Int: 99}, "aliases": {List: []string{"trc"}}, "keywords": {List: []string{"prueba"}},
			},
		})
		must(st, "insert class", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindClass, classID); err != nil {
				t.Errorf("cleanup class: %v", err)
			}
		})

		got, err := repo.Get(ctx, domain.KindClass, classID)
		must(st, "get class", err)
		if got.Values["code"].Text != "TEST_REPO_CLASS" || got.Values["name"].Text != "Test Repo Clase" || !got.Active {
			st.Fatalf("Get() class = %+v, want code/name/active to match insert", got)
		}

		list, err := repo.List(ctx, domain.KindClass, domain.CatalogFilter{Text: "TEST_REPO_CLASS"})
		must(st, "list classes", err)
		if len(list) != 1 || list[0].ID != classID {
			st.Fatalf("List() classes = %+v, want exactly the inserted class", list)
		}

		must(st, "update class", repo.Update(ctx, domain.CatalogRecord{
			Kind: domain.KindClass, ID: classID, Active: true,
			Values: map[string]domain.CatalogValue{
				"code": {Text: "TEST_REPO_CLASS"}, "name": {Text: "Test Repo Clase Renombrada"}, "plural": {Text: "Test Repo Clases"},
				"slug": {Text: "test-repo-clase"}, "order": {Int: 5}, "aliases": {List: []string{"trc"}}, "keywords": {List: []string{"prueba"}},
			},
		}))
		got, err = repo.Get(ctx, domain.KindClass, classID)
		must(st, "get class after update", err)
		if got.Values["name"].Text != "Test Repo Clase Renombrada" || got.Values["order"].Int != 5 {
			st.Fatalf("Update() class = %+v, want renamed name and order=5", got)
		}

		must(st, "deactivate class", repo.SetActive(ctx, domain.KindClass, classID, false))
		got, err = repo.Get(ctx, domain.KindClass, classID)
		must(st, "get class after deactivate", err)
		if got.Active {
			st.Fatalf("SetActive(false) class Active = true, want false")
		}
		must(st, "reactivate class", repo.SetActive(ctx, domain.KindClass, classID, true))
		got, err = repo.Get(ctx, domain.KindClass, classID)
		must(st, "get class after reactivate", err)
		if !got.Active {
			st.Fatalf("SetActive(true) class Active = false, want true")
		}

		fam1ID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindFamily, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_REPO_CLASS"}},
				"code":  {Text: "TEST_REPO_FAM1"}, "name": {Text: "Test Repo Familia 1"},
			},
		})
		must(st, "insert family 1", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindFamily, fam1ID); err != nil {
				t.Errorf("cleanup family 1: %v", err)
			}
		})

		fam2ID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindFamily, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_REPO_CLASS"}},
				"code":  {Text: "TEST_REPO_FAM2"}, "name": {Text: "Test Repo Familia 2"},
			},
		})
		must(st, "insert family 2", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindFamily, fam2ID); err != nil {
				t.Errorf("cleanup family 2: %v", err)
			}
		})

		got, err = repo.Get(ctx, domain.KindFamily, fam1ID)
		must(st, "get family 1", err)
		if got.Values["class"].Ref.Code != "TEST_REPO_CLASS" || got.Values["code"].Text != "TEST_REPO_FAM1" {
			st.Fatalf("Get() family 1 = %+v", got)
		}
		if got.Values["class"].Ref.Label != "Test Repo Clase Renombrada" {
			st.Fatalf("Get() family class label = %q, want enriched human name", got.Values["class"].Ref.Label)
		}

		must(st, "update family 1", repo.Update(ctx, domain.CatalogRecord{
			Kind: domain.KindFamily, ID: fam1ID, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_REPO_CLASS"}},
				"code":  {Text: "TEST_REPO_FAM1"}, "name": {Text: "Test Repo Familia 1 Renombrada"},
			},
		}))
		got, err = repo.Get(ctx, domain.KindFamily, fam1ID)
		must(st, "get family 1 after update", err)
		if got.Values["name"].Text != "Test Repo Familia 1 Renombrada" {
			st.Fatalf("Update() family 1 name = %q, want renamed", got.Values["name"].Text)
		}

		deps, err := repo.Dependents(ctx, domain.KindClass, classID)
		must(st, "dependents class", err)
		if depCount(deps, domain.KindFamily) != 2 {
			st.Fatalf("Dependents(KindClass) Family count = %d, want 2", depCount(deps, domain.KindFamily))
		}

		typeID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindType, Active: true,
			Values: map[string]domain.CatalogValue{
				"class":  {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_REPO_CLASS"}},
				"family": {Ref: domain.CatalogRef{Kind: domain.KindFamily, Code: "TEST_REPO_FAM1"}},
				"code":   {Text: "TEST_REPO_TYPE"}, "name": {Text: "Test Repo Tipo"},
			},
		})
		must(st, "insert type", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindType, typeID); err != nil {
				t.Errorf("cleanup type: %v", err)
			}
		})

		got, err = repo.Get(ctx, domain.KindType, typeID)
		must(st, "get type", err)
		if got.Values["family"].Ref.Code != "TEST_REPO_FAM1" || got.Values["code"].Text != "TEST_REPO_TYPE" {
			st.Fatalf("Get() type = %+v", got)
		}
		if got.Values["class"].Ref.Label != "Test Repo Clase Renombrada" || got.Values["family"].Ref.Label != "Test Repo Familia 1 Renombrada" {
			st.Fatalf("Get() type hierarchy labels = %#v, want enriched class/family names", got.Values)
		}

		must(st, "deactivate type", repo.SetActive(ctx, domain.KindType, typeID, false))
		must(st, "reactivate type", repo.SetActive(ctx, domain.KindType, typeID, true))

		deps, err = repo.Dependents(ctx, domain.KindFamily, fam1ID)
		must(st, "dependents family 1", err)
		if depCount(deps, domain.KindType) != 1 {
			st.Fatalf("Dependents(KindFamily) Type count = %d, want 1", depCount(deps, domain.KindType))
		}
	})

	t.Run("attribute definition and option CRUD round trip", func(st *testing.T) {
		definitionID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindAttributeDefinition, Active: true,
			Values: map[string]domain.CatalogValue{
				"code": {Text: "test_repo_caract"}, "name": {Text: "Test Repo Característica"},
				"valueType": {Text: "CONTROLLED_OPTION"}, "dimension": {Text: ""}, "defaultIdentityParticipates": {Bool: true},
			},
		})
		must(st, "insert attribute definition", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindAttributeDefinition, definitionID); err != nil {
				t.Errorf("cleanup attribute definition: %v", err)
			}
		})

		got, err := repo.Get(ctx, domain.KindAttributeDefinition, definitionID)
		must(st, "get attribute definition", err)
		if got.Values["code"].Text != "test_repo_caract" || got.Values["valueType"].Text != "CONTROLLED_OPTION" {
			st.Fatalf("Get() attribute definition = %+v", got)
		}

		must(st, "update attribute definition", repo.Update(ctx, domain.CatalogRecord{
			Kind: domain.KindAttributeDefinition, ID: definitionID, Active: true,
			Values: map[string]domain.CatalogValue{
				"code": {Text: "test_repo_caract"}, "name": {Text: "Test Repo Característica Renombrada"},
				"valueType": {Text: "CONTROLLED_OPTION"}, "dimension": {Text: "LENGTH"}, "defaultIdentityParticipates": {Bool: false},
			},
		}))
		got, err = repo.Get(ctx, domain.KindAttributeDefinition, definitionID)
		must(st, "get attribute definition after update", err)
		if got.Values["name"].Text != "Test Repo Característica Renombrada" || got.Values["dimension"].Text != "LENGTH" || got.Values["defaultIdentityParticipates"].Bool {
			st.Fatalf("Update() attribute definition = %+v, want updated name/dimension/defaultIdentityParticipates", got)
		}

		must(st, "deactivate attribute definition", repo.SetActive(ctx, domain.KindAttributeDefinition, definitionID, false))
		got, err = repo.Get(ctx, domain.KindAttributeDefinition, definitionID)
		must(st, "get attribute definition after deactivate", err)
		if got.Active {
			st.Fatalf("SetActive(false) attribute definition Active = true, want false")
		}
		must(st, "reactivate attribute definition", repo.SetActive(ctx, domain.KindAttributeDefinition, definitionID, true))

		optionSetID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindOptionSet, Active: true,
			Values: map[string]domain.CatalogValue{"code": {Text: "TEST_REPO_OPTSET"}, "name": {Text: "Test Repo Conjunto"}},
		})
		must(st, "insert option set", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindOptionSet, optionSetID); err != nil {
				t.Errorf("cleanup option set: %v", err)
			}
		})

		opt1ID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindOption, Active: true,
			Values: map[string]domain.CatalogValue{
				"optionSet":      {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_REPO_OPTSET"}},
				"characteristic": {Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: "test_repo_caract"}},
				"code":           {Text: "TEST_REPO_OPT1"}, "label": {Text: "Opción Test 1"},
			},
		})
		must(st, "insert option", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindOption, opt1ID); err != nil {
				t.Errorf("cleanup option 1: %v", err)
			}
		})

		got, err = repo.Get(ctx, domain.KindOption, opt1ID)
		must(st, "get option", err)
		if got.Values["code"].Text != "TEST_REPO_OPT1" || got.Values["label"].Text != "Opción Test 1" {
			st.Fatalf("Get() option = %+v", got)
		}

		must(st, "update option", repo.Update(ctx, domain.CatalogRecord{
			Kind: domain.KindOption, ID: opt1ID, Active: true,
			Values: map[string]domain.CatalogValue{"code": {Text: "TEST_REPO_OPT1"}, "label": {Text: "Opción Test 1 Renombrada"}},
		}))
		got, err = repo.Get(ctx, domain.KindOption, opt1ID)
		must(st, "get option after update", err)
		if got.Values["label"].Text != "Opción Test 1 Renombrada" {
			st.Fatalf("Update() option label = %q, want renamed", got.Values["label"].Text)
		}

		must(st, "deactivate option", repo.SetActive(ctx, domain.KindOption, opt1ID, false))
		must(st, "reactivate option", repo.SetActive(ctx, domain.KindOption, opt1ID, true))

		deps, err := repo.Dependents(ctx, domain.KindOptionSet, optionSetID)
		must(st, "dependents option set", err)
		if depCount(deps, domain.KindOption) != 1 {
			st.Fatalf("Dependents(KindOptionSet) Option count = %d, want 1", depCount(deps, domain.KindOption))
		}

		deps, err = repo.Dependents(ctx, domain.KindAttributeDefinition, definitionID)
		must(st, "dependents attribute definition", err)
		if depCount(deps, domain.KindOption) != 1 {
			st.Fatalf("Dependents(KindAttributeDefinition) Option count = %d, want 1", depCount(deps, domain.KindOption))
		}
	})

	t.Run("option relation junction kind CRUD round trip", func(st *testing.T) {
		caract2ID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindAttributeDefinition, Active: true,
			Values: map[string]domain.CatalogValue{
				"code": {Text: "test_repo_caract2"}, "name": {Text: "Test Repo Característica 2"},
				"valueType": {Text: "CONTROLLED_OPTION"},
			},
		})
		must(st, "insert attribute definition 2", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindAttributeDefinition, caract2ID); err != nil {
				t.Errorf("cleanup attribute definition 2: %v", err)
			}
		})

		opt2ID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindOption, Active: true,
			Values: map[string]domain.CatalogValue{
				"optionSet":      {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_REPO_OPTSET"}},
				"characteristic": {Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: "test_repo_caract2"}},
				"code":           {Text: "TEST_REPO_OPT2"}, "label": {Text: "Opción Test 2"},
			},
		})
		must(st, "insert option 2", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindOption, opt2ID); err != nil {
				t.Errorf("cleanup option 2: %v", err)
			}
		})

		relationID, err = repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindOptionRelation, Active: true,
			Values: map[string]domain.CatalogValue{
				"optionSet":          {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_REPO_OPTSET"}},
				"fromCharacteristic": {Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: "test_repo_caract"}},
				"fromOption":         {Ref: domain.CatalogRef{Kind: domain.KindOption, Code: "TEST_REPO_OPT1"}},
				"toCharacteristic":   {Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: "test_repo_caract2"}},
				"toOption":           {Ref: domain.CatalogRef{Kind: domain.KindOption, Code: "TEST_REPO_OPT2"}},
			},
		})
		must(st, "insert option relation", err)
		t.Cleanup(func() {
			if err := repo.Delete(ctx, domain.KindOptionRelation, relationID); err != nil {
				t.Errorf("cleanup option relation: %v", err)
			}
		})

		got, err := repo.Get(ctx, domain.KindOptionRelation, relationID)
		must(st, "get option relation", err)
		if got.Values["fromOption"].Ref.Code != "TEST_REPO_OPT1" || got.Values["toOption"].Ref.Code != "TEST_REPO_OPT2" || !got.Active {
			st.Fatalf("Get() option relation = %+v", got)
		}

		list, err := repo.List(ctx, domain.KindOptionRelation, domain.CatalogFilter{Parent: map[string]domain.CatalogValue{
			"optionSet": {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_REPO_OPTSET"}},
		}})
		must(st, "list option relations", err)
		if len(list) != 1 || list[0].ID != relationID {
			st.Fatalf("List() option relations = %+v, want exactly the inserted relation", list)
		}

		must(st, "deactivate option relation", repo.SetActive(ctx, domain.KindOptionRelation, relationID, false))
		got, err = repo.Get(ctx, domain.KindOptionRelation, relationID)
		must(st, "get option relation after deactivate", err)
		if got.Active {
			st.Fatalf("SetActive(false) option relation Active = true, want false")
		}
		must(st, "reactivate option relation", repo.SetActive(ctx, domain.KindOptionRelation, relationID, true))

		deps, err := repo.Dependents(ctx, domain.KindOption, opt1ID)
		must(st, "dependents option 1", err)
		if depCount(deps, domain.KindOptionRelation) != 1 {
			st.Fatalf("Dependents(KindOption) OptionRelation count = %d, want 1", depCount(deps, domain.KindOptionRelation))
		}

		referenced, err := repo.ReferencedByResources(ctx, domain.KindOptionRelation, relationID)
		must(st, "referenced by resources option relation", err)
		if referenced {
			st.Fatalf("ReferencedByResources(KindOptionRelation) = true, want false (junction kind exemption — no código field)")
		}
	})

	t.Run("código immutability once referenced by resources, and guarded delete", func(st *testing.T) {
		referenced, err := repo.ReferencedByResources(ctx, domain.KindClass, classID)
		must(st, "referenced by resources before", err)
		if referenced {
			st.Fatalf("ReferencedByResources(KindClass) = true before creating a resource, want false")
		}

		var unitID int64
		must(st, "resolve seeded unit PZA", pool.QueryRow(ctx, `SELECT id FROM public.unit_definitions WHERE code='PZA'`).Scan(&unitID))

		var resourceID int64
		must(st, "insert recursos", pool.QueryRow(ctx, `
			INSERT INTO public.recursos (class_id, family_id, type_id, natural_unit_id, display_name, identity_key)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			classID, fam1ID, typeID, unitID, "Test Repo Recurso", canonicalResourceIdentityKey("TEST_REPO_CLASS", "TEST_REPO_FAM1", "TEST_REPO_TYPE")).Scan(&resourceID))
		t.Cleanup(func() {
			if _, err := pool.Exec(ctx, `DELETE FROM public.recursos WHERE id=$1`, resourceID); err != nil {
				t.Errorf("cleanup recursos: %v", err)
			}
		})

		referenced, err = repo.ReferencedByResources(ctx, domain.KindClass, classID)
		must(st, "referenced by resources after", err)
		if !referenced {
			st.Fatalf("ReferencedByResources(KindClass) = false after creating a resource, want true")
		}

		must(st, "update referenced class", repo.Update(ctx, domain.CatalogRecord{
			Kind: domain.KindClass, ID: classID, Active: true,
			Values: map[string]domain.CatalogValue{
				"code": {Text: "TEST_REPO_CLASS_RENAMED"}, "name": {Text: "Test Repo Clase Referenciada"}, "plural": {Text: "Test Repo Clases"},
				"slug": {Text: "test-repo-clase"}, "order": {Int: 7}, "aliases": {List: []string{"trc"}}, "keywords": {List: []string{"prueba"}},
			},
		}))

		got, err := repo.Get(ctx, domain.KindClass, classID)
		must(st, "get class after update", err)
		if got.Values["code"].Text != "TEST_REPO_CLASS" {
			st.Fatalf("Update() on a referenced class changed código to %q, want it left unchanged at TEST_REPO_CLASS "+
				"(design D11 layer 2: repository omits ImmutableOnceReferenced columns from the generated SET)", got.Values["code"].Text)
		}
		if got.Values["name"].Text != "Test Repo Clase Referenciada" {
			st.Fatalf("Update() on a referenced class left name = %q, want the new name (only código is protected)", got.Values["name"].Text)
		}

		err = repo.Delete(ctx, domain.KindClass, classID)
		if !errors.Is(err, domain.ErrCatalogInUse) {
			st.Fatalf("Delete() referenced class error = %v, want ErrCatalogInUse", err)
		}
	})

	t.Run("unit names accept duplicates and round trip", func(st *testing.T) {
		makeUnit := func(code, name, symbol string) domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnit, Active: true, Values: map[string]domain.CatalogValue{
				"code": {Text: code}, "name": {Text: name}, "symbol": {Text: symbol}, "dimension": {Text: "LENGTH"},
			}}
		}
		first, err := repo.Insert(ctx, makeUnit("TEST_REPO_UNIT_A", "Provisional: Código TEST_REPO_UNIT_A", "TRA"))
		must(st, "insert first unit", err)
		second, err := repo.Insert(ctx, makeUnit("TEST_REPO_UNIT_B", "Provisional: Código TEST_REPO_UNIT_A", "TRB"))
		must(st, "insert duplicate-name unit", err)
		st.Cleanup(func() { _ = repo.Delete(ctx, domain.KindUnit, first); _ = repo.Delete(ctx, domain.KindUnit, second) })
		updated := makeUnit("TEST_REPO_UNIT_A", "Unidad corregida", "TRA")
		updated.ID = first
		must(st, "update unit name", repo.Update(ctx, updated))
		got, err := repo.Get(ctx, domain.KindUnit, first)
		must(st, "get updated unit", err)
		if got.Values["name"].Text != "Unidad corregida" || got.Values["code"].Text != "TEST_REPO_UNIT_A" || got.Values["symbol"].Text != "TRA" {
			st.Fatalf("unit round-trip = %+v, want editable name and stable code/symbol", got)
		}
		list, err := repo.List(ctx, domain.KindUnit, domain.CatalogFilter{Text: "corregida"})
		if err != nil || len(list) != 1 || list[0].ID != first {
			st.Fatalf("unit name search = %+v, err=%v, want updated unit", list, err)
		}
		updated.Values["name"] = domain.CatalogValue{Text: "Unidad estable"}
		must(st, "correct provisional unit", repo.Update(ctx, updated))
		var classID, familyID, typeID, resourceID int64
		must(st, "resolve class", pool.QueryRow(ctx, `SELECT id FROM public.resource_classes WHERE code='MATERIAL'`).Scan(&classID))
		must(st, "resolve family", pool.QueryRow(ctx, `SELECT id FROM public.resource_families WHERE code='CONDUCTORES'`).Scan(&familyID))
		must(st, "resolve type", pool.QueryRow(ctx, `SELECT id FROM public.resource_types WHERE code='CABLE'`).Scan(&typeID))
		must(st, "insert stable reference", pool.QueryRow(ctx, `INSERT INTO public.recursos (class_id,family_id,type_id,natural_unit_id,display_name,identity_key) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, classID, familyID, typeID, first, "U2a restart", canonicalResourceIdentityKey("MATERIAL", "CONDUCTORES", "CABLE")).Scan(&resourceID))
		st.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM public.recursos WHERE id=$1", resourceID) })
		freshPool, err := pgxpool.New(ctx, dsn)
		must(st, "reconnect to PostgreSQL", err)
		st.Cleanup(freshPool.Close)
		freshRepo := NewCatalogAdminRepository(freshPool)
		got, err = freshRepo.Get(ctx, domain.KindUnit, first)
		if err != nil || got.Values["name"].Text != "Unidad estable" || got.Values["code"].Text != "TEST_REPO_UNIT_A" {
			st.Fatalf("reloaded unit = %+v, error = %v", got, err)
		}
		referenced, err := freshRepo.ReferencedByResources(ctx, domain.KindUnit, first)
		if err != nil || !referenced {
			st.Fatalf("reloaded unit reference = %v, error = %v; want stable reference", referenced, err)
		}
	})
}

// TestCatalogAdminRepositoryAttributeBindingParentFilterIntegration proves
// List(KindAttributeBinding, ...) actually scopes by the type and optionSet
// parent filters (listAttributeBindings previously built its WHERE clause
// from only class/family/characteristic, silently ignoring type/optionSet —
// caught live via garfex-api's ?typeCode=/?optionSetCode= query params
// returning every record regardless of the filter value).
func TestCatalogAdminRepositoryAttributeBindingParentFilterIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	repo := NewCatalogAdminRepository(pool)

	must := func(label string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
	}

	classID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindClass, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "TEST_FILTER_CLASS"}, "name": {Text: "Test Filter Clase"}, "plural": {Text: "Test Filter Clases"},
			"slug": {Text: "test-filter-clase"}, "order": {Int: 99},
		},
	})
	must("insert class", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindClass, classID) })

	famID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindFamily, Active: true,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_FILTER_CLASS"}},
			"code":  {Text: "TEST_FILTER_FAM"}, "name": {Text: "Test Filter Familia"},
		},
	})
	must("insert family", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindFamily, famID) })

	typeAID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindType, Active: true,
		Values: map[string]domain.CatalogValue{
			"class":  {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_FILTER_CLASS"}},
			"family": {Ref: domain.CatalogRef{Kind: domain.KindFamily, Code: "TEST_FILTER_FAM"}},
			"code":   {Text: "TEST_FILTER_TYPE_A"}, "name": {Text: "Test Filter Tipo A"},
		},
	})
	must("insert type A", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindType, typeAID) })

	typeBID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindType, Active: true,
		Values: map[string]domain.CatalogValue{
			"class":  {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_FILTER_CLASS"}},
			"family": {Ref: domain.CatalogRef{Kind: domain.KindFamily, Code: "TEST_FILTER_FAM"}},
			"code":   {Text: "TEST_FILTER_TYPE_B"}, "name": {Text: "Test Filter Tipo B"},
		},
	})
	must("insert type B", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindType, typeBID) })

	definitionID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindAttributeDefinition, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "test_filter_caract"}, "name": {Text: "Test Filter Característica"},
			"valueType": {Text: "CONTROLLED_OPTION"}, "dimension": {Text: ""}, "defaultIdentityParticipates": {Bool: true},
		},
	})
	must("insert attribute definition", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindAttributeDefinition, definitionID) })

	optSetAID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindOptionSet, Active: true,
		Values: map[string]domain.CatalogValue{"code": {Text: "TEST_FILTER_OPTSET_A"}, "name": {Text: "Test Filter Conjunto A"}},
	})
	must("insert option set A", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindOptionSet, optSetAID) })

	optSetBID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindOptionSet, Active: true,
		Values: map[string]domain.CatalogValue{"code": {Text: "TEST_FILTER_OPTSET_B"}, "name": {Text: "Test Filter Conjunto B"}},
	})
	must("insert option set B", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindOptionSet, optSetBID) })

	bindingAID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindAttributeBinding, Active: true,
		Values: map[string]domain.CatalogValue{
			"class":                {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_FILTER_CLASS"}},
			"family":               {Ref: domain.CatalogRef{Kind: domain.KindFamily, Code: "TEST_FILTER_FAM"}},
			"type":                 {Ref: domain.CatalogRef{Kind: domain.KindType, Code: "TEST_FILTER_TYPE_A"}},
			"characteristic":       {Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: "test_filter_caract"}},
			"optionSet":            {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_FILTER_OPTSET_A"}},
			"mode":                 {Text: "REQUIRED"},
			"identityParticipates": {Bool: true},
		},
	})
	must("insert attribute binding A", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindAttributeBinding, bindingAID) })

	bindingBID, err := repo.Insert(ctx, domain.CatalogRecord{
		Kind: domain.KindAttributeBinding, Active: true,
		Values: map[string]domain.CatalogValue{
			"class":                {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "TEST_FILTER_CLASS"}},
			"family":               {Ref: domain.CatalogRef{Kind: domain.KindFamily, Code: "TEST_FILTER_FAM"}},
			"type":                 {Ref: domain.CatalogRef{Kind: domain.KindType, Code: "TEST_FILTER_TYPE_B"}},
			"characteristic":       {Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: "test_filter_caract"}},
			"optionSet":            {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_FILTER_OPTSET_B"}},
			"mode":                 {Text: "REQUIRED"},
			"identityParticipates": {Bool: true},
		},
	})
	must("insert attribute binding B", err)
	t.Cleanup(func() { _ = repo.Delete(ctx, domain.KindAttributeBinding, bindingBID) })

	byType, err := repo.List(ctx, domain.KindAttributeBinding, domain.CatalogFilter{Parent: map[string]domain.CatalogValue{
		"type": {Ref: domain.CatalogRef{Kind: domain.KindType, Code: "TEST_FILTER_TYPE_A"}},
	}})
	must("list by type", err)
	if len(byType) != 1 || byType[0].ID != bindingAID {
		t.Fatalf("List() by type = %+v, want exactly binding A", byType)
	}

	byOptionSet, err := repo.List(ctx, domain.KindAttributeBinding, domain.CatalogFilter{Parent: map[string]domain.CatalogValue{
		"optionSet": {Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: "TEST_FILTER_OPTSET_B"}},
	}})
	must("list by option set", err)
	if len(byOptionSet) != 1 || byOptionSet[0].ID != bindingBID {
		t.Fatalf("List() by option set = %+v, want exactly binding B", byOptionSet)
	}

	byNonexistentType, err := repo.List(ctx, domain.KindAttributeBinding, domain.CatalogFilter{Parent: map[string]domain.CatalogValue{
		"type": {Ref: domain.CatalogRef{Kind: domain.KindType, Code: "TEST_FILTER_TYPE_NONE"}},
	}})
	must("list by nonexistent type", err)
	if len(byNonexistentType) != 0 {
		t.Fatalf("List() by nonexistent type = %+v, want empty", byNonexistentType)
	}
}

func depCount(deps []domain.CatalogDependency, kind domain.CatalogKindCode) int {
	for _, d := range deps {
		if d.Kind == kind {
			return d.Count
		}
	}
	return -1
}
