package postgres

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

func TestChildListQueryContracts(t *testing.T) {
	active := true
	branchID := int64(17)

	tests := []struct {
		name      string
		query     func() (string, []any)
		wantArgs  []any
		wantParts []string
		unwanted  []string
	}{
		{
			name: "branch text status offset and lookahead",
			query: func() (string, []any) {
				return buildListBranchesQuery(42, domain.ListCriteria{
					Text:   "North",
					Active: &active,
					Limit:  25,
					Offset: 50,
				})
			},
			wantArgs: []any{int64(42), true, "North", 26, 50},
			wantParts: []string{
				"supplier_id = $1",
				"active = $2",
				"name ILIKE '%' || $3 || '%'",
				"branch_reference ILIKE '%' || $3 || '%'",
				"city ILIKE '%' || $3 || '%'",
				"state ILIKE '%' || $3 || '%'",
				"country ILIKE '%' || $3 || '%'",
				"address ILIKE '%' || $3 || '%'",
				"general_phone ILIKE '%' || $3 || '%'",
				"general_email ILIKE '%' || $3 || '%'",
				"notes ILIKE '%' || $3 || '%'",
				"ORDER BY lower(city), lower(name), id",
				"LIMIT $4 OFFSET $5",
			},
		},
		{
			name: "contact supplier scope optional branch and empty text",
			query: func() (string, []any) {
				return buildListContactsQuery(42, domain.ContactListCriteria{
					Text:     "Ada",
					Active:   nil,
					BranchID: &branchID,
					Limit:    25,
					Offset:   75,
				})
			},
			wantArgs: []any{int64(42), nil, &branchID, "Ada", 26, 75},
			wantParts: []string{
				"supplier_id = $1",
				"active = $2",
				"branch_id = $3",
				"name ILIKE '%' || $4 || '%'",
				"role ILIKE '%' || $4 || '%'",
				"phone ILIKE '%' || $4 || '%'",
				"mobile ILIKE '%' || $4 || '%'",
				"email ILIKE '%' || $4 || '%'",
				"notes ILIKE '%' || $4 || '%'",
				"ORDER BY lower(name), id",
				"LIMIT $5 OFFSET $6",
			},
		},
		{
			name: "branch defaults preserve empty text predicate and offset floor",
			query: func() (string, []any) {
				return buildListBranchesQuery(5, domain.ListCriteria{Offset: -1})
			},
			wantArgs: []any{int64(5), nil, "", 101, 0},
			wantParts: []string{
				"supplier_id = $1",
				"($2::BOOLEAN IS NULL OR active = $2)",
				"($3 = '' OR",
				"ORDER BY lower(city), lower(name), id",
				"LIMIT $4 OFFSET $5",
			},
		},
		{
			name: "contact supplier scope without branch",
			query: func() (string, []any) {
				return buildListContactsQuery(9, domain.ContactListCriteria{Text: "mail", Limit: 10})
			},
			wantArgs: []any{int64(9), nil, (*int64)(nil), "mail", 11, 0},
			wantParts: []string{
				"supplier_id = $1",
				"($3::BIGINT IS NULL OR branch_id = $3)",
				"($4 = '' OR",
				"ORDER BY lower(name), id",
				"LIMIT $5 OFFSET $6",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args := tt.query()
			for _, part := range tt.wantParts {
				if !strings.Contains(query, part) {
					t.Errorf("query missing %q: %s", part, query)
				}
			}
			for _, part := range tt.unwanted {
				if strings.Contains(query, part) {
					t.Errorf("query unexpectedly contains %q: %s", part, query)
				}
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args length = %d, want %d: %#v", len(args), len(tt.wantArgs), args)
			}
			for i := range args {
				if !reflect.DeepEqual(args[i], tt.wantArgs[i]) {
					t.Errorf("args[%d] = %#v, want %#v", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestChildListRepositoriesReturnErrorsWithoutPool(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "branches",
			call: func() error {
				_, err := (&repository{}).ListBranches(context.Background(), 42, domain.ListCriteria{Text: "North"})
				return err
			},
		},
		{
			name: "contacts",
			call: func() error {
				_, err := (&repository{}).ListContacts(context.Background(), 42, domain.ContactListCriteria{Text: "Ada"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected nil-pool error")
			}
		})
	}
}
