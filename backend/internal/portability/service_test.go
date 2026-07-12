package portability

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateProjectBundle(t *testing.T) {
	valid := ProjectBundle{
		Schema:  ProjectSchema,
		Project: ExportProject{Name: "Portable project", Description: "Description"},
		Labels:  []ExportLabel{{Name: "Backend", Color: "#336699"}},
		Tickets: []ExportTicket{
			{SourceID: "epic-1", Title: "Epic", Type: "epic", Status: "open", Priority: "high", Labels: []string{"Backend"}},
			{SourceID: "task-1", ParentSourceID: stringPointer("epic-1"), Title: "Task", Type: "task", Status: "done", Priority: "medium", Labels: []string{"Backend"}},
		},
	}

	tests := []struct {
		name   string
		mutate func(*ProjectBundle)
	}{
		{name: "unsupported schema", mutate: func(value *ProjectBundle) { value.Schema = "progo.project.v2" }},
		{name: "duplicate source id", mutate: func(value *ProjectBundle) { value.Tickets[1].SourceID = "epic-1" }},
		{name: "unknown label", mutate: func(value *ProjectBundle) { value.Tickets[0].Labels = []string{"Unknown"} }},
		{name: "invalid hierarchy", mutate: func(value *ProjectBundle) { value.Tickets[0].ParentSourceID = stringPointer("task-1") }},
		{name: "oversized title", mutate: func(value *ProjectBundle) { value.Tickets[0].Title = strings.Repeat("x", 121) }},
	}

	require.NoError(t, ValidateProjectBundle(valid, true))
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			value := valid
			value.Labels = append([]ExportLabel(nil), valid.Labels...)
			value.Tickets = append([]ExportTicket(nil), valid.Tickets...)
			testCase.mutate(&value)
			require.ErrorIs(t, ValidateProjectBundle(value, true), ErrInvalidBundle)
		})
	}
}

func stringPointer(value string) *string { return &value }
