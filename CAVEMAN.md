# Caveman harness, for Superopen

The local half of Caveman: the skill, the compression engine, recovery, the loopback proxy, and the smaller tools. Written 23 October 2026 from [caveman.so](https://caveman.so/), [docs.caveman.so](https://docs.caveman.so/llms.txt), and [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman). This is their published behavior, not their source.

These features land in this repo. Superopen already cuts tokens by answering from a code graph instead of grep. Caveman does not do that. Do not replace the graph. The missing lever is shrinking what the model just read.

Caveman Cloud (gateway, evals, improvements, governance) stays in the OpenLIT enterprise repo, in `CAVEMAN.md` there.

A local token estimate never becomes a verified dollar saving. They keep four labels and do not let wording convert one into another: `inferred` (offline tokenizer or catalog list price), `provider-reported` (the provider’s own usage counters), `benchmark_counterfactual` (a paired run with a pinned method), and `verified` (only Cloud, and only with a causal method). Local tools never mint `verified`.

Install is `npx @caveman-ai/cli`. One command finds installed agents and rewrites their launch so the child process talks to the local proxy. The agent loop stays the agent’s. Declarative profiles currently launch Claude Code, Codex, Gemini CLI, Aider, Hermes, OpenClaw, and OpenCode. The README claims 30+ agents for the skill. `caveman status` shows what is on. `caveman off <rock>` turns one rock off. `caveman doctor` prints one problem per line with a fix.

Six rocks ship on after install. Only Auto needs an account.

## 1. Says less

**What.** A skill that makes the assistant reply shorter. Code, commands, paths, and error strings stay character for character. The user’s own prompt is never rewritten. Adobe’s finding, which they cite, is that rewriting the user prompt makes answers worse.

**How.**

- The skill is instructions in the agent, not a proxy rewrite of the response. Levels are `/caveman`, `/ultracave`, and `/megacave` (Codex uses `$caveman`). `stop caveman` turns it off.
- Rules they publish: answer first, one idea per sentence, at most about 20 words, active voice, one term per thing (they cite ASD-STE100). Articles may go. The words *not*, *never*, *no*, and *only* stay. Numbers and units stay exact. If the caveman phrasing is not shorter, plain English wins. Security warnings, irreversible actions, and a confused user get full sentences, then the short style resumes. No chatter between tool calls.
- The skill text itself costs input tokens (they measured about 1,160). A short task can cost more in total. They compare against “be concise,” not against a silent baseline, because current models already know “be concise.”
- Install paths: `npx skills add JuliusBrussee/caveman -g` for the skill alone; a Claude Code plugin that auto-starts; a Gemini extension; Claude account plugin for web, phone, and Cowork (they say the last is not tested end to end).
- Related slash commands, same skill family: `/caveman-commit` (one-line conventional commit), `/caveman-review` (one finding per line), `/caveman-compress <file>` (shrink a memory file, back up the original; this one sends the file to the configured model), `/caveman-stats`, `/caveman-help`. Work-pattern skills the agent may pick up: investigate-first, lean-build, surgical-patch, safe-refactor, migration, verify-and-stop. `cavecrew` is subagents that search, edit, and review, then report back in the short style.

**Take it as an opt-in skill.** Do not turn it on by default. Superopen has already lost a paired run because always-on injected text cost more than it saved. Ship it only after a paired run shows fewer provider-reported tokens in total.

## 2. Reads less (Engine)

**What.** Logs, JSON, CSV, YAML, diffs, test output, and similar blobs are transformed before the model reads them. The original stays on disk. If the transform is not smaller, not parseable, or cannot store a recovery, the original bytes are what get sent.

**How.** Engine is a Go library (`Compress`, `Retrieve`, `Detect`, `Stats`, plus `Simulate` which does not commit). Pipeline:

1. Detect the shape.
2. Pick a compressor.
3. Transform.
4. If the result is not smaller and valid, emit the original.
5. If the transform is lossy, store the exact original first. If the store fails, emit the original.
6. Emit the compact bytes plus a handle.

`record` mode always passes input through. Unknown modes return the original. CLI input cap is 64 MiB. Above that the command fails with `cave_input_too_large`. It does not truncate.

Automatic detection covers JSON, JUnit / pytest-json-report / Jest (only if at least one test parses), terminal output, diffs, HTML, tables, source, logs, search results, config text, and general text. TOON, accessibility trees, tool-schema transforms, and repetition transforms are explicit-only. They are not auto-selected, because a wrong pick would be unsafe.

The default registry has 16 compressors: JSON, log, code, diff, search result, text, HTML, tabular, configuration, tool schema, tool-schema annotations, TOON, accessibility tree, repetition, terminal output, test report. The test-report compressor keeps the counts and every failure. The code compressor keeps imports, signatures, and type declarations and drops function bodies. Release `caveman-proxy` and `caveman-engine` are cgo builds and parse Go, Python, TypeScript, JavaScript, Rust, Java, C, and C++ with tree-sitter. A pure-Go build parses Go only. GDScript is handled by a line scanner. Every compressor is classed S4 (lossy) even when a particular input happens to round-trip. Callers must not treat a compressor name as a byte-safety promise.

Local token counts use an offline `o200k_base` tokenizer, or a character estimate if that is missing. Those counts are `inferred`. They are not the provider’s bill.

Their published session number is 33.2% fewer input tokens across six tasks, three paired runs each, 18 of 18 answers still right (885,793 to 591,673). They also say the session run is from August 2026 on an older engine, HTML had no compressor so that cell got worse, and each fixture hid one record in a large noise file. A headroom setting on the same suite was 15 of 18 right and only 6.7% fewer tokens.

**Take this first.** Prefer a hook that replaces the tool result the agent will read, and keep the original under `.so/`. Add a loopback proxy only for agents whose hooks never see those bytes. Superopen is one binary, no daemon, and hooks fail open. A proxy on every request fights that.

## 3. Context recovery (CCR)

**What.** The exact original bytes behind a short handle, so a lossy compress can still be undone.

**How.**

- Byte handles are `ccr_` plus the first 16 bytes of SHA-256, hex. Identical bytes share a handle. The handle is an identifier. It is not encryption and not an authorization token.
- A record stores the exact bytes, content type, compressor metadata, original and compact sizes, and local timestamps.
- Typed objects use `ccr_obj_` and `ccr://` pointers so a client can fetch one field. A bad pointer fails. It does not guess.
- Native store is SQLite, normally `~/.caveman/ccr.db`. Proxy measurements use a separate `~/.caveman/caveman.db`. WebAssembly keeps an in-memory store that dies with the process.
- Default cap is 512 MiB (`CAVEMAN_CCR_MAX_BYTES`). At the cap, new lossy transforms pass the original through. Existing handles are never evicted, so an old handle cannot dangle.
- The store watches database and journal file identity. If the file is replaced under a live process, the store goes terminal (`recovery storage changed`) until the process restarts. It will not adopt a swapped file. This is their fix for a WAL/orphaned-descriptor bug.
- Recovery proves the source is still there. It does not prove the compressed context had the same quality.

Retrieve with `caveman tools retrieve ccr_…` or the MCP recovery tool.

**Take it beside the compressor.** Same local store. Do not upload originals.

## 4. Local proxy

**What.** A loopback reverse proxy. The wrapped agent’s provider base URL points at it. The proxy runs Engine, forwards to the real provider with the user’s own key, and writes local usage rows.

**How.**

- Binds to loopback. Assumes one trusted user. Credentials stay the user’s. A request is not sent to Caveman unless the user signed in and the request is explicitly routed to a Cloud model.
- Modes include compress and off (`caveman wrap --off` is byte-identical local metering). `caveman wrap --pixel` renders dense text as an image for a vision model (TOON and pixel are separate encodings; pixel is explicit).
- Failure: if a transform fails, the original request continues. They describe the proxy as failing open so a caveman fault does not block the agent.
- Team deploy exists: the `caveman-proxy` binary in Docker, Kubernetes, ECS, Cloud Run, or Fly, with auth, TLS, and identity. That is still the local product, shared by a team. It is not Caveman Cloud.
- Cache planner: on the local proxy, it adds provider-native prompt-cache markers where the request shape and the economics support them. It inspects the request, finds a stable prefix, and adds the hint. It does not rewrite model-visible text for that hint.

**Take it only as the fallback** when hooks cannot see tool bytes. The shared-team proxy is a later deploy question, not the first slice.

## 5. Picks the model (Auto)

**What.** A model named Auto in the agent’s picker. Caveman chooses the model and the effort for that turn. A hard turn stays on the frontier model. If the router is missing, slow, or fails, the agent’s own request runs unchanged.

**How.**

- Requires `caveman login` (free account). Unsigned, nothing is sent and Auto does not appear.
- What they say is sent, only after sign-in and only when Auto is picked: the latest ask (including whatever the agent attached, such as CLAUDE.md or an @-mentioned file), the previous ask, the end of the last reply, tool names, effort, a few agent headers, and the previous request’s token counts. On the free plan they may keep that to improve routing. `caveman off routing` stops it.
- Optional `caveman providers add openai` adds models on the user’s own key. `caveman providers cloud off` stops any request from being proxied through Caveman Cloud.
- Their published number: 85 tasks from SWE-Interact and SWE-Together, one try each, routed Claude Code versus Opus 5.5 at max effort. They report about 0.46× the cost and a solve rate inside the noise (±7 points at that sample). They call it interim, list-price, and mostly effort reduction on easy asks. Someone already on Opus at medium should expect less. Not measured.

**Do not port this the way Caveman does it.** Superopen does not upload prompts. If routing is ever added, it uses the user’s own keys and stays off until they turn it on. Gateway `auto` routing is a different mechanism and belongs in OpenLIT.

## 6. Blocks

**What.** The agent writes a script, throws it away, and writes it again next session. Blocks keep that script in the repo, check it on pull requests, and hand it back. When the agent starts typing a script a block already covers, it gets one line telling it to use the block. The command is never denied or rewritten.

**How.** Pre-release (they cite v0.1.0-rc.5). Python only. No sandbox yet. Separate repo `caveman-ai/blocks`. Eight built-ins: `json-peek`, `jsonl-stats`, `test-summary`, `first-error`, `wait-for`, `grep-defs`, `http-json`, `replace-in-file`. `caveman-blocks scan` ranks retyped scripts from local history; script text does not leave the machine. `caveman-blocks init` writes rules and an index into the repo’s AGENTS.md. They cite one week on one machine where the same script was written 394 times, and Purdue’s ceiling of 9.57% of task cost. They also publish a row where `grep-defs` loses to a plain grep, and they leave that row red.

**Take it next to harvest.** Harvest already proposes a playbook patch and waits for `so harvest apply`. Blocks are the same human gate, for scripts instead of AGENTS.md.

## 7. Smaller local rocks

**Learn.** `caveman learn` ranks where tokens went, from agent history already on disk. `caveman learn implement` applies fixes one diff at a time, only on yes. They say this is not measured yet.

**Warm cache.** If the user steps away, Anthropic forgets the prompt cache and the next turn pays full input price. Caveman sends keep-warm requests only when that user’s own history says they return in time. Every warm is listed in `caveman stats`. Not measured yet. This is a provider call, so it spends money to save a later call.

**Browse.** `caveman browse <url>` returns a compressed page. Their bench: a Playwright snapshot of a 200-row table went from 15,704 tokens to 121. Tiny forms got worse (2.3×). The loss stays in the bench.

**Memory-file compress.** `/caveman-compress` shrinks CLAUDE.md-style files and keeps a backup. Their bench: five files, 6,198 tokens to 4,138, headings, code blocks, and paths intact. This path sends the file to the configured model.

**Shrink CLI.** `caveman shrink -- pnpm test` compresses one command’s noisy output.

**Trial.** `caveman trial -- claude` A/B’s a real session on the user’s own work.

**Convert.** `caveman convert --dry-run` is the pixel path: skills the model reads as images.

**Stats.** `caveman stats` is local token history.

**Subagents.** A read-only explorer and compact delegation guides. The point is to keep the search transcript out of the main context and return only the evidence.

**MCP.** A local MCP server exposes recovery, memory, browse, and shrink without an account.

**Memory store.** `caveman tools mem`. Local. Not Caveman Cloud.

**Middleware and SDKs.** `@caveman-ai/middleware` and `@caveman-ai/sdk` (TypeScript), `caveman-sdk` (Python). `withCaveman` wraps a Vercel AI SDK `streamText` call. A local runtime sits at `127.0.0.1:8787` in `compress` mode. Scope is namespace, session id, branch, cache epoch. They list adapters for Vercel AI SDK, OpenAI, Anthropic, Google GenAI, LangChain, Strands, Mastra, and MCP, and recipes for LiteLLM, CrewAI, Pydantic AI, and raw HTTP. The app’s framework, provider, keys, and saved history stay. The middleware is where tool results become model input.

**Telemetry.** On by default in an interactive terminal: commands run, token counts, blocks usage counts, a random install id, OS, and IP. Not prompts, code, paths, or scripts. `caveman telemetry off` or `DO_NOT_TRACK=1` disables it, including Blocks.

**Take learn, browse, shrink, and memory-file compress after reads-less works.** Warm cache spends provider money; leave it until a proxy exists. Middleware for apps the user builds is not a Superopen feature. It is a later SDK if a customer asks.

## Claims worth beating

Use these as benchmarks, not as copy for a marketing page. Layers in their savings calculator are multiplied. They were not measured together.

| Claim | Their caveat |
| --- | --- |
| 33.2% fewer input tokens, 18 of 18 right | Older engine, August 2026, HTML cell lost, fixtures are one record in a large file. 95% interval on the token cut is 14.6% to 48.5%. |
| About 65% less readable output, about 10% of a long coding session | The 10% is their estimate, because code and tool calls dominate and the voice leaves those alone. “Be concise” is the real baseline. Skill input can erase the saving on a short task. |
| 0.46× cost versus always-Opus at max effort, same solve rate | 85 tasks, one try, interim, list price, gap inside ±7 points. Mostly lower effort on easy asks. Requires sending the ask to their cloud. |
| Blocks, up to 9.57% of task cost | Purdue’s published ceiling, not their measurement. Pre-release. One of eight blocks loses to plain grep. |
| Adobe 1.4× to 2.4× cheaper | Caveman-style output, eight models. They cite it. They also cite JetBrains: 82 paired tasks, no detectable quality difference, 8.5% fewer output tokens, total cost varied. |
| Elastic 63.6% fewer response tokens | Elastic’s own port, eight MCP scenarios. Not the upstream repo. |

## Order to take them

1. Shrink noisy tool results in the hook path. Keep the original locally. Measure net provider-reported tokens.
2. Opt-in shorter-reply skill. Off by default until a paired run wins.
3. Reusable scripts beside harvest. The user applies them.
