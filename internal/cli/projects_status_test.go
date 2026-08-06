package cli

import "testing"

func TestProjectsListHasStatusFlag(t *testing.T) {
	flag := newProjectsListCmd().Flags().Lookup("status")
	if flag == nil {
		t.Fatal("projects list is missing --status")
	}
}
