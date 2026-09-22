package recursos

import (
	"context"
	"errors"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	"reflect"
	"testing"
)

func TestServiceSearchValidatesAndPropagatesPageContract(t *testing.T) {
	repositoryError := errors.New("connection lost")
	criteria := domain.SearchCriteria{Text: "cable", LifecycleScope: domain.LifecycleScopeInactive, Limit: 10, Offset: 10}
	repo := &fakeRepo{page: domain.ResourcePage{Criteria: criteria, HasPrevious: true}}
	service := NewService(repo, domain.SeedResourceCatalog())
	got, err := service.SearchPage(context.Background(), criteria)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(repo.gotCriteria, criteria) || !reflect.DeepEqual(got.Criteria, criteria) {
		t.Fatalf("Search() criteria = repo %+v/page %+v, want %+v", repo.gotCriteria, got.Criteria, criteria)
	}
	repo.pageErr = repositoryError
	if _, err := service.SearchPage(context.Background(), criteria); !errors.Is(err, repositoryError) {
		t.Fatalf("Search() error = %v, want %v", err, repositoryError)
	}
	got, err = service.SearchPage(context.Background(), domain.SearchCriteria{Limit: domain.MaxSearchPageSize + 1})
	if err == nil || repo.gotCriteria.Limit != criteria.Limit {
		t.Fatalf("invalid page input error = %v, repository criteria = %+v; want validation before repository", err, repo.gotCriteria)
	}
}
