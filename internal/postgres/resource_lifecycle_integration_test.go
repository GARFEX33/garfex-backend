package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/app/recursos"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResourceRepositoryLifecycleIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer p.Close()
	cleanupResources(ctx, t, p)
	defer cleanupResources(ctx, t, p)

	catalog := domain.SeedResourceCatalog()
	resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{
		domain.OptionValue("conductor_material", "COBRE"),
		domain.OptionValue("gauge", "10 AWG"),
		domain.OptionValue("insulation", "THW"),
		domain.OptionValue("color", "NEGRO"),
		domain.OptionValue("voltage", "600 V"),
	})
	repo := NewResourceRepository(p)
	if err := repo.Create(ctx, resource); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	stored, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}

	deactivated, err := repo.Deactivate(ctx, stored.ID)
	if err != nil || !deactivated.Changed || deactivated.Resource.Active {
		t.Fatalf("Deactivate() = %+v, %v; want changed inactive result", deactivated, err)
	}
	repeated, err := repo.Deactivate(ctx, stored.ID)
	if err != nil || repeated.Changed || repeated.Resource.Active {
		t.Fatalf("repeated Deactivate() = %+v, %v; want inactive no-op", repeated, err)
	}
	if active, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES"}); err != nil || len(active) != 0 {
		t.Fatalf("default Search() after Deactivate = %#v, %v; want no rows", active, err)
	}
	inactive, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", LifecycleScope: domain.LifecycleScopeInactive})
	if err != nil || len(inactive) != 1 || inactive[0].Active {
		t.Fatalf("inactive Search() = %#v, %v; want one inactive resource", inactive, err)
	}

	if _, err := p.Exec(ctx, `UPDATE public.resource_families SET active=FALSE WHERE code='CONDUCTORES'`); err != nil {
		t.Fatalf("deactivate catalog dependency: %v", err)
	}
	defer func() {
		if _, err := p.Exec(ctx, `UPDATE public.resource_families SET active=TRUE WHERE code='CONDUCTORES'`); err != nil {
			t.Errorf("restore catalog dependency: %v", err)
		}
	}()
	if _, err := repo.Reactivate(ctx, stored.ID, resource.IdentityKey); !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("Reactivate() with inactive catalog = %v, want ErrResourceReference", err)
	}
	stillInactive, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey)
	if err != nil || stillInactive.Active {
		t.Fatalf("Get() after failed Reactivate = %+v, %v; want inactive", stillInactive, err)
	}
	if _, err := p.Exec(ctx, `UPDATE public.resource_families SET active=TRUE WHERE code='CONDUCTORES'`); err != nil {
		t.Fatalf("restore catalog dependency: %v", err)
	}

	reactivated, err := repo.Reactivate(ctx, stored.ID, resource.IdentityKey)
	if err != nil || !reactivated.Changed || !reactivated.Resource.Active {
		t.Fatalf("Reactivate() = %+v, %v; want changed active result", reactivated, err)
	}
	if _, err := repo.Deactivate(ctx, stored.ID); err != nil {
		t.Fatalf("Deactivate() before conflict = %v", err)
	}
	if _, err := repo.Reactivate(ctx, stored.ID, "conflicting-identity"); !errors.Is(err, domain.ErrResourceIntegrity) {
		t.Fatalf("Reactivate() identity conflict = %v, want ErrResourceIntegrity", err)
	}
	var originalActive bool
	if err := p.QueryRow(ctx, `SELECT active FROM public.recursos WHERE id=$1`, stored.ID).Scan(&originalActive); err != nil || originalActive {
		t.Fatalf("original active after conflict = %v, %v; want inactive", originalActive, err)
	}
}

// TestResourceServiceCanonicalReactivationRevisionIntegration is the
// mandatory canonical reactivation exit evidence for design "Resource
// revisions and identity": create/deactivate/reactivate exclusively through
// recursos.Service (never the repository directly), passing the current
// revision, and asserting unchanged identity-v1, exact revision increments,
// complete attributes, and authoritative validation.
func TestResourceServiceCanonicalReactivationRevisionIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer p.Close()
	cleanupResources(ctx, t, p)
	defer cleanupResources(ctx, t, p)

	catalog := domain.SeedResourceCatalog()
	svc := recursos.NewService(NewResourceRepository(p), catalog)
	attributes := []domain.ResourceAttributeValue{
		domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"),
		domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V"),
	}
	created, err := svc.Create(ctx, domain.CreateCommand{Scope: conductoresScope, NaturalUnit: "M", Attributes: attributes})
	if err != nil {
		t.Fatalf("Service.Create() = %v", err)
	}
	var id int64
	var revision uint64
	if err := p.QueryRow(ctx, `SELECT id, revision FROM public.recursos WHERE identity_key=$1`, created.IdentityKey).Scan(&id, &revision); err != nil || revision != 1 {
		t.Fatalf("post-create id/revision = %d/%d, %v; want revision 1 (migration-8 default)", id, revision, err)
	}

	deactivated, err := svc.DeactivateRevision(ctx, id, 1)
	if err != nil || !deactivated.Changed || deactivated.Resource.Active || deactivated.Resource.Revision != 2 || deactivated.Resource.IdentityKey != created.IdentityKey {
		t.Fatalf("DeactivateRevision() = %+v, %v; want changed inactive revision 2 with unchanged identity", deactivated, err)
	}
	noop, err := svc.DeactivateRevision(ctx, id, 2)
	if err != nil || noop.Changed || noop.Resource.Revision != 2 {
		t.Fatalf("idempotent DeactivateRevision() = %+v, %v; want unchanged no-op at revision 2", noop, err)
	}

	// TRIANGULATE: stale revision is distinct from not-found/identity
	// mismatch; the failed attempt leaves attributes untouched (rollback).
	if _, err := svc.ReactivateRevision(ctx, id, 1); !errors.Is(err, domain.ErrResourceRevisionConflict) {
		t.Fatalf("ReactivateRevision() with stale revision = %v, want ErrResourceRevisionConflict", err)
	}
	stillInactive, err := svc.Get(ctx, created.ClassCode, created.IdentityKey)
	if err != nil || stillInactive.Active || len(stillInactive.Attributes) != len(attributes) {
		t.Fatalf("Get() after stale-revision conflict = %+v, %v; want inactive with %d complete attributes (rollback proof)", stillInactive, err, len(attributes))
	}

	// TRIANGULATE: authoritative validation — the resource's catalog scope
	// must still be active, even at a current revision.
	if _, err := p.Exec(ctx, `UPDATE public.resource_families SET active=FALSE WHERE code='CONDUCTORES'`); err != nil {
		t.Fatalf("deactivate catalog dependency: %v", err)
	}
	if _, err := svc.ReactivateRevision(ctx, id, 2); !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("ReactivateRevision() with inactive catalog dependency = %v, want ErrResourceReference", err)
	}
	var revisionAfterFailedReactivate uint64
	if err := p.QueryRow(ctx, `SELECT revision FROM public.recursos WHERE id=$1`, id).Scan(&revisionAfterFailedReactivate); err != nil || revisionAfterFailedReactivate != 2 {
		t.Fatalf("revision after failed authoritative validation = %d, %v; want unchanged 2", revisionAfterFailedReactivate, err)
	}
	if _, err := p.Exec(ctx, `UPDATE public.resource_families SET active=TRUE WHERE code='CONDUCTORES'`); err != nil {
		t.Fatalf("restore catalog dependency: %v", err)
	}

	reactivated, err := svc.ReactivateRevision(ctx, id, 2)
	if err != nil || !reactivated.Changed || !reactivated.Resource.Active || reactivated.Resource.Revision != 3 || reactivated.Resource.IdentityKey != created.IdentityKey {
		t.Fatalf("ReactivateRevision() = %+v, %v; want changed active revision 3 with unchanged identity", reactivated, err)
	}
	if len(reactivated.Resource.Attributes) != len(attributes) {
		t.Fatalf("reactivated attributes = %d, want %d (complete attributes)", len(reactivated.Resource.Attributes), len(attributes))
	}

	// TRIANGULATE: two independent connections — one advances the revision
	// past what the other still expects, which must conflict, not race.
	p2, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect second isolated connection: %v", err)
	}
	defer p2.Close()
	svc2 := recursos.NewService(NewResourceRepository(p2), catalog)

	if _, err := svc.DeactivateRevision(ctx, id, 3); err != nil {
		t.Fatalf("advance revision via first connection: %v", err)
	}
	if _, err := svc2.DeactivateRevision(ctx, id, 3); !errors.Is(err, domain.ErrResourceRevisionConflict) {
		t.Fatalf("second connection's stale DeactivateRevision() = %v, want ErrResourceRevisionConflict", err)
	}
	var finalRevision uint64
	if err := p.QueryRow(ctx, `SELECT revision FROM public.recursos WHERE id=$1`, id).Scan(&finalRevision); err != nil || finalRevision != 4 {
		t.Fatalf("final revision = %d, %v; want exactly-once increment to 4", finalRevision, err)
	}

	// TRIANGULATE: absent-row lookup is ErrResourceNotFound.
	if _, err := svc.DeactivateRevision(ctx, 987654321, 1); !errors.Is(err, domain.ErrResourceNotFound) {
		t.Fatalf("DeactivateRevision() on absent id = %v, want ErrResourceNotFound", err)
	}
}

// TestResourceRepositoryUpdateRevisionAtomicCAS is the 3H correction's exit
// evidence (design "Update replaces parent and attributes atomically only
// after CAS"): stale-revision and forced attribute-write failure both leave
// no partial write, and a successful update fully replaces the attribute
// set while identity-v1 stays byte-for-byte unchanged.
func TestResourceRepositoryUpdateRevisionAtomicCAS(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer p.Close()
	cleanupResources(ctx, t, p)
	defer cleanupResources(ctx, t, p)

	catalog := domain.SeedResourceCatalog()
	originalAttributes := []domain.ResourceAttributeValue{
		domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"),
		domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V"),
	}
	resource := mustCreateResource(t, catalog, conductoresScope, "M", originalAttributes)
	repo := NewResourceRepository(p)
	v2, ok := repo.(domain.ResourceRepositoryV2)
	if !ok {
		t.Fatalf("NewResourceRepository() does not implement domain.ResourceRepositoryV2")
	}
	if err := repo.Create(ctx, resource); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	stored, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	mustScalar := func(query string) uint64 {
		var got uint64
		if err := p.QueryRow(ctx, query, stored.ID).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return got
	}
	initialRevision := mustScalar(`SELECT revision FROM public.recursos WHERE id=$1`)
	if initialRevision != 1 {
		t.Fatalf("initial revision = %d, want 1", initialRevision)
	}
	newAttributes := []domain.ResourceAttributeValue{
		domain.OptionValue("conductor_material", "ALUMINIO"), domain.OptionValue("gauge", "8 AWG"),
		domain.OptionValue("insulation", "THHN"), domain.OptionValue("color", "AZUL"), domain.OptionValue("voltage", "1000 V"),
	}

	// TRIANGULATE 1: stale revision is rejected without any partial write.
	staleCandidate := stored
	staleCandidate.Attributes = newAttributes
	if _, err := v2.UpdateRevision(ctx, staleCandidate, initialRevision+1); !errors.Is(err, domain.ErrResourceRevisionConflict) {
		t.Fatalf("UpdateRevision() with stale revision = %v, want ErrResourceRevisionConflict", err)
	}
	assertRoundTrip(t, ctx, repo, stored)
	if got := mustScalar(`SELECT revision FROM public.recursos WHERE id=$1`); got != initialRevision {
		t.Fatalf("revision after stale UpdateRevision = %d, want unchanged %d (no partial write)", got, initialRevision)
	}

	// TRIANGULATE 2: an unresolvable attribute rolls back the parent CAS too.
	failingCandidate := stored
	failingCandidate.Attributes = append([]domain.ResourceAttributeValue(nil), originalAttributes...)
	failingCandidate.Attributes[0].AttributeCode = "nonexistent_attribute_code"
	if _, err := v2.UpdateRevision(ctx, failingCandidate, initialRevision); !errors.Is(err, domain.ErrResourceIntegrity) {
		t.Fatalf("UpdateRevision() with unresolvable attribute = %v, want ErrResourceIntegrity", err)
	}
	assertRoundTrip(t, ctx, repo, stored)
	if got := mustScalar(`SELECT revision FROM public.recursos WHERE id=$1`); got != initialRevision {
		t.Fatalf("revision after failed attribute write = %d, want unchanged %d (rollback proof)", got, initialRevision)
	}
	if got := mustScalar(`SELECT COUNT(*) FROM public.resource_attribute_values WHERE resource_id=$1`); got != uint64(len(originalAttributes)) {
		t.Fatalf("attribute count after failed write = %d, want unchanged %d (rollback proof)", got, len(originalAttributes))
	}

	// GREEN: replaces attributes and keeps identity-v1 unchanged (every changed attribute participates in it).
	candidate := stored
	candidate.Attributes = newAttributes
	updated, err := v2.UpdateRevision(ctx, candidate, initialRevision)
	if err != nil {
		t.Fatalf("UpdateRevision() = %v", err)
	}
	if updated.Revision != initialRevision+1 || updated.IdentityKey != resource.IdentityKey {
		t.Fatalf("UpdateRevision() = %+v, want revision %d and unchanged identity %q (never regenerated)", updated, initialRevision+1, resource.IdentityKey)
	}
	var storedIdentityKey string
	if err := p.QueryRow(ctx, `SELECT identity_key FROM public.recursos WHERE id=$1`, stored.ID).Scan(&storedIdentityKey); err != nil || storedIdentityKey != resource.IdentityKey {
		t.Fatalf("persisted identity_key = %q, %v; want unchanged %q", storedIdentityKey, err, resource.IdentityKey)
	}
	// assertRoundTrip requires an exact attribute-code/count match: complete replacement, never merge/append.
	assertRoundTrip(t, ctx, repo, domain.Resource{
		ID: stored.ID, ClassCode: stored.ClassCode, FamilyCode: stored.FamilyCode, TypeCode: stored.TypeCode,
		NaturalUnit: stored.NaturalUnit, IdentityKey: stored.IdentityKey, Attributes: newAttributes,
	})
}

// TestResourceRepositoryConcurrentRevisionRaceIntegration is 3I's mandatory
// true concurrent two-connection race proof (design "PostgreSQL fixtures and
// evidence": "two independent database connections issue writes with the
// same expected revision"; 3H explicitly deferred this exact goroutine
// variant to 3I). Two independent *pgxpool.Pool connections both call
// DeactivateRevision with the SAME expected revision, released together off
// one start barrier so both are genuinely in flight rather than issued
// sequentially. Both share setLifecycleRevision's row-locked
// `SELECT ... FOR UPDATE`, so PostgreSQL deterministically serializes them —
// never flaky, never a half-committed/speculative read: whichever
// transaction acquires the lock first commits and advances the revision by
// exactly one; the other re-reads under lock and its CAS predicate then
// observes that advanced revision, so it is rejected as stale rather than
// racing to a second, corrupting success.
func TestResourceRepositoryConcurrentRevisionRaceIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	p1, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect first isolated connection: %v", err)
	}
	defer p1.Close()
	cleanupResources(ctx, t, p1)
	defer cleanupResources(ctx, t, p1)
	p2, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect second isolated connection: %v", err)
	}
	defer p2.Close()

	catalog := domain.SeedResourceCatalog()
	resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{
		domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"),
		domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V"),
	})
	repo1 := NewResourceRepository(p1)
	if err := repo1.Create(ctx, resource); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	stored, err := repo1.Get(ctx, resource.ClassCode, resource.IdentityKey)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	v1, ok := repo1.(domain.ResourceRepositoryV2)
	if !ok {
		t.Fatalf("first connection repository does not implement domain.ResourceRepositoryV2")
	}
	v2, ok := NewResourceRepository(p2).(domain.ResourceRepositoryV2)
	if !ok {
		t.Fatalf("second connection repository does not implement domain.ResourceRepositoryV2")
	}

	start := make(chan struct{})
	var ready, wg sync.WaitGroup
	ready.Add(2)
	wg.Add(2)
	results := make([]domain.Resource, 2)
	errs := make([]error, 2)
	fire := func(slot int, v domain.ResourceRepositoryV2) {
		defer wg.Done()
		ready.Done()
		<-start
		results[slot], errs[slot] = v.DeactivateRevision(ctx, stored.ID, 1)
	}
	go fire(0, v1)
	go fire(1, v2)
	ready.Wait()
	close(start)
	wg.Wait()

	successes, conflicts := 0, 0
	for i, err := range errs {
		switch {
		case err == nil:
			successes++
			if results[i].Active || results[i].Revision != 2 {
				t.Fatalf("winning connection result = %+v, want inactive at revision 2", results[i])
			}
		case errors.Is(err, domain.ErrResourceRevisionConflict):
			conflicts++
		default:
			t.Fatalf("connection %d unexpected error = %v, want nil or ErrResourceRevisionConflict", i, err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("race outcome = %d successes, %d conflicts; want exactly one of each (never both succeeding, never both rejected)", successes, conflicts)
	}
	var finalRevision uint64
	if err := p1.QueryRow(ctx, `SELECT revision FROM public.recursos WHERE id=$1`, stored.ID).Scan(&finalRevision); err != nil || finalRevision != 2 {
		t.Fatalf("final revision = %d, %v; want exactly-once increment to 2 (no speculative publication)", finalRevision, err)
	}
}
