package projects

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/joa23/linear-cli/pkg/linear/core"
	"github.com/joa23/linear-cli/pkg/linear/testutil"
)

func TestNormalizeStatusNames(t *testing.T) {
	got, err := NormalizeStatusNames([]string{" In Progress ", "on hold", "IN PROGRESS"})
	if err != nil {
		t.Fatalf("NormalizeStatusNames() error = %v", err)
	}
	want := []string{"In Progress", "on hold"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("NormalizeStatusNames() = %#v, want %#v", got, want)
	}

	_, err = NormalizeStatusNames([]string{"In Progress", ""})
	if err == nil || !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("expected empty-value error, got %v", err)
	}
}

func TestProjectUnmarshalStatusPreservesStateAlias(t *testing.T) {
	var project core.Project
	err := json.Unmarshal([]byte(`{"id":"p1","state":"started","status":{"id":"s1","name":"On Hold","type":"started"}}`), &project)
	if err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if project.Status == nil || project.Status.Name != "On Hold" {
		t.Fatalf("status = %#v, want named status", project.Status)
	}
	if project.State != "started" {
		t.Fatalf("state = %q, want started", project.State)
	}
	if project.StatusName() != "On Hold" {
		t.Fatalf("StatusName() = %q, want On Hold", project.StatusName())
	}
}

func TestResolveProjectStatusNamesIgnoresArchivedAndRejectsUnknown(t *testing.T) {
	archived := "2026-01-01T00:00:00Z"
	body := testutil.NewGraphQLDataResponse(map[string]interface{}{
		"organization": map[string]interface{}{
			"projectStatuses": []map[string]interface{}{
				{"id": "active", "name": "In Progress", "type": "started"},
				{"id": "archived", "name": "Old", "type": "started", "archivedAt": archived},
			},
		},
	})
	base := core.NewBaseClient("test-token")
	base.SetHTTPClient(testHTTPClient(body))
	client := NewClient(base)

	ids, err := client.ResolveProjectStatusNames([]string{" in progress "})
	if err != nil || len(ids) != 1 || ids[0] != "active" {
		t.Fatalf("ResolveProjectStatusNames() = %#v, %v", ids, err)
	}
	base = core.NewBaseClient("test-token")
	base.SetHTTPClient(testHTTPClient(body))
	client = NewClient(base)
	_, err = client.ResolveProjectStatusNames([]string{"Old"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected archived status to be unavailable, got %v", err)
	}
}

func testHTTPClient(body interface{}) *http.Client {
	return &http.Client{Transport: testutil.NewSuccessTransport(body)}
}
