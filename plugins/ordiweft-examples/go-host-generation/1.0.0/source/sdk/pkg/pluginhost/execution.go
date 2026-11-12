package pluginhost

import (
	"context"
	"errors"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// ErrExecutionBusy means no execution intent was consumed. Only this error may
// be waited on before the original task starts; other failures are final.
var ErrExecutionBusy = errors.New("plugin_execution_pool_busy")

// Execution identifies one host-created resource. None of its fields are
// accepted from an executable package or an unbound RPC request.
type Execution struct {
	ID, JobID, Owner, Provider, ReleaseID, Image, Scope, Container, CodeDir string
	State                                                                   string
}

// ExecutionJournal must persist the send intent before a runner starts code.
// A released resource does not grant another execution of the original job.
type ExecutionJournal interface {
	ClaimPluginExecution(context.Context, Execution) error
	ReleasePluginExecution(context.Context, Execution) error
	PendingPluginExecutions(context.Context, string) ([]Execution, error)
}

// ExecutionOutcomeJournal preserves a protocol-complete candidate before any
// resource cleanup. Reading it neither adopts a business result nor permits replay.
type ExecutionOutcomeJournal interface {
	SavePluginExecutionOutcome(context.Context, Execution, aigc.Result) error
	ReadPluginExecutionOutcome(context.Context, Execution) (aigc.Result, error)
}

// ActionOutcomeJournal records a non-generation protocol result. It does not
// allocate a usage job or adopt the candidate into a business document.
type ActionOutcomeJournal interface {
	ExecutionJournal
	SavePluginActionOutcome(context.Context, Execution, ActionCompletion) error
	ReadPluginActionOutcome(context.Context, Execution) (ActionCompletion, error)
}
