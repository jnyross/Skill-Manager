package main

import (
	"testing"

	"github.com/jnyross/Skill-Manager/internal/setup"
)

func TestValidateEvidenceFailsClosedOnLaneAndNegativeControlFailures(t *testing.T) {
	valid := artifact{
		Outcome:          setup.OutcomeVerified,
		RepeatNoOp:       true,
		RemovalVerified:  true,
		NegativeControls: map[string]bool{"claude-code": true, "codex": true},
		Lanes:            make([]lane, 96),
	}
	for index := range valid.Lanes {
		valid.Lanes[index] = lane{
			Member: "member", Tool: "tool", RequestedActivation: setup.ActivationAuto,
			ObservedActivation: setup.ActivationAuto, StaticVerified: true, Authenticated: true, RuntimeVerified: true,
		}
	}
	if err := validateEvidence(valid, true); err != nil {
		t.Fatalf("valid evidence: %v", err)
	}

	failedLane := valid
	failedLane.Lanes = append([]lane(nil), valid.Lanes...)
	failedLane.Lanes[17].RuntimeVerified = false
	if err := validateEvidence(failedLane, true); err == nil {
		t.Fatal("runtime failure passed live evidence gate")
	}

	failedControl := valid
	failedControl.NegativeControls = map[string]bool{"claude-code": false, "codex": true}
	if err := validateEvidence(failedControl, true); err == nil {
		t.Fatal("unknown-skill control failure passed live evidence gate")
	}
}
