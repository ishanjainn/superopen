package clineplugin

import _ "embed"

// Template embeds the checked copy used by Superopen's Go installer.
// The root source of truth lives at plugins/cline-superopen/src/superopen.ts.
// Tests fail if this copy drifts.
//
//go:embed superopen.ts
var Template string
