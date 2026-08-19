// Package pi implements the Pi coding-agent hook adapter.
// Event shapes for the Pi coding-agent plugin:
// session_start, before_agent_start, message_end, tool_execution_*,
// turn_end, agent_end (turn boundary), session_shutdown (true close), compaction.
package pi

import (
	"context"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/sdk/go/semconv"
)

type adapter struct{}

// New returns a Pi normalize.Adapter.
func New() normalize.Adapter { return &adapter{} }

func (a *adapter) Vendor() string { return semconv.CodingAgentVendorPi }

func (a *adapter) Handle(ctx context.Context, in normalize.Input) error {
	_ = ctx
	return handle(in)
}
