package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// pgxLikeError simulates a PostgreSQL driver error. The mapper MUST NOT
// inspect these fields; they belong to the diagnostic sink only.
type pgxLikeError struct {
	SQLState, ConstraintName, TableName, ColumnName, ServerMessage string
}

func (e pgxLikeError) Error() string { return e.ServerMessage }

func TestMapPrecedenceAndCategories(t *testing.T) {
	pgxErr := pgxLikeError{"23505", "resource_classes_code_key", "resource_classes", "code", "duplicate key value violates unique constraint"}
	tests := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{"nil", nil, ""},
		{"revision conflict", domain.ErrRevisionConflict, Conflict},
		{"attribute order revision conflict", domain.ErrAttributeOrderRevisionConflict, Conflict},
		{"identity conflict", domain.ErrIdentityConflict, IdentityConflict},
		{"identity over integrity", fmt.Errorf("ctx: %w", domain.ErrIdentityConflict), IdentityConflict},
		{"reactivation over validation", domain.WrapReactivationImpossible(domain.ErrResourceValidation), ReactivationImpossible},
		{"reactivation over reference", domain.WrapReactivationImpossible(domain.ErrResourceReference), ReactivationImpossible},
		{"invalid catalog over validation", domain.WrapInvalidCatalog(domain.ErrResourceValidation), InvalidCatalog},
		{"invalid lifecycle", domain.ErrInvalidLifecycle, InvalidLifecycle},
		{"integrity", domain.ErrResourceIntegrity, Integrity},
		{"in use", domain.ErrCatalogInUse, InUse},
		{"immutable code", domain.ErrCodeImmutable, ImmutableCode},
		{"validation", domain.ErrResourceValidation, Validation},
		{"catalog duplicate", domain.ErrCatalogDuplicate, Duplicate},
		{"resource duplicate", domain.ErrDuplicateResource, Duplicate},
		{"catalog invalid reference", domain.ErrCatalogReference, InvalidReference},
		{"resource invalid reference", domain.ErrResourceReference, InvalidReference},
		{"catalog not found", domain.ErrCatalogRecordNotFound, NotFound},
		{"resource not found", domain.ErrResourceNotFound, NotFound},
		{"invalid argument sentinel", ErrInvalidArgument, InvalidArgument},
		{"unavailable sentinel", ErrUnavailable, Unavailable},
		{"attribute order unavailable", domain.ErrAttributeOrderUnavailable, Unavailable},
		{"context canceled", context.Canceled, Unavailable},
		{"context deadline exceeded", context.DeadlineExceeded, Unavailable},
		{"unknown error", errors.New("boom"), Internal},
		{"pgx-like driver error", pgxErr, Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Map(tt.err)
			if got.Code() != tt.want {
				t.Fatalf("Map.Code() = %q, want %q", got.Code(), tt.want)
			}
			if !IsCode(tt.err, tt.want) {
				t.Fatalf("IsCode() = false for %q", tt.want)
			}
			if Code(tt.err) != tt.want {
				t.Fatalf("Code() = %q, want %q", Code(tt.err), tt.want)
			}
		})
	}
}

func TestErrorHasNoUnwrapAndSafeFormatting(t *testing.T) {
	driver := pgxLikeError{"23503", "fk_resource_family_class_id", "resource_families", "class_id", "insert or update violates foreign key"}
	got := Map(driver)
	if errors.Unwrap(got) != nil {
		t.Fatalf("neutral error must not unwrap, got %v", errors.Unwrap(got))
	}
	for _, repr := range []string{fmt.Sprintf("%v", got), fmt.Sprintf("%+v", got)} {
		for _, leak := range []string{driver.SQLState, driver.ConstraintName, driver.TableName, driver.ColumnName, driver.ServerMessage} {
			if strings.Contains(repr, leak) {
				t.Fatalf("format %q leaked %q", repr, leak)
			}
		}
	}
}

func TestCodeThroughRecursiveChain(t *testing.T) {
	inner := New(Conflict, "stale revision")
	outer := fmt.Errorf("service call failed: %w", inner)
	if Code(outer) != Conflict || !IsCode(outer, Conflict) {
		t.Fatalf("Code/IsCode through wrapper failed")
	}
	if Code(errors.Unwrap(outer)) != Conflict {
		t.Fatalf("Code of unwrapped neutral error failed")
	}
}

type spySink struct{ records []DiagnosticRecord }

func (s *spySink) Record(ctx context.Context, op Operation, kind string, id int64, cause error) {
	s.records = append(s.records, NewDiagnosticRecord(ctx, op, kind, id, cause))
}

func TestActor_WithActor_ActorFrom_RoundTrip(t *testing.T) {
	if got := ActorFrom(context.Background()); got != "" {
		t.Fatalf("expected empty actor on bare context, got %q", got)
	}
	ctx := WithActor(context.Background(), "PI")
	if got := ActorFrom(ctx); got != "PI" {
		t.Fatalf("ActorFrom() = %q, want %q", got, "PI")
	}
	if got := WithActor(context.Background(), ""); ActorFrom(got) != "" {
		t.Fatalf("blank actor must not attach to the context")
	}
}

func TestActor_NewDiagnosticRecord_IncludesActorAndBlankNoop(t *testing.T) {
	ctx := WithActor(context.Background(), "PI")
	rec := NewDiagnosticRecord(ctx, Operation("catalog.create"), "CLASE", 7, errors.New("boom"))
	if rec.Actor != "PI" || rec.Op != "catalog.create" || rec.Kind != "CLASE" || rec.ID != 7 {
		t.Fatalf("unexpected diagnostic record: %+v", rec)
	}
	blank := NewDiagnosticRecord(context.Background(), Operation("catalog.create"), "", 0, errors.New("boom"))
	if blank.Actor != "" {
		t.Fatalf("expected empty actor when context carries none, got %q", blank.Actor)
	}
}

func TestDiagnosticSinkRetainsCauseWithoutLeaking(t *testing.T) {
	spy := &spySink{}
	SetSink(spy)
	defer SetSink(nil)
	driver := pgxLikeError{"40001", "", "resource_classes", "", "could not serialize access"}
	got := MapWithDiagnostic(context.Background(), Operation("resource.create"), driver)
	if got.Code() != Internal {
		t.Fatalf("MapWithDiagnostic.Code() = %q, want %q", got.Code(), Internal)
	}
	if len(spy.records) != 1 || spy.records[0].Cause != driver || spy.records[0].Op != "resource.create" {
		t.Fatalf("diagnostic record not retained correctly: %+v", spy.records)
	}
	if strings.Contains(got.Error(), driver.ServerMessage) {
		t.Fatalf("neutral error leaked server message")
	}
}

func TestFifteenCategoriesAreDistinct(t *testing.T) {
	codes := []ErrorCode{InvalidArgument, NotFound, Duplicate, InvalidReference, Validation, Integrity, IdentityConflict, InvalidLifecycle, ReactivationImpossible, InvalidCatalog, InUse, ImmutableCode, Conflict, Unavailable, Internal}
	seen := make(map[ErrorCode]struct{}, len(codes))
	for _, c := range codes {
		if _, ok := seen[c]; ok {
			t.Fatalf("duplicate code %q", c)
		}
		seen[c] = struct{}{}
	}
}
