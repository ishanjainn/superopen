// __SO_MANAGED_MARKER__
// Superopen endpoint plugin for Cline.
// Managed by superopen endpoint hooks install --harness cline.

import { execFileSync } from "node:child_process"
import { statSync } from "node:fs"
import { basename, dirname, isAbsolute, join, resolve } from "node:path"

// The argv Superopen's installer writes, as an array rather than a command line.
//
// Cline plugins run inside whichever host loaded them -- the VS Code extension host, a JetBrains
// plugin host, or the CLI -- so the hook may be spawned on Windows as readily as on macOS. An argv
// array is passed straight to the OS with no shell in between, which sidesteps quoting entirely: a
// POSIX-quoted command line is not valid in cmd.exe, and a Windows path inside double quotes is
// still expanded by bash.
const superopenArgv: string[] = ["__SO_ARGV__"]

const debugEnabled = process.env.SO_CLINE_DEBUG === "1"

// A send that has not finished in this long is abandoned. Cline awaits these handlers, so this is
// the worst case Superopen can add to a tool call.
const sendTimeoutMs = 2000
const promptTimeoutMs = 4500

// How deep into a hook context the serializer will walk before giving up.
const maxDepth = 8

function debugLog(message: string, extra?: unknown) {
  if (!debugEnabled) return
  try {
    // eslint-disable-next-line no-console
    console.error("[superopen-cline]", message, extra ?? "")
  } catch {
    // Debug logging must stay best-effort.
  }
}

// safeClone turns a hook context into something JSON.stringify accepts.
//
// The contexts Cline passes are live SDK objects, not plain data: they carry functions, class
// instances, and parent references. JSON.stringify throws on a circular reference and on a bigint,
// and silently drops an Error to {}, so each of those is handled here rather than losing the whole
// payload to one bad field.
//
// Cycles are tracked along the current path and released on the way out, so an object that legibly
// appears twice -- the same workspace root referenced from two places -- is kept both times. A
// WeakSet that never released would drop the second occurrence as if it were a cycle.
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

async function sendToSuperopen(payload: Record<string, unknown>, capture: "context" | "replacement" | false = false): Promise<string | undefined> {
  // Flattened before anything else sees it, so every consumer of this function is handed plain
  // data. Doing it further down, on the way into the child process, would leave a live SDK object
  // -- cycles, functions and all -- reaching whatever else is wired up here.
  const safe = safeClone(payload)

  const testSender = (globalThis as Record<symbol, unknown>)[Symbol.for("superopen.cline.testSender")]
  if (typeof testSender === "function") {
    const result = await (testSender as (value: unknown) => unknown)(safe)
    return capture && typeof result === "string" ? result : undefined
  }

  let body: string
  try {
    body = JSON.stringify(safe)
  } catch (err) {
    debugLog("payload could not be serialized", err)
    return
  }
  if (!body) return

  try {
    // Imported lazily so a host without node:child_process fails to send rather than failing to
    // load: a throwing import at module scope would surface to the user as a broken plugin.
    const { spawn } = await import("node:child_process")
    return await new Promise<string | undefined>((resolve) => {
      let settled = false
      let output = ""
      const finish = (context?: string) => {
        if (settled) return
        settled = true
        resolve(context)
      }
      let child
      try {
        const eventName = typeof payload?.type === "string" && payload.type ? payload.type : "tool"
        child = spawn(superopenArgv[0], superopenArgv.slice(1).concat(["--event=" + eventName]), {
          stdio: ["pipe", capture ? "pipe" : "ignore", "ignore"],
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
      }, capture ? promptTimeoutMs : sendTimeoutMs)
      // An unreferenced timer cannot keep a CLI host alive waiting on Superopen telemetry.
      if (typeof timer.unref === "function") timer.unref()
      const done = () => {
        clearTimeout(timer)
        if (capture) {
          try {
            const parsed = JSON.parse(output)
            const value = capture === "context" ? parsed?.additionalContext : parsed?.output_replacement
            finish(typeof value === "string" ? value : undefined)
          } catch { finish() }
        } else finish()
      }
      child.stdout?.on("data", (chunk: { toString(): string }) => {
        output += chunk.toString()
        if (output.length > 65536) { child.kill(); finish() }
      })
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
    // Superopen telemetry must never interrupt Cline execution.
  }
}

// Fields Superopen needs at the top level of the envelope, lifted out of wherever the context keeps
// them.
//
// Best-effort by design: Cline documents the base fields every hook receives, but not where each
// handler's context puts them, so several shapes are tried and a miss costs the field rather than
// the event. The hook adapter reads the same names, so anything missed here is still recoverable
// from the forwarded context.
function asRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function identity(context: Record<string, unknown> | undefined): Record<string, unknown> {
  if (!context) return {}
  const task = asRecord(context.task ?? context.run)
  // The setup context is reported to carry the workspace under an object of its own rather than at
  // the top level. Read in addition to the top level, not instead of it: the two hook surfaces do
  // not agree on shape, and this is the only place the workspace is guaranteed to appear at all --
  // a miss here costs the working directory, the repository and the branch on every event of the
  // task, which is what makes it worth reading more than one spelling.
  const workspace = asRecord(context.workspaceInfo ?? context.workspace_info ?? context.workspace)
  const fields: Record<string, unknown> = {}
  const taskId = context.taskId ?? context.taskID ?? context.task_id ?? task.id ?? task.taskId
  if (taskId) fields.taskId = taskId
  const roots =
    context.workspaceRoots ??
    context.workspace_roots ??
    context.workspaceFolders ??
    workspace.workspaceRoots ??
    workspace.roots
  if (roots) fields.workspaceRoots = roots
  const cwd =
    context.cwd ??
    context.workingDirectory ??
    context.working_directory ??
    workspace.rootPath ??
    workspace.root_path ??
    workspace.cwd ??
    workspace.path
  if (cwd) fields.cwd = cwd
  const version = context.clineVersion ?? context.cline_version ?? context.version
  if (version) fields.clineVersion = version
  return fields
}

function directoryExists(path: string): boolean {
  try { return statSync(path).isDirectory() } catch { return false }
}

function fileExists(path: string): boolean {
  try { return statSync(path).isFile() } catch { return false }
}

function gitPath(root: string, flag: string): string {
  try {
    const path = execFileSync("git", ["-C", root, "rev-parse", flag], { encoding: "utf8", timeout: 1000, stdio: ["ignore", "pipe", "ignore"] }).trim()
    return path ? (isAbsolute(path) ? path : resolve(root, path)) : ""
  } catch { return "" }
}

function managedWorkspace(context: HookContext): boolean {
  const fields = identity(context)
  const start = typeof fields.cwd === "string" ? fields.cwd : ""
  if (!start) return false
  const root = gitPath(start, "--show-toplevel") || start
  if (directoryExists(join(root, ".so"))) return true
  if (!fileExists(join(root, ".git"))) return false
  const gitDir = gitPath(root, "--git-dir")
  const commonDir = gitPath(root, "--git-common-dir")
  if (!gitDir || !commonDir || gitDir === commonDir) return false
  const parent = basename(commonDir) === ".git" ? dirname(commonDir) : commonDir
  return directoryExists(join(parent, ".so"))
}

type HookContext = Record<string, unknown> | undefined

function currentPrompt(context: HookContext): string {
  if (!context) return ""
  for (const candidate of [context.prompt, context.input, context.message]) {
    if (typeof candidate === "string" && candidate.trim()) return candidate
  }
  const snapshot = asRecord(context.snapshot)
  const messages = Array.isArray(snapshot.messages) ? snapshot.messages : []
  const message = asRecord(messages[messages.length - 1])
  if (message.role !== "user") return ""
  if (typeof message.content === "string") return message.content
  if (Array.isArray(message.content)) {
    return message.content.map((part: unknown) => asRecord(part).text).filter((part: unknown) => typeof part === "string").join("\n")
  }
  return ""
}

function runUsage(context: HookContext): Record<string, unknown> {
  const result = asRecord(context?.result)
  const usage = asRecord(result.usage)
  const fields: Record<string, unknown> = {}
  const input = usage.inputTokens ?? usage.totalInputTokens
  const output = usage.outputTokens ?? usage.totalOutputTokens
  if (typeof input === "number" || typeof output === "number") {
    fields.usage = {
      input_tokens: typeof input === "number" ? input : 0,
      output_tokens: typeof output === "number" ? output : 0,
      cache_read_tokens: typeof usage.cacheReadTokens === "number" ? usage.cacheReadTokens : 0,
      cache_creation_tokens: typeof usage.cacheWriteTokens === "number" ? usage.cacheWriteTokens : 0,
    }
  }
  if (typeof usage.totalCost === "number") fields.cost_usd = usage.totalCost
  if (typeof result.outputText === "string") fields.response = result.outputText
  return fields
}

export function createSuperopenPlugin() {
  let base: Record<string, unknown> = {}
  let managed = false

  const forward = async (type: string, context: HookContext, captureContext = false) => {
    if (!managed && !managedWorkspace(context)) return undefined
    try {
      const extra = type === "afterRun" ? runUsage(context) : {}
      return await sendToSuperopen({ ...base, ...identity(context), ...(context ?? {}), ...extra, type }, captureContext ? "context" : false)
    } catch (err) {
      // Unreachable in practice: sendToSuperopen swallows its own failures. Kept because a hook that
      // rejects is a hook that can break the run it was observing.
      debugLog("hook failed", err)
      return undefined
    }
  }

  return {
    name: "superopen-endpoint",
    manifest: { capabilities: ["hooks", "rules"] },

    // The setup context carries the workspace identity for every later hook, so it is captured
    // once rather than re-derived per event.
    setup(api: { registerRule?: (rule: { id: string; content: string; source: string }) => void }, context: HookContext) {
      base = identity(context)
      managed = managedWorkspace(context)
      // Cline's runtime does not consume hook stdout as prompt context. Its rule registry is
      // model-facing and survives compaction, so the graph route belongs here.
      if (managed) api.registerRule?.({
        id: "superopen-graph",
        source: "superopen",
        content: `For codebase questions in this Superopen workspace, start with ${superopenArgv[0]} graph code-search -- '<literal>' for exact flags, keys and errors; use ${superopenArgv[0]} graph query '<question>' for behavior and relationships. Read the listed graph source bodies and use graph snippet for a listed symbol before Grep or full-file Read. For prior-work facts use ${superopenArgv[0]} memory recall '<question>'. For noisy tests or logs use ${superopenArgv[0]} shrink -- <command>; for repeated shell work use ${superopenArgv[0]} blocks scan and blocks run <name>.`,
      })
    },

    hooks: {
      beforeRun: async (context: HookContext) => {
        await forward("beforeRun", context)
        const prompt = currentPrompt(context)
        if (!prompt) return undefined
        const text = await forward("UserPromptSubmit", { ...(context ?? {}), prompt }, true)
        return text ? { appendContext: text } : undefined
      },
      beforeTool: (context: HookContext) => forward("beforeTool", context),
      afterTool: async (context: HookContext) => {
        await forward("afterTool", context)
        if (!managed || !context || asRecord(context.toolCall).toolName !== "run_commands") return undefined
        const result = asRecord(context.result)
        if (result.isError || !Array.isArray(result.output)) return undefined
        let changed = false
        const next = await Promise.all(result.output.map(async (part: unknown) => {
          const row = asRecord(part)
          if (row.success !== true || typeof row.query !== "string" || typeof row.result !== "string" || row.result.length < 2048) return part
          const replacement = await sendToSuperopen({
            ...base, ...identity(context), type: "transformToolResult", tool_name: "bash",
            tool_input: { command: row.query }, tool_response: row.result,
          }, "replacement")
          if (replacement === undefined) return part
          changed = true
          return { ...row, result: replacement }
        }))
        return changed ? { result: { ...result, output: next } } : undefined
      },
      afterRun: (context: HookContext) => forward("afterRun", context),
    },
  }
}

export const SuperopenEndpointPlugin = createSuperopenPlugin()

export default SuperopenEndpointPlugin
