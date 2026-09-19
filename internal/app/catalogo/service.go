// Package catalogo provides the catalog-structure administration use cases
// (design §4, tasks 5.1/5.2): the generic descriptor-driven read/write API
// consumed by the future TUI admin engine (PR6+) and the AC#21 e2e proof
// (PR10). Service holds exactly one in-memory ResourceCatalog snapshot — the
// same value main.go loads at boot via postgres.LoadResourceCatalog — used
// SOLELY to validate a pending mutation against the whole catalog's
// structural invariants before persisting it (design D9); it is never the
// read source for List/Get (see their doc comments).
package catalogo

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/core"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
)

// ErrInvalidArgument is returned when a caller omits a required lookup key
// (e.g. a zero record ID). It aliases the core neutral classifier so the
// internal mapper recognizes it without parsing strings.
var ErrInvalidArgument = core.ErrInvalidArgument

// ErrCatalogAdminRepositoryV2Unavailable is returned by every additive V2
// write method when the Service was never wired via
// WithCatalogAdminRepositoryV2 — the V2 path is optional.
var ErrCatalogAdminRepositoryV2Unavailable = fmt.Errorf("%w: catalog admin repository v2 is not configured", core.ErrUnavailable)

// ErrCatalogWriteIndeterminate classifies a V2 write whose commit outcome
// could not be confirmed (design "Catalog transaction and coherent
// publication"). A domain.CatalogAdminRepositoryV2 implementation wraps this
// sentinel to signal it; receiving it latches ErrCatalogWriterUnavailable.
var ErrCatalogWriteIndeterminate = fmt.Errorf("%w: catalog write commit outcome is indeterminate", core.ErrUnavailable)

// ErrCatalogWriterUnavailable is returned once ErrCatalogWriteIndeterminate
// has latched the writer unavailable: no further V2 write is attempted, and
// nothing publishes, until ResetCatalogWriterAvailability runs.
var ErrCatalogWriterUnavailable = fmt.Errorf("%w: catalog writer is unavailable after an indeterminate commit; an explicit coherent reload/restart is required", core.ErrUnavailable)

// Service implements the catalog-structure admin use cases (design §4).
type Service struct {
	mu                sync.Mutex
	repo              domain.CatalogAdminRepository
	repoV2            domain.CatalogAdminRepositoryV2
	registry          domain.CatalogRegistry
	snapshot          domain.ResourceCatalog
	authority         *domain.CatalogAuthority
	writerUnavailable bool
}

// NewService returns a Service backed by repo, using registry to describe
// every administrable CatalogKind and snapshot as the initial in-memory
// catalog state (design §4) — normally the exact value postgres.
// LoadResourceCatalog returned at boot.
func NewService(repo domain.CatalogAdminRepository, registry domain.CatalogRegistry, snapshot domain.ResourceCatalog) *Service {
	return NewServiceWithCatalogAuthority(repo, registry, domain.NewCatalogAuthority(snapshot))
}

// NewServiceWithCatalogAuthority shares committed catalog versions with every
// consumer wired to authority.
func NewServiceWithCatalogAuthority(repo domain.CatalogAdminRepository, registry domain.CatalogRegistry, authority *domain.CatalogAuthority) *Service {
	snapshot, _ := authority.Current()
	return &Service{repo: repo, registry: registry, snapshot: snapshot, authority: authority}
}

// Kinds returns every registered CatalogKind (design §3) — the descriptor
// data the generic TUI admin engine renders from.
func (s *Service) Kinds() []domain.CatalogKind {
	return s.registry.Kinds()
}

// List returns kind's records matching filter, read directly from the
// repository. domain.ResourceCatalog carries no ID field (design D10), so
// only the repository can return the ID-bearing CatalogRecords a caller
// needs for a later Update/Deactivate/Reactivate/Delete/Dependencies call —
// the in-memory snapshot exists solely to run ApplyCatalogMutation/Validate,
// never to serve reads.
func (s *Service) List(ctx context.Context, kind domain.CatalogKindCode, filter domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	return s.repo.List(ctx, kind, filter)
}

// Get returns one record by kind and repository-assigned id, read directly
// from the repository (see List's doc comment for why).
func (s *Service) Get(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
	if id == 0 {
		return domain.CatalogRecord{}, ErrInvalidArgument
	}
	return s.repo.Get(ctx, kind, id)
}

// Dependencies reports which other kinds reference id, read directly from
// the repository's dependency probes (design §6) — the data a guarded
// Desactivar/Eliminar confirmation renders.
func (s *Service) Dependencies(ctx context.Context, kind domain.CatalogKindCode, id int64) ([]domain.CatalogDependency, error) {
	return s.repo.Dependents(ctx, kind, id)
}

// ReferencedByResources reports whether id is referenced by at least one
// real resource instance, read directly from the repository's
// ReferencedByResources probe (design §6). It is the TUI-facing signal task
// 7.2's Código-immutability UI (and the guarded-delete flow, task 7.1) need
// to decide, respectively, whether to render a "código" field read-only and
// whether a hard delete must be blocked in favor of Desactivar — the same
// probe Update already consults internally, exposed here as its own method
// since a caller may need the answer BEFORE attempting a write (e.g. to
// decide how to render a question in the first place).
func (s *Service) ReferencedByResources(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
	return s.repo.ReferencedByResources(ctx, kind, id)
}

// Create validates rec as a new kind record against the whole catalog
// before persisting it (design D9): rec is applied to an in-memory COPY of
// the current snapshot via domain.ApplyCatalogMutation, the copy's
// Validate() re-checks every structural invariant, and only when that
// succeeds does Create persist it — nothing persists on rejection, and the
// snapshot commits only once persistence itself also succeeds.
//
// Once WithCatalogAdminRepositoryV2 has wired repoV2 (the shipped production
// and integration composition, design "Composition and authority switch"),
// Create routes through the same committed-coherent-result V2 writer as
// CreateV2 — insertion needs no CAS expected revision, so this is the one
// legacy write method 4D can safely retarget without TUI-carried revision
// state. It falls back to the pre-4D legacy repo.Insert path only when
// repoV2 is unconfigured (every existing fixture that never calls
// WithCatalogAdminRepositoryV2).
func (s *Service) Create(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) (domain.CatalogRecord, error) {
	rec.Kind = kind

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.repoV2 != nil {
		if err := s.prepareV2Write(ctx); err != nil {
			return domain.CatalogRecord{}, err
		}
		return s.insertLocked(ctx, rec)
	}

	rec = s.canonicalizeCodes(rec)
	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: domain.OpInsert, Record: rec})
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	if err := next.Validate(); err != nil {
		return domain.CatalogRecord{}, domain.WrapInvalidCatalog(err)
	}

	id, err := s.repo.Insert(ctx, rec)
	if err != nil {
		return domain.CatalogRecord{}, err
	}

	s.publish(next)
	rec.ID = id
	return rec, nil
}

// Update validates rec's changes against the whole catalog before
// persisting them (design D9), and enforces Código immutability's
// authoritative, service-level layer (design D11 layer 1): for every field
// marked domain.ImmutableOnceReferenced (every "código" field), a changed
// value is rejected with domain.ErrCodeImmutable when
// repo.ReferencedByResources reports the record is already in use by at
// least one resource — checked, and rejected, BEFORE calling repo.Update.
//
// When a código change is allowed, the current record identifies the element
// while rec supplies the replacement published after persistence.
func (s *Service) Update(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) (domain.CatalogRecord, error) {
	if rec.ID == 0 {
		return domain.CatalogRecord{}, ErrInvalidArgument
	}
	rec.Kind = kind

	s.mu.Lock()
	defer s.mu.Unlock()

	rec = s.canonicalizeCodes(rec)

	current, err := s.repo.Get(ctx, kind, rec.ID)
	if err != nil {
		return domain.CatalogRecord{}, err
	}

	def, ok := s.registry.Kind(kind)
	if !ok {
		return domain.CatalogRecord{}, domain.ErrCatalogKindUnknown
	}

	mutationRecord := rec
	mutationRecord.Values = cloneCatalogValues(rec.Values)
	for _, field := range def.Fields {
		if field.Immutable != domain.ImmutableOnceReferenced {
			continue
		}
		oldValue, hasOld := current.Values[field.Name]
		newValue, hasNew := rec.Values[field.Name]
		if !hasOld {
			continue
		}
		if hasNew && oldValue.Text != newValue.Text {
			referenced, err := s.repo.ReferencedByResources(ctx, kind, rec.ID)
			if err != nil {
				return domain.CatalogRecord{}, err
			}
			if referenced {
				return domain.CatalogRecord{}, domain.ErrCodeImmutable
			}
		}
		// Freeze to the current value for the snapshot mutation lookup — see
		// the method doc comment above.
		mutationRecord.Values[field.Name] = oldValue
	}

	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: domain.OpUpdate, Record: mutationRecord, Replacement: &rec})
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	if err := next.Validate(); err != nil {
		return domain.CatalogRecord{}, domain.WrapInvalidCatalog(err)
	}

	if err := s.repo.Update(ctx, rec); err != nil {
		return domain.CatalogRecord{}, err
	}

	s.publish(next)
	return rec, nil
}

// Deactivate soft-deletes kind's record id (design §1a/Risk#3): validated
// and persisted the same validate-before-persist way as Create/Update.
// Returns domain.ErrSoftDeleteUnsupported, unchanged, for a kind whose
// underlying ResourceCatalog struct has no Go Active field yet (see
// domain.CatalogKind.SoftDelete's doc comment) — nothing is persisted.
func (s *Service) Deactivate(ctx context.Context, kind domain.CatalogKindCode, id int64) error {
	return s.setActive(ctx, kind, id, false)
}

// Reactivate restores kind's record id — see Deactivate's doc comment.
func (s *Service) Reactivate(ctx context.Context, kind domain.CatalogKindCode, id int64) error {
	return s.setActive(ctx, kind, id, true)
}

func (s *Service) setActive(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool) error {
	if id == 0 {
		return ErrInvalidArgument
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.repo.Get(ctx, kind, id)
	if err != nil {
		return err
	}

	op := domain.OpDeactivate
	if active {
		op = domain.OpReactivate
	}
	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: op, Record: current})
	if err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return domain.WrapInvalidCatalog(err)
	}

	if err := s.repo.SetActive(ctx, kind, id, active); err != nil {
		return err
	}

	s.publish(next)
	return nil
}

// Delete permanently removes kind's record id only after the authoritative
// service guards have approved it. The current persisted record is loaded
// first, then active targets, every non-zero dependency count, and every
// resource reference are rejected before a private candidate is materialized
// and validated. The candidate is published only after repository deletion
// succeeds; consumer-side preflight is never used as authorization.
func (s *Service) Delete(ctx context.Context, kind domain.CatalogKindCode, id int64) error {
	if id == 0 {
		return ErrInvalidArgument
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.repo.Get(ctx, kind, id)
	if err != nil {
		return err
	}
	// Hard delete of an active record is a lifecycle violation (design
	// "Internal Core outcomes and mapping"), not a request-level rejection.
	if current.Active {
		return domain.ErrInvalidLifecycle
	}

	dependencies, err := s.repo.Dependents(ctx, kind, id)
	if err != nil {
		return err
	}
	for _, dependency := range dependencies {
		if dependency.Count != 0 {
			return domain.ErrCatalogInUse
		}
	}

	referenced, err := s.repo.ReferencedByResources(ctx, kind, id)
	if err != nil {
		return err
	}
	if referenced {
		return domain.ErrCatalogInUse
	}

	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: domain.OpDelete, Record: current})
	if err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return domain.WrapInvalidCatalog(err)
	}

	if err := s.repo.Delete(ctx, kind, id); err != nil {
		return err
	}

	s.publish(next)
	return nil
}

func (s *Service) publish(next domain.ResourceCatalog) {
	s.snapshot = next
	s.authority.Publish(next)
}

// --- V2 additive path (design "Catalog transaction and coherent
// publication"): coexists with, never replaces, the legacy methods above;
// production composition stays on the legacy path until 4D.

// WithCatalogAdminRepositoryV2 additively wires repoV2 into s and returns s,
// enabling the V2 methods below without touching legacy behavior.
func (s *Service) WithCatalogAdminRepositoryV2(repoV2 domain.CatalogAdminRepositoryV2) *Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repoV2 = repoV2
	return s
}

// CatalogAdminRepositoryV2Configured reports whether repoV2 is wired, so a
// DB-less composition test (design "Composition and authority switch") can
// assert that a production/integration composition root owns the V2 write
// authority without a real database.
func (s *Service) CatalogAdminRepositoryV2Configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repoV2 != nil
}

// ResetCatalogWriterAvailability clears a writer-unavailable latch after an
// operator performs the coherent reload/restart design.md requires.
func (s *Service) ResetCatalogWriterAvailability() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writerUnavailable = false
}

// prepareV2Write rejects a call before any candidate work when the writer is
// latched or unconfigured. Callers must already hold s.mu.
func (s *Service) prepareV2Write(ctx context.Context) error {
	if s.writerUnavailable {
		err := ErrCatalogWriterUnavailable
		core.Record(ctx, core.Operation("catalogo.v2write"), "", 0, err)
		return err
	}
	if s.repoV2 == nil {
		err := ErrCatalogAdminRepositoryV2Unavailable
		core.Record(ctx, core.Operation("catalogo.v2write"), "", 0, err)
		return err
	}
	return nil
}

// classifyV2WriteError latches the writer unavailable on
// ErrCatalogWriteIndeterminate; either way nothing publishes. Callers must
// already hold s.mu.
func (s *Service) classifyV2WriteError(ctx context.Context, kind domain.CatalogKindCode, id int64, err error) error {
	core.Record(ctx, core.Operation("catalogo.v2write"), string(kind), id, err)
	if errors.Is(err, ErrCatalogWriteIndeterminate) {
		s.writerUnavailable = true
		return ErrCatalogWriterUnavailable
	}
	return err
}

// publishCoherent publishes the repository's own committed result — never a
// hand-built candidate. Callers must hold s.mu and call this only on success.
func (s *Service) publishCoherent(catalog domain.ResourceCatalog) {
	s.snapshot = catalog
	s.authority.Publish(catalog)
}

// buildV2UpdateCandidate mirrors Update's D11 layer-1 guard and validates
// the resulting candidate — used only for pre-repository validation, never
// published. Callers must already hold s.mu.
func (s *Service) buildV2UpdateCandidate(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) error {
	current, err := s.repo.Get(ctx, kind, rec.ID)
	if err != nil {
		return err
	}
	def, ok := s.registry.Kind(kind)
	if !ok {
		return domain.ErrCatalogKindUnknown
	}
	mutationRecord := rec
	mutationRecord.Values = cloneCatalogValues(rec.Values)
	for _, field := range def.Fields {
		if field.Immutable != domain.ImmutableOnceReferenced {
			continue
		}
		oldValue, hasOld := current.Values[field.Name]
		newValue, hasNew := rec.Values[field.Name]
		if !hasOld {
			continue
		}
		if hasNew && oldValue.Text != newValue.Text {
			referenced, err := s.repo.ReferencedByResources(ctx, kind, rec.ID)
			if err != nil {
				return err
			}
			if referenced {
				return domain.ErrCodeImmutable
			}
		}
		mutationRecord.Values[field.Name] = oldValue
	}
	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: domain.OpUpdate, Record: mutationRecord, Replacement: &rec})
	if err != nil {
		return err
	}
	return domain.WrapInvalidCatalog(next.Validate())
}

// buildV2DeleteCandidate mirrors Delete's conservative guard chain (design
// "Conservative hard delete") and validates the resulting candidate — never
// published. Callers must already hold s.mu.
func (s *Service) buildV2DeleteCandidate(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
	current, err := s.repo.Get(ctx, kind, id)
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	// Hard delete of an active record is a lifecycle violation — see
	// Delete's doc comment.
	if current.Active {
		return domain.CatalogRecord{}, domain.ErrInvalidLifecycle
	}
	dependencies, err := s.repo.Dependents(ctx, kind, id)
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	for _, dependency := range dependencies {
		if dependency.Count != 0 {
			return domain.CatalogRecord{}, domain.ErrCatalogInUse
		}
	}
	referenced, err := s.repo.ReferencedByResources(ctx, kind, id)
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	if referenced {
		return domain.CatalogRecord{}, domain.ErrCatalogInUse
	}
	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: domain.OpDelete, Record: current})
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	if err := next.Validate(); err != nil {
		return domain.CatalogRecord{}, domain.WrapInvalidCatalog(err)
	}
	return current, nil
}

// CreateV2 is Create's additive V2 sibling: validates a private candidate
// but publishes only the repository's coherent CatalogWriteResult.Catalog.
func (s *Service) CreateV2(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) (domain.CatalogRecord, error) {
	rec.Kind = kind

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.prepareV2Write(ctx); err != nil {
		return domain.CatalogRecord{}, err
	}
	return s.insertLocked(ctx, rec)
}

// insertLocked performs the shared V2 create write behind both Create (once
// repoV2 is configured) and CreateV2: validate a private candidate, insert
// through repoV2, and publish only its committed coherent result. Callers
// must already hold s.mu and have already called prepareV2Write.
func (s *Service) insertLocked(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogRecord, error) {
	rec = s.canonicalizeCodes(rec)
	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: domain.OpInsert, Record: rec})
	if err != nil {
		return domain.CatalogRecord{}, err
	}
	if err := next.Validate(); err != nil {
		return domain.CatalogRecord{}, domain.WrapInvalidCatalog(err)
	}

	result, err := s.repoV2.Insert(ctx, rec)
	if err != nil {
		return domain.CatalogRecord{}, s.classifyV2WriteError(ctx, rec.Kind, 0, err)
	}

	s.publishCoherent(result.Catalog)
	if result.Record != nil {
		return *result.Record, nil
	}
	return rec, nil
}

// UpdateRevision is Update's CAS-aware additive sibling.
func (s *Service) UpdateRevision(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogRecord, error) {
	if rec.ID == 0 || expectedRevision == 0 {
		return domain.CatalogRecord{}, ErrInvalidArgument
	}
	rec.Kind = kind

	s.mu.Lock()
	defer s.mu.Unlock()

	rec = s.canonicalizeCodes(rec)

	if err := s.prepareV2Write(ctx); err != nil {
		return domain.CatalogRecord{}, err
	}
	if err := s.buildV2UpdateCandidate(ctx, kind, rec); err != nil {
		return domain.CatalogRecord{}, err
	}

	result, err := s.repoV2.Update(ctx, rec, expectedRevision)
	if err != nil {
		return domain.CatalogRecord{}, s.classifyV2WriteError(ctx, kind, rec.ID, err)
	}

	s.publishCoherent(result.Catalog)
	if result.Record != nil {
		return *result.Record, nil
	}
	return rec, nil
}

// setActiveRevision backs DeactivateRevision/ReactivateRevision. An
// already-current active state is an idempotent no-op: no repository call,
// no publish (domain.CatalogAdminRepositoryV2.SetActive's doc comment).
func (s *Service) setActiveRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool, expectedRevision uint64) error {
	if id == 0 || expectedRevision == 0 {
		return ErrInvalidArgument
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.prepareV2Write(ctx); err != nil {
		return err
	}

	current, err := s.repo.Get(ctx, kind, id)
	if err != nil {
		return err
	}
	if current.Active == active {
		return nil
	}

	op := domain.OpDeactivate
	if active {
		op = domain.OpReactivate
	}
	next, err := domain.ApplyCatalogMutation(s.snapshot, s.registry, domain.CatalogMutation{Op: op, Record: current})
	if err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return domain.WrapInvalidCatalog(err)
	}

	result, err := s.repoV2.SetActive(ctx, kind, id, active, expectedRevision)
	if err != nil {
		return s.classifyV2WriteError(ctx, kind, id, err)
	}

	s.publishCoherent(result.Catalog)
	return nil
}

// DeactivateRevision is Deactivate's CAS-aware additive sibling.
func (s *Service) DeactivateRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) error {
	return s.setActiveRevision(ctx, kind, id, false, expectedRevision)
}

// ReactivateRevision is Reactivate's CAS-aware additive sibling.
func (s *Service) ReactivateRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) error {
	return s.setActiveRevision(ctx, kind, id, true, expectedRevision)
}

// HardDeleteRevision is Delete's CAS-aware additive sibling.
func (s *Service) HardDeleteRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) error {
	if id == 0 || expectedRevision == 0 {
		return ErrInvalidArgument
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.prepareV2Write(ctx); err != nil {
		return err
	}
	if _, err := s.buildV2DeleteCandidate(ctx, kind, id); err != nil {
		return err
	}

	result, err := s.repoV2.Delete(ctx, kind, id, expectedRevision)
	if err != nil {
		return s.classifyV2WriteError(ctx, kind, id, err)
	}

	s.publishCoherent(result.Catalog)
	return nil
}

// canonicalizeCodes canonicalizes rec's FieldCode values per its registered
// kind, so every write path persists the same canonical form it validates.
// An unknown kind is left untouched: the downstream mutation/validation call
// reports domain.ErrCatalogKindUnknown as before.
func (s *Service) canonicalizeCodes(rec domain.CatalogRecord) domain.CatalogRecord {
	if def, ok := s.registry.Kind(rec.Kind); ok {
		rec = domain.CanonicalizeCatalogRecordCodes(def, rec)
	}
	return rec
}

// cloneCatalogValues returns a shallow copy of values — Update must never
// mutate the caller's own rec.Values map in place, since rec itself (with
// its true, unfrozen values) is still needed for the later repo.Update call.
func cloneCatalogValues(values map[string]domain.CatalogValue) map[string]domain.CatalogValue {
	out := make(map[string]domain.CatalogValue, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
