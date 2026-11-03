package guards

// Provenance: how came to know a thing, recorded alongside the thing itself.
//
// collects from runtimes over four different mechanisms whose fidelity differs by roughly
// an order of magnitude. A Claude Code `PermissionRequest` hook hands a typed payload that
// names the operation. A Codex OTLP log record hands it a body string that has to classify
// by substring match, with a catch-all that lands on `tool.invoked` when nothing matches. Both
// produced an `event.action` in the same field, and until these two markers existed nothing in the
// event distinguished them -- so every rule in rules/, every SIEM query, and every dashboard
// treated a reported fact and a keyword guess as the same claim.
//
// Two independent axes, hence two fields rather than one:
//
// - harness.collection_method is about the pipe: which mechanism carried this event off the
// runtime. Stable for the lifetime of an install, useful for "what is my coverage" questions.
// - event.fidelity is about the specific claim: did the source name this action, or did
// derive it. Varies event to event within one collection method, because an OTLP stream
// carries both records that declare `event.name` and records that carry only prose.
//
// Neither field is required. An event emits about itself (a health heartbeat, a self-update
// result) has no collecting harness and no source action to be faithful to, and leaves both empty
// rather than asserting a method it did not use.
const (
	// CollectionMethodHook is a runtime-native hook: the runtime executes `so` and
	// hands it a structured payload for a named lifecycle event.
	CollectionMethodHook = "hook"
	// CollectionMethodOTLP is OpenTelemetry data the runtime exported to the local
	// collector, converted by the json exporter.
	CollectionMethodOTLP = "otlp"
	// CollectionMethodPlugin is a -managed plugin or extension file the runtime loads,
	// which calls `so` on the runtime's own event callbacks. Distinct from `hook`
	// because ships and versions the plugin source, so its coverage is the to fix
	// rather than the vendor's to expose.
	CollectionMethodPlugin = "plugin"
	// CollectionMethodPoll is a periodic pull from a source the runtime maintains -- its API, or
	// the session store it commits to disk -- where sees whatever that source holds after
	// the fact rather than observing the agent as it works. The Devin sessions API and fx's
	// ~/.fx/sessions log are both this: different transports, the same posture, and the same thing
	// a consumer needs to know, which is that nothing on this path could have been intercepted.
	CollectionMethodPoll = "poll"
)

const (
	// FidelityObserved means the source named this action. The value came from a hook event
	// name, a plugin event type, an explicit `event.action` attribute, or a structured
	// attribute whose value identifies the operation (`mcp.method.name`, a known
	// `claude_code.*`/Codex `event.name`, a tool-call id). A consumer can treat the action as
	// a report.
	FidelityObserved = "observed"
	// FidelityInferred means derived this action rather than being told it: a substring
	// match over free text, a catch-all default, or an action synthesized from an adjacent
	// observation. The action is the best reading and may be wrong. A consumer that needs
	// certainty -- counting approvals, alerting on a specific action -- should exclude these.
	FidelityInferred = "inferred"
)

// ValidCollectionMethod reports whether value is one of the four defined methods. The empty
// string is not a method and is rejected here; callers that allow "unset" check for it first,
// which is what Validate does.
func ValidCollectionMethod(value string) bool {
	switch value {
	case CollectionMethodHook, CollectionMethodOTLP, CollectionMethodPlugin, CollectionMethodPoll:
		return true
	}
	return false
}

// ValidFidelity reports whether value is one of the two defined fidelities, on the same terms as
// ValidCollectionMethod.
func ValidFidelity(value string) bool {
	switch value {
	case FidelityObserved, FidelityInferred:
		return true
	}
	return false
}
