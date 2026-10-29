// __SO_MANAGED_MARKER__
// Superopen endpoint telemetry plugin for OpenClaw Gateway.
// Managed by superopen endpoint hooks install --harness openclaw.
import { spawnSync } from "node:child_process"

// The argv Superopen's installer writes, as an array rather than a command line.
//
// An argv array is passed straight to the OS with no shell in between, which sidesteps quoting
// entirely -- the same reason the Cline plugin and the Pi-family extensions use one. OpenClaw runs
// as a long-lived gateway on macOS, Linux and Windows alike, so there is no single shell whose
// quoting rules would hold.
const superopenArgv = ["__SO_ARGV__"]

const debugEnabled = process.env.SO_OPENCLAW_DEBUG === "1"

// A send that has not finished in this long is abandoned.
//
// Well under OpenClaw's 15 s default hook budget on purpose, and the number only matters for the
// observe hooks: `before_tool_call` never waits at all (see dispatch below).
const sendTimeoutMs = 2000
const promptTimeoutMs = 4500

// How deep into an event the serializer will walk before giving up.
const maxDepth = 8

// The hooks this plugin registers, and nothing else.
//
// These eleven hooks cover model context, session activity, tool calls and usage.
//
// Prompt context is supplied through before_prompt_build. Claim and gate hooks remain outside
// this subscription because they can suppress a user message or tool call.
const subscribedHooks = [
  "before_prompt_build",
  "session_start",
  "session_end",
  "message_received",
  "before_tool_call",
  "after_tool_call",
  "llm_output",
  "before_compaction",
  "after_compaction",
  "subagent_spawned",
  "subagent_ended",
]

// The hooks whose handler must never make OpenClaw wait.
//
// `before_tool_call` is registered fail-closed by OpenClaw itself: its default failure policy for
// this hook is "fail-closed", so a handler that throws or exceeds its timeout *blocks the tool
// call*. Superopen telemetry must never be able to stop an agent from working, so this handler
// dispatches its send and returns synchronously, without awaiting and without a promise for
// OpenClaw to wait on. The gateway is a long-lived process, so an unawaited child still runs to
// completion.
//
// The cost is ordering: a `tool.invoked` row can land in the log after the `tool.completed` row
// for the same call. That is acceptable because ordering was never the join -- `gen_ai.tool.call.id`
// is, and both halves carry it.
const nonBlockingHooks = new Set(["before_tool_call"])

// How many in-flight tool calls the apply_patch enrichment below will remember at once.
//
// An entry is consumed by its own `after_tool_call`, so this bound only has to cover calls that
// never produce one -- an aborted run, a session torn down mid-tool, a gateway shutting down
// between the two halves. Those are reclaimed by oldest-first eviction here and by nothing else:
// no lifecycle hook clears this map, deliberately, because in a gateway every session boundary
// belongs to *one* of the conversations in flight and clearing on it would discard the others'
// live state. A gateway fans several conversations into one process, so this is set well above a
// single agent's working set and is still only a few kilobytes at worst.
const maxPendingToolCalls = 256

// rememberDerivedPaths records the destination paths OpenClaw derived for a proposed tool call,
// keyed by OpenClaw's own tool call id.
//
// This exists for one reason: `before_tool_call` carries `derivedPaths` and `after_tool_call` does
// not. `apply_patch` takes the whole patch envelope as a single string argument, so without those
// paths a completed patch is a tool call with a blob attached and no file rows at all -- and every
// file-scoped detection Superopen ships matches on `file.path`.
//
// The join is OpenClaw's own call id rather than a timestamp or a guess, and both halves fire in
// this same process, so remembering the value until the call completes is exact. That id is either
// the provider's own (`call_...`, `toolu_...`) or `openclaw-<randomUUID()>`, so it is unique across
// every conversation the gateway is serving and two sessions cannot collide on one entry.
function rememberDerivedPaths(pending, event) {
  const id = typeof event?.toolCallId === "string" ? event.toolCallId : ""
  if (!id || !Array.isArray(event.derivedPaths) || event.derivedPaths.length === 0) return
  // Re-inserting moves the entry to the end of the Map's insertion order, which is what makes the
  // eviction below drop the genuinely oldest call rather than the least recently seen one.
  pending.delete(id)
  pending.set(id, event.derivedPaths)
  while (pending.size > maxPendingToolCalls) {
    const oldest = pending.keys().next()
    if (oldest.done) break
    pending.delete(oldest.value)
  }
}

// takeDerivedPaths returns and forgets the paths remembered for a completed call.
//
// Absent rather than guessed when the call is unknown: a patch event enriched with some other
// call's paths would be worse than one with none, because it would read as evidence that those
// files changed.
function takeDerivedPaths(pending, event) {
  const id = typeof event?.toolCallId === "string" ? event.toolCallId : ""
  if (!id) return undefined
  const paths = pending.get(id)
  pending.delete(id)
  return paths
}

function debugLog(message, extra) {
  if (!debugEnabled) return
  try {
    console.error("[superopen-openclaw]", message, extra ?? "")
  } catch {
    // Debug logging must stay best-effort.
  }
}

// safeClone turns an OpenClaw hook payload into something JSON.stringify accepts.
//
// The payloads carry live objects: an AbortSignal on the tool context, provider message objects on
// `llm_output`, and tool params that are mutable by design. JSON.stringify throws on a circular
// reference and on a bigint, and silently flattens an Error to {}, so each is handled here rather
// than losing the whole payload to one bad field.
//
// Cycles are tracked along the current path and released on the way out, so an object that legibly
// appears twice -- the same file path referenced from two places in one patch -- is kept both
// times. A WeakSet that never released would drop the second occurrence as if it were a cycle.
function safeClone(value, depth = 0, seen = new WeakSet()) {
  if (value === null) return null
  const kind = typeof value
  if (kind === "function" || kind === "symbol" || kind === "undefined") return undefined
  if (kind === "bigint") return String(value)
  if (kind !== "object") return value
  if (depth >= maxDepth) return undefined

  if (seen.has(value)) return undefined
  if (value instanceof Error) return { name: value.name, message: value.message }
  if (value instanceof Date) return value.toISOString()

  seen.add(value)
  try {
    if (Array.isArray(value)) {
      return value.map((item) => safeClone(item, depth + 1, seen))
    }
    const out = {}
    for (const [key, nested] of Object.entries(value)) {
      const cloned = safeClone(nested, depth + 1, seen)
      if (cloned !== undefined) out[key] = cloned
    }
    return out
  } finally {
    seen.delete(value)
  }
}

// sendToSuperopen spawns the hook binary and writes one envelope to its stdin.
//
// Returns a promise that always resolves. Nothing in this file rejects: a handler that rejects is
// a handler that can break the run it was observing, and on `before_tool_call` it would block the
// tool call outright.
function sendToSuperopen(payload) {
  try {
    return dispatch(payload)
  } catch (err) {
    // A synchronous throw would escape the handler, and on `before_tool_call` that blocks the
    // tool call. The body below is written not to throw; this is the guard that makes "not to
    // throw" a property of the function rather than a property of its current shape.
    debugLog("send failed", err)
    return Promise.resolve()
  }
}

function dispatch(payload) {
  // Flattened before anything else sees it, so every consumer is handed plain data rather than a
  // live payload with cycles and functions still attached.
  const safe = safeClone(payload)

  const testSender = globalThis[Symbol.for("superopen.openclaw.testSender")]
  if (typeof testSender === "function") {
    return Promise.resolve(testSender(safe))
  }

  let body
  try {
    body = JSON.stringify(safe)
  } catch (err) {
    debugLog("payload could not be serialized", err)
    return Promise.resolve()
  }
  if (!body) return Promise.resolve()

  // Imported lazily so a host without node:child_process fails to send rather than failing to
  // load: a throwing import at module scope would surface to the operator as a broken plugin, and
  // OpenClaw disables a plugin that throws while loading rather than continuing without it.
  return import("node:child_process")
    .then(
      ({ spawn }) =>
        new Promise((resolve) => {
          let settled = false
          const finish = () => {
            if (settled) return
            settled = true
            resolve()
          }
          let child
          try {
            const eventName = payload && typeof payload.hook === "string" && payload.hook ? payload.hook : "tool"
            child = spawn(superopenArgv[0], superopenArgv.slice(1).concat(["--event=" + eventName]), {
              stdio: ["pipe", "ignore", "ignore"],
              windowsHide: true,
            })
          } catch (err) {
            debugLog("hook binary could not be spawned", err)
            finish()
            return
          }
          const timer = setTimeout(() => {
            try {
              child.kill()
            } catch {
              // Already gone.
            }
            debugLog("hook binary timed out", { hook: payload?.hook })
            finish()
          }, sendTimeoutMs)
          // An unreferenced timer cannot keep the gateway alive at exit waiting on Superopen
          // telemetry.
          if (typeof timer.unref === "function") timer.unref()
          const done = () => {
            clearTimeout(timer)
            finish()
          }
          child.on("error", (err) => {
            debugLog("hook binary failed", err)
            done()
          })
          child.on("close", done)
          // Without this, a hook binary that exits before reading stdin turns an EPIPE into an
          // unhandled error event in the gateway process. Superopen telemetry must never do that.
          child.stdin?.on("error", () => {})
          try {
            child.stdin?.end(body)
          } catch (err) {
            debugLog("payload could not be written", err)
          }
        }),
    )
    .catch((err) => {
      debugLog("send failed", err)
    })
}

// identity lifts the session and workspace fields Superopen needs out of the hook context.
//
// Read per event rather than captured once at load, because a gateway process serves many
// conversations at once: a session id cached at registration would label every later event with
// whichever conversation happened to be first.
//
// `sessionKey` is carried alongside `sessionId` rather than instead of it. The two are not
// interchangeable -- `sessionKey` is the canonical conversation, stable across the session-id
// rotation a compaction causes, while `sessionId` is one generation of it. Superopen groups events by
// the session and keeps the conversation key beside them.
//
// `workspaceDir` is absent for a chat turn that never touched a repository, and is left absent
// rather than defaulted to the gateway's own working directory, which would attribute the work to
// wherever the daemon happened to be started.
function identity(ctx) {
  const fields = {}
  if (!ctx || typeof ctx !== "object") return fields
  for (const [target, source] of [
    ["sessionId", "sessionId"],
    ["sessionKey", "sessionKey"],
    ["runId", "runId"],
    ["agentId", "agentId"],
    ["channel", "channel"],
    ["accountId", "accountId"],
    ["trigger", "trigger"],
  ]) {
    const value = ctx[source]
    if (typeof value === "string" && value) fields[target] = value
  }
  if (typeof ctx.workspaceDir === "string" && ctx.workspaceDir) fields.cwd = ctx.workspaceDir
  // OpenClaw keeps the provider and the model id apart on the hook context. Joined here into the
  // one provider-prefixed string every other Superopen collection path reports, which the writer
  // then splits back into the canonical model name and `gen_ai.provider.name`. Sending the bare
  // model id would leave the provider unrecorded, and a gateway routes one model through several.
  if (typeof ctx.modelId === "string" && ctx.modelId) {
    fields.model =
      typeof ctx.modelProviderId === "string" && ctx.modelProviderId
        ? `${ctx.modelProviderId}/${ctx.modelId}`
        : ctx.modelId
  }
  return fields
}

// buildEnvelope assembles what the hook binary reads from stdin.
//
// The hook event stays nested under `event` rather than being spread onto the envelope. OpenClaw
// hands a handler `(event, ctx)` as two objects whose field names overlap -- `runId`, `sessionKey`
// and `sessionId` are on both -- and `before_tool_call` carries a `params` object whose keys belong
// to the model rather than to OpenClaw. A flattened envelope would let a tool argument named
// `sessionId` overwrite the session the event belongs to. One level of nesting makes that
// impossible, and the mapper is written against it.
function buildEnvelope(hook, event, ctx) {
  const envelope = identity(ctx)
  envelope.hook = hook
  // A hook whose event carries its own session identity wins over the context's: `session_start`,
  // `session_end` and `message_received` each name the session they are about, and the context is
  // the more general answer.
  if (event && typeof event === "object") {
    for (const key of ["sessionId", "sessionKey", "runId"]) {
      const value = event[key]
      if (typeof value === "string" && value) envelope[key] = value
    }
  }
  envelope.event = event ?? {}
  if (hook === "before_prompt_build") {
    // An explicit empty currentUserMessage is an image-only or continuation turn.
    envelope.prompt = typeof event?.currentUserMessage === "string" ? event.currentUserMessage : event?.prompt ?? ""
  }
  return envelope
}

// Prompt hooks consume the command's control JSON; observation hooks discard stdout.
// Bound both wait time and output so an unavailable CLI never stalls the gateway.
async function promptContext(envelope) {
  const testReceiver = globalThis[Symbol.for("superopen.openclaw.testContextReceiver")]
  if (typeof testReceiver === "function") return testReceiver(envelope)
  try {
    const body = JSON.stringify(safeClone(envelope))
    if (!body) return undefined
    const { spawn } = await import("node:child_process")
    return await new Promise((resolve) => {
      let child
      try {
        child = spawn(superopenArgv[0], superopenArgv.slice(1).concat(["--event=before_prompt_build"]), {
          stdio: ["pipe", "pipe", "ignore"], windowsHide: true,
        })
      } catch {
        resolve(undefined)
        return
      }
      let output = ""
      let settled = false
      const finish = (value) => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        resolve(value)
      }
      const timer = setTimeout(() => { child.kill(); finish(undefined) }, promptTimeoutMs)
      child.on("error", () => finish(undefined))
      child.stdout?.on("data", (chunk) => {
        output += chunk.toString()
        if (output.length > 65536) { child.kill(); finish(undefined) }
      })
      child.on("close", () => {
        try {
          const data = JSON.parse(output)
          const context = data?.additionalContext
          finish(typeof context === "string" && context ? { prependContext: context } : undefined)
        } catch { finish(undefined) }
      })
      child.stdin?.on("error", () => {})
      child.stdin?.end(body)
    })
  } catch { return undefined }
}

// createSuperopenPlugin is exported for tests; OpenClaw loads the default export below.
export function createSuperopenPlugin() {
  // Per plugin instance rather than per module. OpenClaw loads the plugin once, so this is the
  // same lifetime in production, and it keeps one instance's in-flight calls out of another's.
  const pendingDerivedPaths = new Map()
  const pendingShrinkInputs = new Map()

  const persistShrunkResult = (event, ctx) => {
    try {
      const message = event?.message
      const id = event?.toolCallId || message?.toolCallId || message?.tool_call_id
      const input = typeof id === "string" ? pendingShrinkInputs.get(id) : undefined
      if (typeof id === "string") pendingShrinkInputs.delete(id)
      if (!input || !message || event?.isError || message?.isError) return
      const blocks = Array.isArray(message.content) ? message.content : null
      const text = typeof message.content === "string" ? message.content :
        blocks ? blocks.filter((block) => block?.type === "text" && typeof block.text === "string")
          .map((block) => block.text).join("") : ""
      if (text.length < 2048 || Buffer.byteLength(text, "utf8") > 2 * 1024 * 1024) return
      const cwd = input.params.workdir || input.params.cwd || ctx?.cwd || process.cwd()
      const payload = JSON.stringify({
        type: "tool_result_persist", cwd, tool_name: input.toolName,
        tool_input: input.params, tool_response: text,
      })
      const result = spawnSync(superopenArgv[0], ["sessions", "hook", "--vendor=openclaw", "--event=tool_result_persist"], {
        input: payload, encoding: "utf8", timeout: 12000, maxBuffer: 2 * 1024 * 1024,
        stdio: ["pipe", "pipe", "ignore"], windowsHide: true,
      })
      if (result.status !== 0) return
      const replacement = JSON.parse(result.stdout)?.output_replacement
      if (typeof replacement !== "string" || !replacement) return
      return { message: {
        ...message,
        content: blocks ? [{ type: "text", text: replacement }, ...blocks.filter((block) => block?.type !== "text")] : replacement,
      } }
    } catch {
      return
    }
  }

  const handlerFor = (hook) => (event, ctx) => {
    let envelope
    try {
      envelope = buildEnvelope(hook, event, ctx)

      if (hook === "before_prompt_build") {
        return promptContext(envelope)
      }

      if (hook === "before_tool_call") {
        rememberDerivedPaths(pendingDerivedPaths, event)
        if (typeof event.toolCallId === "string" && ["exec", "bash", "shell"].includes(event.toolName)) {
          pendingShrinkInputs.delete(event.toolCallId)
          pendingShrinkInputs.set(event.toolCallId, { toolName: event.toolName, params: event.params || {} })
          while (pendingShrinkInputs.size > 256) pendingShrinkInputs.delete(pendingShrinkInputs.keys().next().value)
        }
      } else if (hook === "after_tool_call") {
        // OpenClaw does not repeat `derivedPaths` on the completion, so the value remembered from
        // the proposal is attached here under the same key the mapper already reads.
        const derived = takeDerivedPaths(pendingDerivedPaths, event)
        if (derived !== undefined && envelope.event && typeof envelope.event === "object") {
          envelope.event = { ...envelope.event, derivedPaths: derived }
        }
      }
      // No session lifecycle hook clears pendingDerivedPaths, and that is deliberate. A gateway
      // serves many conversations in one process, so a `session_end` belongs to one of them --
      // clearing on it would discard the paths of every apply_patch in flight for every *other*
      // conversation, and their completions would then be recorded with no file rows at all.
      // Compaction also ends a session, so this would fire in ordinary operation rather than only
      // at shutdown. There is nothing to protect against by clearing: the key is a per-call id
      // OpenClaw never reuses, so a stale entry cannot be joined to a later call, and the bound
      // above reclaims it.
    } catch (err) {
      // Unreachable in practice, and caught anyway: a throw here on `before_tool_call` would block
      // the tool call, because OpenClaw registers that hook fail-closed.
      debugLog("envelope could not be built", err)
      return undefined
    }

    if (nonBlockingHooks.has(hook)) {
      // Dispatched without awaiting and without returning a promise. See nonBlockingHooks.
      void sendToSuperopen(envelope)
      return undefined
    }
    // Observation hooks return no control result. The prompt hook above is the only handler
    // that contributes model context.
    return sendToSuperopen(envelope).then(() => undefined)
  }

  return {
    register(api) {
      for (const hook of subscribedHooks) {
        api.on(hook, handlerFor(hook))
      }
      api.on("tool_result_persist", persistShrunkResult)
    },
    // Exposed so a test can assert the subscription list without a live gateway.
    subscribedHooks: [...subscribedHooks],
  }
}

export default {
  id: "superopen-endpoint",
  name: "Superopen Endpoint Telemetry",
  description: "Records agent activity and supplies graph and memory context.",
  register(api) {
    createSuperopenPlugin().register(api)
  },
}
