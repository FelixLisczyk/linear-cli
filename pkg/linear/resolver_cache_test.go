package linear

import (
	"testing"
	"time"

	"github.com/joa23/linear-cli/pkg/linear/core"
)

func TestResolverCacheStoresNormalizedLabelMetadata(t *testing.T) {
	cache := newResolverCache(time.Hour)
	defer cache.clear()

	parent := &core.LabelRef{ID: "parent-1", Name: "Issue Type"}
	cache.setLabels("TEAM-1", []core.Label{{ID: "LABEL-1", Name: "Tests", Parent: parent}})

	id, ok := cache.getLabelByName("team-1", " TESTS ")
	if !ok || id != "LABEL-1" {
		t.Fatalf("cached name = %q, %v; want LABEL-1, true", id, ok)
	}
	label, ok := cache.getLabelByID("team-1", "label-1")
	if !ok || label.Parent == nil || label.Parent.Name != "Issue Type" {
		t.Fatalf("cached metadata = %#v, %v", label, ok)
	}
	label.Parent.Name = "mutated"
	again, ok := cache.getLabelByID("team-1", "LABEL-1")
	if !ok || again.Parent.Name != "Issue Type" {
		t.Fatalf("cache returned mutable metadata: %#v, %v", again, ok)
	}
}

func TestResolverCacheDoesNotCacheAmbiguousLabelNames(t *testing.T) {
	cache := newResolverCache(time.Hour)
	defer cache.clear()
	cache.setLabels("team", []core.Label{
		{ID: "one", Name: "Duplicate"},
		{ID: "two", Name: "duplicate"},
	})
	if _, ok := cache.getLabelByName("TEAM", "duplicate"); ok {
		t.Fatal("ambiguous label name should not be cached")
	}
}
