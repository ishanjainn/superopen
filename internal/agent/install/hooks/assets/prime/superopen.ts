// __SO_MANAGED_MARKER__
// Superopen endpoint telemetry extension for Prime Agent.
// Managed by superopen endpoint hooks install --harness prime.

// The argv Superopen's installer writes, as an array rather than a command line.
//
// An argv array is passed straight to the OS with no shell in between, which sidesteps quoting
// entirely -- the same reason the Cline plugin uses one. Prime Agent runs as a Bun-compiled binary
// or under Node depending on how it was installed, and on Windows as readily as on macOS, so there
// is no one shell whose quoting rules would hold.
const superopenArgv: string[] = ["__SO_ARGV__"]

const debugEnabled = process.env.SO_PRIME_DEBUG === "1"

// A send that has not finished in this long is abandoned. Prime Agent awaits event handlers before
// continuing the agent loop, so this is the worst case Superopen can add to a tool call.
const sendTimeoutMs = 2000

// How deep into an event the serializer will walk before giving up.
const maxDepth = 8

// Prime Agent's own event objects, narrowed to the fields Superopen reads.
//
// Declared here rather than imported from @earendil-works/pi-coding-agent on purpose -- and Prime
// Agent really does publish under that name, because it is a hard fork of pi-mono that kept the
// upstream package names. The installed file sits in ~/.prime/agent/extensions with no node_modules
// beside it, and Prime Agent loads it through jiti, which strips types without resolving them. A
// value import would fail at load; a type import would resolve for nobody. Local structural types
// cost the compile-time link to Prime Agent's definitions and buy a file that loads wherever Prime
// Agent does.
type PrimeEvent = Record<string, unknown> & { type: string }

interface PrimeSessionManager {
  getSessionId?: () => string
  getCwd?: () => string
}

interface PrimeContext {
  cwd?: string
  sessionManager?: PrimeSessionManager
  model?: { id?: string; name?: string; provider?: string } | undefined
  hasUI?: boolean
}

// The events Superopen subscribes to, and nothing else.
//
// Prime Agent publishes upwards of forty event types: provider request and response internals,
// streaming message updates, compaction, refinement and tree-navigation signals, resource discovery
// and TUI plumbing. Most describe how the runtime got its work done rather than what the agent did.
// Subscribing to all of them would fill the runtime log with rows no query asks for -- the same
// reason the Cline mapper drops unrecognized stages -- and would put Superopen in the path of every
// streaming token update.
//
// `tool_call` and `tool_result` are the pair that carries tool activity: the first names the tool
// and its arguments before it runs, the second carries the outcome. On this runtime that is nearly
// everything, because its only default tool is `ipython` -- a persistent Python kernel through
// which the agent reads files, writes files and runs shell commands via a `bash()` helper -- so a
// cell's code and the diffs its kernel streams are the agent's actions rather than a detail of one
// tool. `user_bash` is a command the human ran with the `!` prefix, which no tool event covers.
//
// Deliberately absent: any approval event. Prime Agent's `tool_call` handler can block a call, but
// that is an extension deciding, not an operator being asked -- it exposes no operator approval
// decision through this API. Synthesizing an approval from a pre-tool notification would put a
// decision nobody made into the log, so Superopen records these as tool activity and leaves approval
// telemetry empty, exactly as it does for Pi and Cline.
//
// Also absent, and not an omission: `user_python`. Oh My Pi added a `$` operator prefix that runs
// Python outside the agent loop and subscribes to its event; Prime Agent forked pi-mono separately
// and has no such prefix. Its operator surface is `!` alone.
const subscribedEvents = [
  "session_start",
  "session_shutdown",
  "input",
  "tool_call",
  "tool_result",
  "user_bash",
  "message_end",
] as const

function debugLog(message: string, extra?: unknown) {
  if (!debugEnabled) return
  try {
    // eslint-disable-next-line no-console
    console.error("[superopen-prime]", message, extra ?? "")
  } catch {
    // Debug logging must stay best-effort.
  }
}

// safeClone turns a Prime Agent event into something JSON.stringify accepts.
//
// Its events carry live objects: an AbortSignal on the session events, AgentMessage arrays holding
// class instances, and tool inputs that are mutable by design. JSON.stringify throws on a circular
// reference and on a bigint, and silently flattens an Error to {}, so each is handled here rather
// than losing the whole payload to one bad field.
//
// Cycles are tracked along the current path and released on the way out, so an object that legibly
// appears twice -- the same file path referenced from two places in one edit -- is kept both times.
// A WeakSet that never released would drop the second occurrence as if it were a cycle.
function safeClone(value: unknown, depth = 0, seen = new WeakSet<object>()): unknown {
  if (value === null) return null
  const kind = typeof value
  if (kind === "function" || kind === "symbol" || kind === "undefined") return undefined
  if (kind === "bigint") return String(value)
  if (kind !== "object") return value
  if (depth >= maxDepth) return undefined

  const object = value as object
  if (seen.has(object)) return undefined
  if (value instanceof Error) return { name: value.name, message: value.message }
  if (value instanceof Date) return value.toISOString()

  seen.add(object)
  try {
    if (Array.isArray(value)) {
      return value.map((item) => safeClone(item, depth + 1, seen))
    }
    const out: Record<string, unknown> = {}
    for (const [key, nested] of Object.entries(value as Record<string, unknown>)) {
      const cloned = safeClone(nested, depth + 1, seen)
      if (cloned !== undefined) out[key] = cloned
    }
    return out
  } finally {
    seen.delete(object)
  }
}

function shrinkCandidate(event: Record<string, unknown>, input: unknown) {
  const name = typeof event.toolName === "string" ? event.toolName.toLowerCase() : ""
  if (event.type !== "tool_result" || event.isError || (name !== "bash" && name !== "shell")) return
  if (!input || typeof input !== "object" || Array.isArray(input)) return
  const blocks = Array.isArray(event.content) ? event.content as Array<Record<string, unknown>> : []
  const text = blocks.filter((block) => block?.type === "text" && typeof block.text === "string")
    .map((block) => block.text as string).join("")
  if (text.length < 2048 || Buffer.byteLength(text, "utf8") > 2 * 1024 * 1024) return
  return { text, blocks }
}

function shrinkPatch(stdout: string | undefined, blocks: Array<Record<string, unknown>>) {
  try {
    const response = JSON.parse(stdout || "")
    const replacement = response?.output_replacement
    if (typeof replacement !== "string" || !replacement) return
    return { content: [{ type: "text", text: replacement }, ...blocks.filter((block) => block.type !== "text")] }
  } catch {
    return
  }
}

async function sendToSuperopen(payload: Record<string, unknown>, capture = false): Promise<string | undefined> {
  // Flattened before anything else sees it, so every consumer of this function is handed plain
  // data rather than a live Prime Agent event with cycles and functions still attached.
  const safe = safeClone(payload)

  const testSender = (globalThis as Record<symbol, unknown>)[Symbol.for("superopen.prime.testSender")]
  if (typeof testSender === "function") {
    const response = await (testSender as (value: unknown) => unknown)(safe)
    return capture && typeof response === "string" ? response : undefined
  }

  let body: string
  try {
    body = JSON.stringify(safe)
  } catch (err) {
    debugLog("payload could not be serialized", err)
    return
  }
  if (!body) return

  if (capture) {
    try {
      const { spawnSync } = await import("node:child_process")
      const result = spawnSync(superopenArgv[0], superopenArgv.slice(1), {
        input: body, encoding: "utf8", timeout: 12000, maxBuffer: 2 * 1024 * 1024,
        stdio: ["pipe", "pipe", "ignore"], windowsHide: true,
      })
      return result.status === 0 && typeof result.stdout === "string" ? result.stdout : undefined
    } catch {
      return undefined
    }
  }

  try {
    // Imported lazily so a host without node:child_process fails to send rather than failing to
    // load: a throwing import at module scope would surface to the user as a broken extension, and
    // Prime Agent reports an extension that throws at load time rather than continuing without it.
    const { spawn } = await import("node:child_process")
    await new Promise<void>((resolve) => {
      let settled = false
      const finish = () => {
        if (settled) return
        settled = true
        resolve()
      }
      let child
      try {
        child = spawn(superopenArgv[0], superopenArgv.slice(1), {
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
        debugLog("hook binary timed out", { type: payload?.type })
        finish()
      }, sendTimeoutMs)
      // An unreferenced timer cannot keep Prime Agent alive at exit waiting on Superopen telemetry.
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
      // unhandled error event in the host process. Superopen telemetry must never do that.
      child.stdin?.on("error", () => {})
      try {
        child.stdin?.end(body)
      } catch (err) {
        debugLog("payload could not be written", err)
      }
    })
  } catch (err) {
    debugLog("send failed", err)
    // Superopen telemetry must never interrupt Prime Agent execution.
  }
}

// identity lifts the session and workspace fields Superopen needs to the top level of the envelope.
//
// Prime Agent keeps them on the handler context rather than on the event, and behind accessor
// functions rather than as properties, so they have to be read per event: a session id captured
// once at extension load would be wrong after `/new`, a resume, a fork or a `/tree` navigation, all
// of which replace the session without reloading the extension.
//
// Each accessor is called defensively. `getSessionId` and `getCwd` are documented members of the
// read-only session manager, but an extension that loses its session id loses the field that groups
// every event of a run, so a throwing accessor costs that field rather than the event.
function identity(ctx: PrimeContext | undefined): Record<string, unknown> {
  const fields: Record<string, unknown> = {}
  if (!ctx) return fields
  const manager = ctx.sessionManager
  try {
    const sessionId = manager?.getSessionId?.()
    if (sessionId) fields.sessionId = sessionId
  } catch (err) {
    debugLog("session id could not be read", err)
  }
  let cwd = ctx.cwd
  if (!cwd) {
    try {
      cwd = manager?.getCwd?.()
    } catch (err) {
      debugLog("cwd could not be read", err)
    }
  }
  if (cwd) fields.cwd = cwd
  const model = ctx.model
  if (model) {
    // Prime Agent's Model carries an id and a provider separately. Joined here so the envelope's
    // `model` is one string, matching how every other Superopen collection path reports it.
    const name = model.id || model.name
    if (name) fields.model = model.provider ? `${model.provider}/${name}` : name
  }
  // Whether the session has a user-facing surface at all: false in print (`-p`) and JSON mode, true
  // in interactive and RPC mode. Prime Agent exposes no `mode` string the way Pi and Oh My Pi do,
  // and this is the fact those were read for -- an unattended run is not a run somebody was
  // watching, which is what a reader needs to know before drawing a conclusion from an absent
  // objection.
  if (typeof ctx.hasUI === "boolean") fields.primeHasUi = ctx.hasUI
  return fields
}

type PrimeExtensionAPI = {
  on: (event: string, handler: (event: PrimeEvent, ctx: PrimeContext) => Promise<unknown> | unknown) => void
}

// createSuperopenExtension is exported for tests; Prime Agent loads the default export below.
export function createSuperopenExtension() {
  const pending = new Map<string, unknown>()
  const forward = async (event: PrimeEvent, ctx: PrimeContext) => {
    try {
      const id = typeof event.toolCallId === "string" ? event.toolCallId : ""
      if (event.type === "tool_call" && id) {
        pending.delete(id)
        pending.set(id, event.input)
        if (pending.size > 64) pending.delete(pending.keys().next().value as string)
      }
      const toolInput = event.type === "tool_result" ? (event.input ?? pending.get(id)) : undefined
      if (event.type === "tool_result" && id) pending.delete(id)
      if (event.type === "session_start" || event.type === "session_shutdown") pending.clear()
      const envelope: Record<string, unknown> = { ...identity(ctx), ...event }
      const candidate = shrinkCandidate(event, toolInput)
      if (candidate) {
        envelope.tool_name = event.toolName
        envelope.tool_input = toolInput
        envelope.tool_response = candidate.text
      }
      const stdout = await sendToSuperopen(envelope, Boolean(candidate))
      if (candidate) return shrinkPatch(stdout, candidate.blocks)
    } catch (err) {
      // Unreachable in practice: sendToSuperopen swallows its own failures. Kept because a handler
      // that rejects is a handler that can break the run it was observing.
      debugLog("handler failed", err)
    }
  }

  return {
    register(pi: PrimeExtensionAPI) {
      for (const name of subscribedEvents) {
        // Only an eligible shell tool_result may return a smaller, recoverable
        // content patch. Other events remain observation-only.
        pi.on(name, forward)
      }
    },
    // Exposed so a test can assert the subscription list without a live Prime Agent instance.
    subscribedEvents: [...subscribedEvents],
  }
}

export default function superopenExtension(pi: PrimeExtensionAPI) {
  createSuperopenExtension().register(pi)
}
