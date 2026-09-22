package recursos

import (
	"context"
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

func writeCommand() domain.CreateCommand {
	return domain.CreateCommand{Scope: domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}, NaturalUnit: "M", Attributes: []domain.ResourceAttributeValue{domain.OptionValue("voltage", "600 V"), domain.OptionValue("insulation", "THW"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("conductor_material", "COBRE")}}
}
func TestServiceRejectsInvalidIntentBeforeRepository(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.CreateCommand)
	}{
		{"family", func(c *domain.CreateCommand) { c.Scope.FamilyCode = "FORGED" }},
		{"type", func(c *domain.CreateCommand) { c.Scope.TypeCode = "FORGED" }},
		{"unit", func(c *domain.CreateCommand) { c.NaturalUnit = "FORGED" }},
		{"attribute", func(c *domain.CreateCommand) { c.Attributes[0] = domain.OptionValue("FORGED", "VALUE") }},
		{"relation", func(c *domain.CreateCommand) {
			c.Scope = domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CANALIZACIONES", TypeCode: "TUBERIA"}
			c.NaturalUnit = "PZA"
			c.Attributes = []domain.ResourceAttributeValue{domain.OptionValue("tipo", "CONDUIT PARED DELGADA"), domain.OptionValue("diameter_inch", `3/4"`), domain.OptionValue("diameter_mm", "13 mm")}
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cmd := writeCommand()
			tt.mutate(&cmd)
			repo := &fakeRepo{}
			_, err := NewService(repo, domain.SeedResourceCatalog()).Create(context.Background(), cmd)
			if !errors.Is(err, domain.ErrResourceValidation) {
				t.Fatalf("Create() error = %v", err)
			}
			if repo.gotCreate.IdentityKey != "" {
				t.Fatalf("repository received %#v", repo.gotCreate)
			}
		})
	}
}

func TestServiceCanonicalWritesAndStableUpdateID(t *testing.T) {
	repo := &fakeRepo{}
	created, err := NewService(repo, domain.SeedResourceCatalog()).Create(context.Background(), writeCommand())
	if err != nil || created.IdentityKey[:3] != "v1|" || created.IdentityKey != repo.gotCreate.IdentityKey {
		t.Fatalf("Create() = %#v, error %v", created, err)
	}
	update := domain.UpdateCommand{ID: 77, Scope: writeCommand().Scope, NaturalUnit: "M", Attributes: writeCommand().Attributes}
	updated, err := NewService(repo, domain.SeedResourceCatalog()).Update(context.Background(), update)
	if err != nil || updated.ID != 77 || repo.gotUpdate.ID != 77 {
		t.Fatalf("Update() = %#v, repository = %#v, error %v", updated, repo.gotUpdate, err)
	}
}

func TestServiceRejectsInactiveCatalogBeforeCreateOrUpdate(t *testing.T) {
	catalog := domain.SeedResourceCatalog()
	catalog.Families[0].Active = false
	repo := &fakeRepo{}
	service := NewService(repo, catalog)
	if _, err := service.Create(context.Background(), writeCommand()); !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("Create() error = %v, want ErrResourceReference", err)
	}
	if repo.gotCreate.IdentityKey != "" {
		t.Fatalf("Create() called repository with %#v", repo.gotCreate)
	}
	if _, err := service.Update(context.Background(), domain.UpdateCommand{ID: 42, Scope: writeCommand().Scope, NaturalUnit: "M", Attributes: writeCommand().Attributes}); !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("Update() error = %v, want ErrResourceReference", err)
	}
	if repo.gotUpdate.IdentityKey != "" {
		t.Fatalf("Update() called repository with %#v", repo.gotUpdate)
	}
}
