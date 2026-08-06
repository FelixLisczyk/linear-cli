package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/joa23/linear-cli/internal/format"
	"github.com/joa23/linear-cli/internal/service"
	"github.com/joa23/linear-cli/pkg/linear"
	"github.com/joa23/linear-cli/pkg/linear/core"
)

func TestIssueDescriptionFromExplicitStdinReachesService(t *testing.T) {
	issues := &recordingIssueService{}
	deps := &Dependencies{Issues: issues, Stdin: strings.NewReader("  issue body\n")}
	cmd := NewCmdWithDeps(deps, newIssuesCreateCmd)
	cmd.SetArgs([]string{"New issue", "--team", "CEN", "--description", "-"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues.created == nil || issues.created.Description != "issue body" {
		t.Fatalf("created description = %#v, want %q", issues.created, "issue body")
	}
}

func TestIssueUpdateDescriptionFromExplicitStdinReachesService(t *testing.T) {
	issues := &recordingIssueService{}
	deps := &Dependencies{Issues: issues, Stdin: strings.NewReader("  updated body\n")}
	cmd := NewCmdWithDeps(deps, newIssuesUpdateCmd)
	cmd.SetArgs([]string{"CEN-123", "--description", "-"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues.updated == nil || issues.updated.Description == nil || *issues.updated.Description != "updated body" {
		t.Fatalf("updated description = %#v, want %q", issues.updated, "updated body")
	}
}

func TestProjectDescriptionFromExplicitStdinReachesService(t *testing.T) {
	tests := []struct {
		name   string
		create bool
	}{
		{name: "create", create: true},
		{name: "update", create: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projects := &recordingProjectService{}
			deps := &Dependencies{Projects: projects, Stdin: strings.NewReader("  project body\n")}
			factory := newProjectsUpdateCmd
			args := []string{"PROJ-123", "--description", "-"}
			if tt.create {
				factory = newProjectsCreateCmd
				args = []string{"New project", "--team", "CEN", "--description", "-"}
			}

			cmd := NewCmdWithDeps(deps, factory)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.create {
				if projects.created == nil || projects.created.Description != "project body" {
					t.Fatalf("created description = %#v, want %q", projects.created, "project body")
				}
			} else if projects.updated == nil || projects.updated.Description == nil || *projects.updated.Description != "project body" {
				t.Fatalf("updated description = %#v, want %q", projects.updated, "project body")
			}
		})
	}
}

func TestReplyBodyFromExplicitStdinReachesService(t *testing.T) {
	issues := &recordingIssueService{}
	deps := &Dependencies{Issues: issues, Stdin: strings.NewReader("  reply body\n")}
	cmd := NewCmdWithDeps(deps, newIssuesReplyCmd)
	cmd.SetArgs([]string{"CEN-123", "comment-123", "--body", "-"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues.replyBody != "reply body" {
		t.Fatalf("reply body = %q, want %q", issues.replyBody, "reply body")
	}
}

func TestCommentBodyFromExplicitStdinReachesAPI(t *testing.T) {
	transport := &recordingCommentTransport{}
	client := linear.NewClient("test-token")
	client.GetBase().SetHTTPClient(&http.Client{Transport: transport})
	deps := &Dependencies{Client: client, Stdin: strings.NewReader("  comment body\n")}
	cmd := NewCmdWithDeps(deps, newIssuesCommentCmd)
	cmd.SetArgs([]string{"CEN-123", "--body", "-"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport.body != "comment body" {
		t.Fatalf("comment body = %q, want %q", transport.body, "comment body")
	}
}

func TestOrdinaryDescriptionDoesNotReadStdinAtCommandBoundary(t *testing.T) {
	issues := &recordingIssueService{}
	deps := &Dependencies{Issues: issues, Stdin: errorReader{err: fmt.Errorf("stdin should not be read")}}
	cmd := NewCmdWithDeps(deps, newIssuesCreateCmd)
	cmd.SetArgs([]string{"New issue", "--team", "CEN", "--description", "literal body"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues.created == nil || issues.created.Description != "literal body" {
		t.Fatalf("created description = %#v, want %q", issues.created, "literal body")
	}
}

func TestExplicitEmptyStdinPreservesExistingSemantics(t *testing.T) {
	issues := &recordingIssueService{}
	deps := &Dependencies{Issues: issues, Stdin: strings.NewReader("")}
	cmd := NewCmdWithDeps(deps, newIssuesUpdateCmd)
	cmd.SetArgs([]string{"CEN-123", "--description", "-"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues.updated == nil || issues.updated.Description != nil {
		t.Fatalf("empty update description = %#v, want nil", issues.updated)
	}
}

func TestExplicitStdinReadErrorsAreWrapped(t *testing.T) {
	issues := &recordingIssueService{}
	deps := &Dependencies{Issues: issues, Stdin: errorReader{err: fmt.Errorf("stdin failed")}}
	cmd := NewCmdWithDeps(deps, newIssuesCreateCmd)
	cmd.SetArgs([]string{"New issue", "--team", "CEN", "--description", "-"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "failed to read description: stdin failed") {
		t.Fatalf("got error %v, want contextual stdin error", err)
	}
	if issues.created != nil {
		t.Fatal("created issue after stdin read error")
	}
}

func TestEmptyStdinRejectsCommentAndReply(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(*recordingIssueService) bool
	}{
		{name: "comment", args: []string{"CEN-123", "--body", "-"}, check: func(issues *recordingIssueService) bool {
			return issues.replyBody == ""
		}},
		{name: "reply", args: []string{"CEN-123", "comment-123", "--body", "-"}, check: func(issues *recordingIssueService) bool {
			return issues.replyBody == ""
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := &recordingIssueService{}
			client := linear.NewClient("test-token")
			deps := &Dependencies{Client: client, Issues: issues, Stdin: strings.NewReader("")}
			factory := newIssuesReplyCmd
			if tt.name == "comment" {
				factory = newIssuesCommentCmd
			}
			cmd := NewCmdWithDeps(deps, factory)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected empty body error")
			}
			if !tt.check(issues) {
				t.Fatal("empty body reached service")
			}
		})
	}
}

type recordingIssueService struct {
	created   *service.CreateIssueInput
	updated   *service.UpdateIssueInput
	replyBody string
}

func (s *recordingIssueService) Get(string, format.Format) (string, error) { return "", nil }
func (s *recordingIssueService) GetWithOutput(string, format.Verbosity, format.OutputType) (string, error) {
	return "", nil
}
func (s *recordingIssueService) Search(*service.SearchFilters) (string, error) { return "", nil }
func (s *recordingIssueService) SearchWithOutput(*service.SearchFilters, format.Verbosity, format.OutputType) (string, error) {
	return "", nil
}
func (s *recordingIssueService) ListAssigned(int, format.Format) (string, error) { return "", nil }
func (s *recordingIssueService) ListAssignedWithPagination(*core.PaginationInput) (string, error) {
	return "", nil
}
func (s *recordingIssueService) Create(input *service.CreateIssueInput, _ format.OutputType) (string, error) {
	s.created = input
	return "created", nil
}
func (s *recordingIssueService) Update(_ string, input *service.UpdateIssueInput) (string, error) {
	s.updated = input
	return "updated", nil
}
func (s *recordingIssueService) GetComments(string) (string, error)        { return "", nil }
func (s *recordingIssueService) AddComment(string, string) (string, error) { return "", nil }
func (s *recordingIssueService) ReplyToComment(_ string, _ string, body string) (*core.Comment, error) {
	s.replyBody = body
	return &core.Comment{ID: "reply-id", User: core.User{Name: "Test User"}}, nil
}
func (s *recordingIssueService) AddReaction(string, string) error  { return nil }
func (s *recordingIssueService) GetIssueID(string) (string, error) { return "", nil }

type recordingProjectService struct {
	created *service.CreateProjectInput
	updated *service.UpdateProjectInput
}

func (s *recordingProjectService) Get(string) (string, error) { return "", nil }
func (s *recordingProjectService) GetWithOutput(string, format.Verbosity, format.OutputType) (string, error) {
	return "", nil
}
func (s *recordingProjectService) ListAll(int) (string, error) { return "", nil }
func (s *recordingProjectService) ListAllWithOutput(int, format.Verbosity, format.OutputType) (string, error) {
	return "", nil
}
func (s *recordingProjectService) ListByTeam(string, int) (string, error) { return "", nil }
func (s *recordingProjectService) ListByTeamWithOutput(string, int, format.Verbosity, format.OutputType) (string, error) {
	return "", nil
}
func (s *recordingProjectService) ListUserProjects(int) (string, error) { return "", nil }
func (s *recordingProjectService) ListUserProjectsWithOutput(int, format.Verbosity, format.OutputType) (string, error) {
	return "", nil
}
func (s *recordingProjectService) Create(input *service.CreateProjectInput) (string, error) {
	s.created = input
	return "created", nil
}
func (s *recordingProjectService) Update(_ string, input *service.UpdateProjectInput) (string, error) {
	s.updated = input
	return "updated", nil
}

type recordingCommentTransport struct {
	body string
}

func (t *recordingCommentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var request struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, err
	}

	response := `{"data":{"issue":{"id":"issue-id","identifier":"CEN-123","title":"Test issue"}}}`
	if strings.Contains(request.Query, "commentCreate") {
		t.body, _ = request.Variables["body"].(string)
		response = `{"data":{"commentCreate":{"success":true,"comment":{"id":"comment-id","body":"comment body","user":{"id":"user-id","name":"Test User","email":"test@example.com"},"issue":{"id":"issue-id","identifier":"CEN-123"}}}}}`
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(response)),
		Request:    req,
	}, nil
}

var _ service.IssueServiceInterface = (*recordingIssueService)(nil)
var _ service.ProjectServiceInterface = (*recordingProjectService)(nil)
