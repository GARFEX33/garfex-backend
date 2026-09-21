package postgres

import (
	"strings"
	"testing"
)

func TestResolutionOverridePersistenceUsesCASAndAppendOnlyAudit(t *testing.T) {
	for _, want := range []string{
		"purchase_line_resolution_audit",
		"previous_override",
		"new_override",
		"previous_revision",
		"new_revision",
		"decided_at",
	} {
		if !strings.Contains(insertResolutionAuditSQL, want) {
			t.Errorf("audit SQL missing %q", want)
		}
	}
	if !strings.Contains(getPurchaseLineForResolveSQL, "WHERE pl.id = $1") {
		t.Fatal("line resolution read must address one line")
	}
}
