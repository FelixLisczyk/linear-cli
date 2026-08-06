package service

import (
	"strings"
	"testing"

	"github.com/joa23/linear-cli/pkg/linear/core"
)

func TestValidateLabelSelectionReportsDeterministicSiblingConflicts(t *testing.T) {
	labels := []core.Label{
		{ID: "b", Name: "Beta", Parent: &core.LabelRef{ID: "group-2", Name: "Review"}},
		{ID: "a", Name: "A, quoted", Parent: &core.LabelRef{ID: "group-1", Name: "Issue Type"}},
		{ID: "c", Name: "Alpha", Parent: &core.LabelRef{ID: "group-2", Name: "Review"}},
		{ID: "d", Name: "Tests", Parent: &core.LabelRef{ID: "group-1", Name: "Issue Type"}},
		{ID: "standalone", Name: "Standalone"},
		{ID: "d", Name: "Tests", Parent: &core.LabelRef{ID: "group-1", Name: "Issue Type"}},
	}

	err := validateLabelSelection(labels)
	if err == nil {
		t.Fatal("validateLabelSelection returned nil for conflicting siblings")
	}
	message := err.Error()
	if !strings.Contains(message, `"A, quoted"`) || !strings.Contains(message, `"Tests"`) || !strings.Contains(message, `"Issue Type"`) {
		t.Fatalf("conflict message = %q, want labels and group", message)
	}
	if strings.Index(message, `"Issue Type"`) > strings.Index(message, `"Review"`) {
		t.Fatalf("groups are not deterministic: %q", message)
	}
}

func TestValidateLabelSelectionAllowsStandaloneAndDistinctGroups(t *testing.T) {
	labels := []core.Label{
		{ID: "a", Name: "Alpha", Parent: &core.LabelRef{ID: "group-1", Name: "One"}},
		{ID: "b", Name: "Beta", Parent: &core.LabelRef{ID: "group-2", Name: "Two"}},
		{ID: "c", Name: "Standalone"},
	}
	if err := validateLabelSelection(labels); err != nil {
		t.Fatalf("unexpected conflict: %v", err)
	}
}
