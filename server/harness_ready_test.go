package server

import (
	"context"
	"testing"

	nh "github.com/Autumn-27/norma/harness"
	"github.com/barrier-networks/bartex/db"
)

// ReadyFor is true for a task on a whole-task harness backend even with no LLM
// resolver wired (no provider configured) — that is the point of the mode.
func TestReadyForHarnessTaskNeedsNoLLM(t *testing.T) {
	e := NewEngine(nil)
	task := &Task{ID: "1"}

	// No LLM resolver and no harness: not ready.
	if e.ReadyFor(task) {
		t.Fatal("task with neither LLM nor harness should not be ready")
	}

	// Wire a whole-task harness backend that claims this task.
	e.SetHarnessTaskRunner(
		func(context.Context, *Task, string, int64, func(db.Activity)) (nh.TerminalReason, error) {
			return nh.ReasonCompleted, nil
		},
		func(*Task) bool { return true },
	)
	if !e.ReadyFor(task) {
		t.Fatal("task on a harness backend should be ready without an LLM provider")
	}
	if !e.taskOnHarness(task) {
		t.Fatal("taskOnHarness should be true when both runner and predicate are set")
	}
}
