package service

import (
	"testing"

	"safetyplatform/internal/constants"
)

func TestIncidentStateTransitions(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"assign", constants.IncidentReported, constants.IncidentInvestigating, true},
		{"submit rectification", constants.IncidentInvestigating, constants.IncidentReviewPending, true},
		{"review approve", constants.IncidentReviewPending, constants.IncidentClosed, true},
		{"review reject", constants.IncidentReviewPending, constants.IncidentInvestigating, true},
		{"resubmit", constants.IncidentInvestigating, constants.IncidentReviewPending, true},
		{"legacy resolved close", constants.IncidentResolved, constants.IncidentClosed, true},

		{"cannot close investigating", constants.IncidentInvestigating, constants.IncidentClosed, false},
		{"cannot rectify reported", constants.IncidentReported, constants.IncidentReviewPending, false},
		{"cannot review investigating", constants.IncidentInvestigating, constants.IncidentInvestigating, false},
		{"cannot move closed", constants.IncidentClosed, constants.IncidentInvestigating, false},
		{"cannot self review pending", constants.IncidentReviewPending, constants.IncidentReviewPending, false},
		{"unknown from", "bogus", constants.IncidentClosed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canTransitionIncident(tc.from, tc.to); got != tc.want {
				t.Fatalf("canTransitionIncident(%s,%s)=%v want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}
