// Package opencode implements the OpenCode coding-agent hook adapter.
// Event shapes for the OpenCode coding-agent plugin:
// session.created/updated, message.updated, message.part.updated,
// tool.execute.before/after, and dispose/session end.
package opencode

import (
	"context"

	"github.com/superopen/so/internal/coding/normalize"
	"github.com/superopen/so/sdk/go/semconv"
)

type adapter struct{}

// New returns an OpenCode normalize.Adapter.
func New() normalize.Adapter { return &adapter{} }

func (a *adapter) Vendor() string { return semconv.CodingAgentVendorOpenCode }

func (a *adapter) Handle(ctx context.Context, in normalize.Input) error {
	_ = ctx
	return handle(in)
}
