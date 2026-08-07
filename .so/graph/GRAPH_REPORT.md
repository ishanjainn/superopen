# Graph Report - /var/folders/fr/660k4_4n1gs3g_d3c939lcm40000gn/T/so-graphify-100757902  (2026-08-07)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 3236 nodes · 7680 edges · 168 communities (146 shown, 22 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 962 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- release-packages.py
- Store
- Run
- cn
- Remove
- InstallVendor
- Resolve
- entitlement.go
- LLMTurn
- memory.ts
- CityScene.tsx
- projectIdFromRequest
- harness-yaml.ts
- sessions.ts
- sessions/[id]/page.tsx
- misc.ts
- trace.ts
- Config
- Paths
- Store
- codex/handle.go
- session-timeline.tsx
- types.ts
- fileExists
- repoCwd
- agentlinks.go
- playground-shell.tsx
- receiver.go
- title.go
- soPath
- file-content-view.tsx
- .Parse
- Input
- blame.go
- Context
- handle
- .Create
- recommend.go
- compilerOptions
- shell/sidebar.tsx
- MapView.tsx
- attrs.go
- emitGitArtifactsCodex
- guardrails.go
- home
- graph.go
- evals.ts
- backend.go
- html/route.ts
- sessionTraceContext
- attributes.go
- Out
- MineTranscript
- Hud.tsx
- cli.go
- Refresh
- codex/transcript.go
- Emitter
- utils.ts
- PortableSession
- NewPortableSession
- session-search-bar.tsx
- citymap.ts
- reducer.ts
- devDependencies
- run
- HarnessID
- cmds_extra.go
- dependencies
- Load
- cmds_dev.go
- NewEmitter
- Run
- .Port
- vendor_config_scrub_test.go
- agentconfig/config.go
- handle
- Lookup
- codex/handle_test.go
- ResolveForVendor
- handle
- RemoveAll
- .Write
- harness_hooks.go
- stripCursorHooks
- Rebuild
- NewStore
- Init
- prefs.ts
- .paths
- redact_test.go
- NewStore
- harness-files-page.tsx
- AgentsPanel.tsx
- .Parse
- Default
- rec
- Prune
- theme-provider.tsx
- Classify
- inputBuilder
- DefaultPolicy
- TestPiHostCostPreferred
- Ledger
- npm/package.json
- Timeline.tsx
- claudecode/transcript.go
- defaultSampler
- session/store.go
- treeLayout.ts
- dirLabels.ts
- citymap.go
- scripts
- inputBuilder
- marketplace/plugins/opencode/superopen.ts
- launch.go
- EstimateTokens
- plugins/opencode/superopen.ts
- Config
- marketplace/plugins/pi/index.ts
- Display
- plugins/pi/index.ts
- install.sh script
- adapter
- dev_proc_unix.go
- dev_proc_windows.go
- harness.go
- axi_test.go
- TestVendorsFromArg
- So
- install.go
- web/package.json
- process_windows.go
- TestResolveAndEnsureDirs
- TestLocalJSONLWriteQuery
- tailwind.config.ts
- Client
- cmdk
- github.com/ishanjainn/superopen
- @radix-ui/react-dialog
- @radix-ui/react-slot
- react-dom
- sync-plugins.sh
- three
- @types/react
- eslint.config.mjs
- next.config.mjs
- next-env.d.ts
- postcss.config.mjs
- vitest.config.mts

## God Nodes (most connected - your core abstractions)
1. `Paths` - 109 edges
2. `Resolve()` - 80 edges
3. `cn()` - 70 edges
4. `fileExists()` - 63 edges
5. `projectIdFromRequest()` - 52 edges
6. `soPath()` - 44 edges
7. `runWithProject()` - 39 edges
8. `repoRoot()` - 38 edges
9. `readText()` - 37 edges
10. `main()` - 36 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `ExitCode()`  [INFERRED]
  cmd/so/main.go → internal/axi/axi.go
- `runDev()` --calls--> `Resolve()`  [INFERRED]
  cmd/so/cmds_dev.go → internal/harness/harness.go
- `runDevForeground()` --calls--> `NewReceiver()`  [INFERRED]
  cmd/so/cmds_dev.go → internal/otlp/receiver.go
- `runDevForeground()` --calls--> `FanoutLocalRemote()`  [INFERRED]
  cmd/so/cmds_dev.go → internal/otlpremote/remote.go
- `runDevForeground()` --calls--> `Remove()`  [INFERRED]
  cmd/so/cmds_dev.go → internal/projects/registry.go

## Import Cycles
- None detected.

## Communities (168 total, 22 thin omitted)

### Community 0 - "release-packages.py"
Cohesion: 0.06
Nodes (46): Any, apply_version_files(), body_for_component_evidence(), build_pr_evidence(), bump(), chunk_prs(), collect_prs(), collect_single_pr() (+38 more)

### Community 1 - "Store"
Cohesion: 0.06
Nodes (50): Event, SoftWrite, Append(), Time, List(), Path(), splitLines(), Applyable() (+42 more)

### Community 2 - "Run"
Cohesion: 0.05
Nodes (63): AgentSource, GraphSummary, Profile, Options, Report, BuildProfile(), cleanMD(), CollectAgentFiles() (+55 more)

### Community 3 - "cn"
Cohesion: 0.05
Nodes (56): EVAL_COLS, EvalRow, EvalsDashboardView(), EvaluationsInner(), fmtCost(), fmtTokens(), LOG_COLS, LogFilter (+48 more)

### Community 4 - "Remove"
Cohesion: 0.06
Nodes (55): InstallOptions, InstallResult, UninstallResult, Apply(), Brief(), EnsureGlobalSkill(), EnsureSkills(), fileExists() (+47 more)

### Community 5 - "InstallVendor"
Cohesion: 0.07
Nodes (50): cursorHooksFile, hookBinaryAvailable(), Install(), Status(), T, TestStatusClaudeCodeDetectsStaleBinaryPath(), TestStatusClaudeCodeMissingManifest(), TestStatusClaudeCodeOKWithValidBinaryPath() (+42 more)

### Community 6 - "Resolve"
Cohesion: 0.12
Nodes (51): cmdCheckpoint(), cmdGitHook(), cmdImport(), cmdProjects(), cmdStatus(), cmdHarvest(), attachSessionsStart(), cmdAudit() (+43 more)

### Community 7 - "entitlement.go"
Cohesion: 0.06
Nodes (35): Status, Clear(), CloudOTLPEnabled(), configDir(), Time, Load(), LoginPaid(), path() (+27 more)

### Community 8 - "LLMTurn"
Cohesion: 0.06
Nodes (24): recordingEmitter, recordingEmitter, recordingEmitter, Session, ToolCall, Session, ToolCall, Session (+16 more)

### Community 9 - "memory.ts"
Cohesion: 0.11
Nodes (46): dynamic, GET(), POST(), runtime, Lesson, MemoryPage(), Pack, Section (+38 more)

### Community 10 - "CityScene.tsx"
Cohesion: 0.09
Nodes (41): useThemeOptional(), applyCitySceneTheme(), attentionColumnGeometry(), attentionHeight(), baseColor(), centerFor(), CityScene(), colors (+33 more)

### Community 11 - "projectIdFromRequest"
Cohesion: 0.09
Nodes (39): GET(), Body, dynamic, POST(), runtime, dynamic, GET(), runtime (+31 more)

### Community 12 - "harness-yaml.ts"
Cohesion: 0.10
Nodes (40): FileContentView(), Domain, FILE, HarnessItemDetailPage(), hydrate(), KIND_BLURB, CONFIG, EvaluationComposer() (+32 more)

### Community 13 - "sessions.ts"
Cohesion: 0.09
Nodes (44): dynamic, GET(), runtime, CHAT_ATTR_KEYS, clearAgentLink(), clearFalseNesting(), codexRolloutUpdatedAt(), countCheckpointDirs() (+36 more)

### Community 14 - "sessions/[id]/page.tsx"
Cohesion: 0.07
Nodes (35): GraphPage(), MapView, NestedSession, projectFromURL(), SessionDetailInner(), Tab, TabButton(), dateGroupLabel() (+27 more)

### Community 15 - "misc.ts"
Cohesion: 0.10
Nodes (40): DELETE(), dynamic, GET(), runtime, dynamic, GET(), runtime, currentRepoMeta() (+32 more)

### Community 16 - "trace.ts"
Cohesion: 0.10
Nodes (40): agentLabel(), buildAgentGraph(), eventCountFor(), linkMethodFor(), readMeta(), SessionMetaFile, sessionKey(), readText() (+32 more)

### Community 17 - "Config"
Cohesion: 0.08
Nodes (25): Config, CostConfig, EvalsConfig, ExporterConfig, GraphConfig, GuardrailsConfig, InjectConfig, LLMConfig (+17 more)

### Community 18 - "Paths"
Cohesion: 0.14
Nodes (37): Paths, Delta, ledgerEntry, ledgerFile, pendingFile, Result, Trigger, maybeHarvestOnSessionEnd() (+29 more)

### Community 19 - "Store"
Cohesion: 0.14
Nodes (13): IndexEntry, LoadMap(), ListItem, Store, IsEmptyListItem(), Client, Store, Span (+5 more)

### Community 20 - "codex/handle.go"
Cohesion: 0.12
Nodes (39): codexPayload, boolField(), buildSession(), buildToolCall(), canonicalStatus(), clearTurnFragment(), commandFromToolInput(), durationMsOr() (+31 more)

### Community 21 - "session-timeline.tsx"
Cohesion: 0.08
Nodes (33): basename(), buildTimeline(), buildTimelineFromPortableTurns(), ChatMinimap(), classifyTool(), decodeFiltersParam(), DEFAULT_FILTERS, encodeFiltersParam() (+25 more)

### Community 22 - "types.ts"
Cohesion: 0.07
Nodes (34): AgentKind, AgentLinkMethod, AgentLinkQuality, AgentStatus, CityDir, Coverage, DimensionName, Observability (+26 more)

### Community 23 - "fileExists"
Cohesion: 0.12
Nodes (30): GET(), dynamic, GET(), POST(), runtime, dynamic, GET(), runtime (+22 more)

### Community 24 - "repoCwd"
Cohesion: 0.13
Nodes (28): dynamic, POST(), runtime, dynamic, POST(), runtime, DELETE(), PUT() (+20 more)

### Community 25 - "agentlinks.go"
Cohesion: 0.15
Nodes (33): Entry, fileDoc, pendingDoc, PendingSpawn, AllowRegister(), ClaimPending(), DiscoverCursorParent(), ExtractAgentID() (+25 more)

### Community 26 - "playground-shell.tsx"
Cohesion: 0.11
Nodes (26): BreadcrumbContext, BreadcrumbContextValue, BreadcrumbCrumb, BreadcrumbProvider(), useBreadcrumb(), HeaderContextRow(), Option, pageTitle() (+18 more)

### Community 27 - "receiver.go"
Cohesion: 0.11
Nodes (28): AnyValue, anyValueString(), KeyValue, Span, Store, IsSubagentAttrs(), kvMap(), NewReceiver() (+20 more)

### Community 28 - "title.go"
Cohesion: 0.12
Nodes (30): humanizePromptPreview(), DisplayName(), EnsureTitle(), generateTitle(), Client, Meta, loadCodexTitles(), loadOpenCodeTitles() (+22 more)

### Community 29 - "soPath"
Cohesion: 0.12
Nodes (31): dynamic, formatYamlValue(), GET(), isTopLevelKey(), PatchBody, PUT(), runtime, setYamlPath() (+23 more)

### Community 30 - "file-content-view.tsx"
Cohesion: 0.08
Nodes (29): GuardrailsDashboardView(), buildEvalRows(), buildGuardRows(), EVAL_COLUMNS, EVAL_VISIBILITY, EvalColumnKey, EvalRow, EvalRowKind (+21 more)

### Community 31 - ".Parse"
Cohesion: 0.11
Nodes (18): ClaudeImport, CodexExport, CodexImport, SessionRef, peekClaude(), codexCallArgs(), codexMessageText(), codexRoot() (+10 more)

### Community 32 - "Input"
Cohesion: 0.15
Nodes (28): claudePayload, Emitter, CountInlineDiff(), splitLines(), bashStdout(), countMultiEditLines(), drainAssistantTurns(), drainRejectedPendingEdits() (+20 more)

### Community 33 - "blame.go"
Cohesion: 0.09
Nodes (24): LineInfo, WhyResult, File(), Store, isHex(), sessionIDForCommit(), Why(), AppendTrailer() (+16 more)

### Community 34 - "Context"
Cohesion: 0.09
Nodes (17): adapter, adapter, Adapter, Context, NormalizeRepoURL(), remoteURL(), run(), Snapshot() (+9 more)

### Community 35 - "handle"
Cohesion: 0.16
Nodes (29): cursorAttachment, cursorEdit, cursorPayload, agentIDFromPath(), applyAccumulatedTokens(), buildPromptTurn(), buildResponseTurn(), buildSession() (+21 more)

### Community 36 - ".Create"
Cohesion: 0.13
Nodes (19): Meta, Store, containsDotDot(), Time, NewStore(), splitSlash(), stringsHasDotDot(), T (+11 more)

### Community 37 - "recommend.go"
Cohesion: 0.20
Nodes (27): advisoryGuardrailBody(), Apply(), containsStr(), Dismiss(), EnqueuePending(), FingerprintKey(), Generate(), Result (+19 more)

### Community 38 - "compilerOptions"
Cohesion: 0.07
Nodes (28): dom, dom.iterable, esnext, .next/dev/types/**/*.ts, next-env.d.ts, .next/types/**/*.ts, node_modules, **/*.ts (+20 more)

### Community 39 - "shell/sidebar.tsx"
Cohesion: 0.12
Nodes (22): FeaturePageHeader(), FeaturePageHeaderProps, iconForPath(), iconForTitle(), isActive(), NavigationLink(), PrimaryItem(), SectionPanel() (+14 more)

### Community 40 - "MapView.tsx"
Cohesion: 0.15
Nodes (24): apiURL(), describeError(), getAgentTrace(), getJSON(), getSessionAgents(), getSessionReport(), getSessionSnapshot(), listSessions() (+16 more)

### Community 41 - "attrs.go"
Cohesion: 0.19
Nodes (23): scrubFn, bodyAllowed(), buildInputMessagesJSON(), buildOutputMessagesJSON(), Session, Span, ToolCall, inferProvider() (+15 more)

### Community 42 - "emitGitArtifactsCodex"
Cohesion: 0.17
Nodes (26): PatchLineCounts, CountPatchLines(), ExtractCommitMessage(), ExtractCommitSHA(), ExtractPRTitle(), ExtractPRURLAndNumber(), firstGroup(), IsGitCommit() (+18 more)

### Community 43 - "guardrails.go"
Cohesion: 0.15
Nodes (20): Decision, Engine, File, Policy, Rule, CheckCommandString(), CheckPathString(), EnsureDefaults() (+12 more)

### Community 44 - "home"
Cohesion: 0.16
Nodes (9): ClaudeExport, OpenCodeExport, OpenCodeImport, encodeClaudeProjectDir(), home(), exportOpenCodeSQLiteJSON(), opencodeDataDir(), ExportResult (+1 more)

### Community 45 - "graph.go"
Cohesion: 0.17
Nodes (25): Result, stubGraph, Build(), buildStub(), buildWithGraphify(), copyDir(), countGraph(), ensureGraphifyCommunitySidecars() (+17 more)

### Community 46 - "evals.ts"
Cohesion: 0.15
Nodes (25): buildDailySeries(), buildEvaluatorStats(), dayKey(), EvalBadge, EvalFailurePoint, EvalRun, evalsConfigPath(), evaluationScope() (+17 more)

### Community 47 - "backend.go"
Cohesion: 0.13
Nodes (16): ListItem, ComputeAttribution(), ListItem, Meta, Store, Time, metaHasCommit(), metaHasPR() (+8 more)

### Community 48 - "html/route.ts"
Cohesion: 0.16
Nodes (19): dynamic, GET(), HEAD(), loadGraphHtml(), runtime, themeFromRequest(), GraphHtmlStatus, inspectGraphHtml() (+11 more)

### Community 49 - "sessionTraceContext"
Cohesion: 0.19
Nodes (21): sessionIDGenerator, sessionRootMarker, sessionRootMarkerKey, InMemoryExporter, deriveSessionRootSpanID(), deriveSessionTraceID(), randomSpanID(), randomTraceID() (+13 more)

### Community 50 - "attributes.go"
Cohesion: 0.17
Nodes (18): KeyValue, Span, NewAttributeBuilder(), SetBoolAttribute(), SetFloat64Attribute(), SetInt64Attribute(), SetIntAttribute(), SetJSONAttribute() (+10 more)

### Community 51 - "Out"
Cohesion: 0.15
Nodes (13): Error, Flags, Out, Bind(), chainPreRun(), envTruthy(), ExitCode(), Fail() (+5 more)

### Community 52 - "MineTranscript"
Cohesion: 0.16
Nodes (18): Config, mineOnFinalize(), heuristicSkillBody(), MineSessionFile(), MineTranscript(), slugify(), T, TestMineTranscriptWritesLessonAndRec() (+10 more)

### Community 53 - "Hud.tsx"
Cohesion: 0.11
Nodes (12): SessionRail(), SessionRailDrawer(), SessionRailProps, SessionRailTool, ActionCounts, MetricObservability, ACTION_ORDER, ChurnEntry (+4 more)

### Community 54 - "cli.go"
Cohesion: 0.17
Nodes (19): claudeEnvelope, Result, Runner, codexExecArgs(), codexModel(), Detect(), DetectAll(), ensureWorkDir() (+11 more)

### Community 55 - "Refresh"
Cohesion: 0.17
Nodes (20): maxSharedMtime(), startRefreshWatcher(), writeRefreshStatus(), gitSHA(), Time, indexableFilesChanged(), isIndexablePath(), loadRefreshMarker() (+12 more)

### Community 56 - "codex/transcript.go"
Cohesion: 0.20
Nodes (22): codexLine, codexReasoningItem, codexReasoningSummaryPart, codexTokenSnapshot, codexTokenUsage, codexTokenUsageInfo, sessionMeta, RawMessage (+14 more)

### Community 57 - "Emitter"
Cohesion: 0.15
Nodes (12): Emitter, perEventSpansAllowed(), Mutex, Time, ToolCall, commonAttrs(), KeyValue, initMetrics() (+4 more)

### Community 58 - "utils.ts"
Cohesion: 0.15
Nodes (15): actorLabel(), decisionDialogTitle(), decisionLabel(), decisionPlaceholder(), RecDetailPage(), StatusPill(), BACKENDS, SafeConfig (+7 more)

### Community 59 - "PortableSession"
Cohesion: 0.12
Nodes (16): wsCollector, SessionRef, observeOpenCodeToolPart(), capCommands(), capStrings(), extractExitCode(), matchesAny(), newWSCollector() (+8 more)

### Community 60 - "NewPortableSession"
Cohesion: 0.24
Nodes (17): T, TestMaybeInjectMemory_PortResumeWithoutMemory(), NewPortableSession(), ArmResume(), consumeLegacyCursorPending(), ConsumePendingResume(), consumeSOPortPending(), portRunDir() (+9 more)

### Community 61 - "session-search-bar.tsx"
Cohesion: 0.20
Nodes (17): joinQuery(), KNOWN_AGENTS, KNOWN_TOOLS, Props, SessionSearchBar(), splitQuery(), displayUser(), emptySessionQuery() (+9 more)

### Community 62 - "citymap.ts"
Cohesion: 0.14
Nodes (21): applyTreemap(), buildTree(), cachePath(), capAspect(), CityDir, CityFile, CityMap, computeWeight() (+13 more)

### Community 63 - "reducer.ts"
Cohesion: 0.17
Nodes (16): FilePlayback, PlaybackEngine, touchRank, CitySceneProps, TreeSceneProps, CityMap, Target, Touch (+8 more)

### Community 64 - "devDependencies"
Cohesion: 0.10
Nodes (21): autoprefixer, eslint, eslint-config-next, postcss, tailwindcss, @types/node, @types/react-dom, @types/three (+13 more)

### Community 65 - "run"
Cohesion: 0.18
Nodes (19): peekedContext, canonicalVendor(), firstHookEnv(), firstNonEmpty(), foreignParentID(), Adapter, isClaudeCodeVendor(), isRealClaudeCodeInvocation() (+11 more)

### Community 66 - "HarnessID"
Cohesion: 0.17
Nodes (12): ClaudeCode(), Codex(), portToHub(), NewRegistry(), RegisterAll(), DefaultLedgerPath(), Time, ExportAdapter (+4 more)

### Community 67 - "cmds_extra.go"
Cohesion: 0.15
Nodes (17): cmdBlame(), cmdLogin(), cmdLogout(), cmdWhy(), extendSessionsCmd(), firstLine(), Meta, harnessIDFromVendor() (+9 more)

### Community 68 - "dependencies"
Cohesion: 0.11
Nodes (19): class-variance-authority, clsx, lucide-react, next, @radix-ui/react-tooltip, react, react-is, recharts (+11 more)

### Community 69 - "Load"
Cohesion: 0.25
Nodes (18): markCodexSessionRootEmitted(), accumulateStopTokens(), AddPendingEdit(), BumpCodeCounters(), DrainPendingEdits(), GC(), Duration, Time (+10 more)

### Community 70 - "cmds_dev.go"
Cohesion: 0.24
Nodes (17): cmdDev(), logFile(), maybeOpenUI(), pidFile(), readDevStatus(), runDev(), runDevForeground(), runDir() (+9 more)

### Community 71 - "NewEmitter"
Cohesion: 0.20
Nodes (15): detectHostFromBinary(), detectHostFromProcessTree(), drainCodeCounters(), drainCounters(), Session, Tracer, isSessionStart(), NewEmitter() (+7 more)

### Community 72 - "Run"
Cohesion: 0.21
Nodes (16): activitySignals, Result, appendHistory(), collectActivitySignals(), containsAny(), extractJSON(), Config, Span (+8 more)

### Community 73 - ".Port"
Cohesion: 0.14
Nodes (11): Event, RefreshMemoryAfterPort(), Orchestrator, SessionRef, Orchestrator, RemapCWD(), Event, HubFactory (+3 more)

### Community 74 - "vendor_config_scrub_test.go"
Cohesion: 0.25
Nodes (16): FileMode, writeFileAtomic(), stripClaudeMarketplaceJSON(), stripCodexConfigTOML(), stripCodexOwnedSections(), T, mustJSONRead(), mustJSONWrite() (+8 more)

### Community 75 - "agentconfig/config.go"
Cohesion: 0.23
Nodes (15): Defaults, Flags, Resolved, builtinDefaults(), configDir(), isLocalHost(), isSecretKey(), Load() (+7 more)

### Community 76 - "handle"
Cohesion: 0.19
Nodes (13): accumulate(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), itoa() (+5 more)

### Community 77 - "Lookup"
Cohesion: 0.23
Nodes (14): Lookup(), T, TestCostBasic(), TestCostCacheCreationFallback(), TestCostCacheCreationPremium(), TestCostWithCacheRead(), TestCostZeroRate(), TestLookupAnthropic() (+6 more)

### Community 78 - "codex/handle_test.go"
Cohesion: 0.27
Nodes (15): applyPatchBody(), patchLiteralFromCodeMode(), T, TestApplyPatchBodySupportsCodexHookCommand(), TestApplyPatchBodyUnwrapsCodeMode(), TestCodeModeApplyPatchEmitsFileDecisions(), TestCodexEndToEndOneTurn(), TestCodexMetadataModeDropsBodies() (+7 more)

### Community 79 - "ResolveForVendor"
Cohesion: 0.23
Nodes (13): base64URLDecode(), claudeCodeEmail(), codexEmail(), decodeJWTSegment(), emailFromJWT(), FromGitConfig(), ResolveForVendor(), T (+5 more)

### Community 80 - "handle"
Cohesion: 0.21
Nodes (11): accumulateUsage(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), stampUsage() (+3 more)

### Community 81 - "RemoveAll"
Cohesion: 0.27
Nodes (13): Writer, NewCmd(), RemoveAll(), run(), claudeMarketplaceRoot(), claudePluginCacheRoot(), disableClaudeCodePlugin(), disableCodexPlugin() (+5 more)

### Community 82 - ".Write"
Cohesion: 0.23
Nodes (4): CursorExport, CursorImport, loadWorkingStateSidecar(), writeTranscript()

### Community 83 - "harness_hooks.go"
Cohesion: 0.25
Nodes (13): Decision, extractToolTargets(), findRepoRoot(), isSessionEndEvent(), isSessionStartEvent(), isToolGateEvent(), isTurnBoundaryHarvestEvent(), maybeAuditApproval() (+5 more)

### Community 84 - "stripCursorHooks"
Cohesion: 0.29
Nodes (12): RawMessage, isOurHookEntry(), stripCursorHooks(), T, TestStripCursorHooks_DeletesFileWhenOnlyOursAndVersion(), TestStripCursorHooks_KeepsFileWhenOtherTopLevelKeysExist(), TestStripCursorHooks_MissingFileIsNoOp(), TestStripCursorHooks_NoOpWhenNothingOurs() (+4 more)

### Community 85 - "Rebuild"
Cohesion: 0.22
Nodes (11): QueryRetrieve(), indexPath(), Rebuild(), Search(), snippetAround(), T, TestRebuildAndSearch(), Run() (+3 more)

### Community 86 - "NewStore"
Cohesion: 0.25
Nodes (11): NewStore(), T, TestBuildSessionContextAndLesson(), TestDeleteLesson(), TestIsStubMarkdown(), TestSeedReplacesStubOnly(), TestTemporaryMode(), defaultTemplate() (+3 more)

### Community 87 - "Init"
Cohesion: 0.25
Nodes (11): MeterProvider, SetCaptureMessageContent(), GetConfig(), Config, Resource, Init(), newMeterProvider(), newResource() (+3 more)

### Community 88 - "prefs.ts"
Cohesion: 0.29
Nodes (12): dynamic, GET(), POST(), PUT(), runtime, getAllPrefs(), getPref(), load() (+4 more)

### Community 89 - ".paths"
Cohesion: 0.23
Nodes (3): SOHubExport, SOHubImport, SessionRef

### Community 90 - "redact_test.go"
Cohesion: 0.26
Nodes (11): ForCapture(), scrubExfilURLs(), String(), StringFull(), T, TestEmptyString(), TestForCaptureSelector(), TestPostgresURLKeepsHostDropsCreds() (+3 more)

### Community 91 - "NewStore"
Cohesion: 0.28
Nodes (11): T, TestIsEmptyListItem(), TestSpansHaveActivity(), TestUpsertSkipsIdentityOnly(), T, TestResolveNestedParentClearsOrphanSubagentFlag(), TestUpsertActiveFromSpansDoesNotPoisonParentWithSubagentType(), TestUpsertActiveFromSpansNestsSubagents() (+3 more)

### Community 92 - "harness-files-page.tsx"
Cohesion: 0.23
Nodes (6): FileEntry, fileNameFromPath(), HarnessFilesPage(), HarnessFilesPageInner(), matchEntry(), useBreadcrumbCrumb()

### Community 93 - "AgentsPanel.tsx"
Cohesion: 0.22
Nodes (11): AgentGraph, AgentNode, agentDetail(), AgentDetailPopover(), AgentDetailState, AgentRow(), AgentsPanel(), AgentsPanelProps (+3 more)

### Community 94 - ".Parse"
Cohesion: 0.26
Nodes (6): PiExport, PiImport, SessionRef, peekPi(), piHome(), piText()

### Community 95 - "Default"
Cohesion: 0.29
Nodes (10): Check, Default(), T, TestAutoApplyTiers(), TestModelForCLI(), TestNormalizeModelSlug(), TestNormalizeObservabilityLocalOnly(), detectAgentCLIs() (+2 more)

### Community 96 - "rec"
Cohesion: 0.20
Nodes (5): Session, T, ToolCall, TestOpenCodeMessageUsagePrefersHostCost(), rec

### Community 97 - "Prune"
Cohesion: 0.29
Nodes (10): Config, Time, Prune(), pruneAudit(), pruneEvalHistory(), pruneRecommendations(), pruneTraceFiles(), T (+2 more)

### Community 98 - "theme-provider.tsx"
Cohesion: 0.26
Nodes (9): metadata, applyDomTheme(), isPreference(), readStoredPreference(), ResolvedTheme, systemResolved(), ThemeContext, ThemeContextValue (+1 more)

### Community 99 - "Classify"
Cohesion: 0.29
Nodes (9): Classification, Inputs, Classify(), EnvAllowlist(), matchAllowlist(), SplitAllowlist(), T, TestClassify() (+1 more)

### Community 100 - "inputBuilder"
Cohesion: 0.58
Nodes (10): T, inputBuilder(), TestCursorAfterAgentResponseNoEstimate(), TestCursorAfterAgentResponseOmitsTokensEvenWhenPresent(), TestCursorSessionIDPrefersConversation(), TestCursorStopAccumulatesIntoSessionEnd(), TestCursorStopStampsRealTokenAttrs(), TestCursorSubagentStartLinkage() (+2 more)

### Community 101 - "DefaultPolicy"
Cohesion: 0.44
Nodes (10): decideGuardrail(), T, TestDecideGuardrailAllowsEmptyTargets(), TestDecideGuardrailAllowsNormalPath(), TestDecideGuardrailDeniesCommand(), TestDecideGuardrailDeniesSensitivePath(), TestDecideGuardrailPathOnlyDoesNotUseZeroValueDeny(), TestIsSessionEndEventParity() (+2 more)

### Community 103 - "Ledger"
Cohesion: 0.36
Nodes (5): Mutex, Time, Ledger, LedgerEntry, ledgerFile

### Community 104 - "npm/package.json"
Cohesion: 0.18
Nodes (10): bin, so, description, engines, node, files, license, name (+2 more)

### Community 105 - "Timeline.tsx"
Cohesion: 0.25
Nodes (10): Action, Mark, Bucket, clock(), MARK_LABEL, MarkGroup, Speed, SPEEDS (+2 more)

### Community 106 - "claudecode/transcript.go"
Cohesion: 0.42
Nodes (9): assistantContentBlock, assistantMessage, assistantUsage, coalescedTurn, transcriptLine, coalesceAssistants(), RawMessage, mergeAssistantGroup() (+1 more)

### Community 107 - "defaultSampler"
Cohesion: 0.20
Nodes (6): spanNameSampler, firstEnv(), defaultSampler(), Sampler, SamplingParameters, SamplingResult

### Community 108 - "session/store.go"
Cohesion: 0.16
Nodes (12): Explain(), Client, Meta, shortSHA(), countCheckpointDirs(), isNestedSession(), previewFromMessages(), rank() (+4 more)

### Community 109 - "treeLayout.ts"
Cohesion: 0.33
Nodes (7): centerOf(), nearbyFiles(), Node, TreeDir, TreeEdge, TreeLayout, CityFile

### Community 110 - "dirLabels.ts"
Cohesion: 0.25
Nodes (4): DirLabel, DirLabelEntry, DirLabelSet, labelTexture()

### Community 111 - "citymap.go"
Cohesion: 0.32
Nodes (7): BuildCitymap(), BuildReplayFromFootprint(), Meta, Citymap, CitymapNode, Replay, ReplayEvent

### Community 112 - "scripts"
Cohesion: 0.25
Nodes (8): scripts, build, dev, lint, start, test, test:watch, typecheck

### Community 113 - "inputBuilder"
Cohesion: 0.76
Nodes (6): T, inputBuilder(), TestClaudePreToolUseTaskLinksSubagent(), TestClaudeSessionDurationCachedAcrossInvocations(), TestClaudeSubagentStopWithoutPreToolUse(), withIsolatedCache()

### Community 114 - "marketplace/plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 115 - "launch.go"
Cohesion: 0.57
Nodes (6): agentBinary(), ensureInstalled(), NewCmd(), run(), vendorOf(), writeMemoryPack()

### Community 116 - "EstimateTokens"
Cohesion: 0.48
Nodes (5): EstimateTokens(), T, TestEstimateTokensEmpty(), TestEstimateTokensShort(), TestEstimateTokensWordFloor()

### Community 117 - "plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 118 - "Config"
Cohesion: 0.33
Nodes (4): Config, IDGenerator, Duration, Sampler

### Community 119 - "marketplace/plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 120 - "Display"
Cohesion: 0.40
Nodes (4): Display(), NewCmd(), T, TestDisplaySemver()

### Community 121 - "plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 122 - "install.sh script"
Cohesion: 0.67
Nodes (5): fatal(), info(), need(), install.sh script, warn()

### Community 123 - "adapter"
Cohesion: 0.40
Nodes (3): adapter, Adapter, New()

### Community 126 - "harness.go"
Cohesion: 0.50
Nodes (3): FindRoot(), migrateDir(), migrateFile()

### Community 127 - "axi_test.go"
Cohesion: 0.60
Nodes (4): T, TestEmptyText(), TestRowsJSON(), TestTruncate()

### Community 128 - "TestVendorsFromArg"
Cohesion: 0.60
Nodes (4): equalSlice(), T, TestRemovePath(), TestVendorsFromArg()

### Community 130 - "install.go"
Cohesion: 0.83
Nodes (3): NewCmd(), run(), vendorsFromArg()

### Community 132 - "web/package.json"
Cohesion: 0.50
Nodes (3): name, private, version

## Knowledge Gaps
- **293 isolated node(s):** `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter`, `Emitter`, `sessionRootMarkerKey` (+288 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **22 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Command` connect `cmds_dev.go` to `launch.go`, `cmds_extra.go`, `Resolve`, `shell/sidebar.tsx`?**
  _High betweenness centrality (0.395) - this node is a cross-community bridge._
- **Why does `Resolve()` connect `Resolve` to `Run`, `TestResolveAndEnsureDirs`, `Paths`, `.Create`, `recommend.go`, `guardrails.go`, `graph.go`, `backend.go`, `MineTranscript`, `Refresh`, `cmds_extra.go`, `cmds_dev.go`, `NewEmitter`, `Run`, `.Port`, `.Write`, `harness_hooks.go`, `Rebuild`, `NewStore`, `.paths`, `Default`, `Prune`, `launch.go`, `harness.go`?**
  _High betweenness centrality (0.198) - this node is a cross-community bridge._
- **Why does `cn()` connect `cn` to `playground-shell.tsx`, `shell/sidebar.tsx`, `memory.ts`, `harness-yaml.ts`, `sessions/[id]/page.tsx`, `session-timeline.tsx`, `utils.ts`, `session-search-bar.tsx`, `file-content-view.tsx`?**
  _High betweenness centrality (0.198) - this node is a cross-community bridge._
- **Are the 78 inferred relationships involving `Resolve()` (e.g. with `.paths()` and `.soRoot()`) actually correct?**
  _`Resolve()` has 78 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter` to the rest of the system?**
  _293 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `release-packages.py` be split into smaller, more focused modules?**
  _Cohesion score 0.06155950752393981 - nodes in this community are weakly interconnected._
- **Should `Store` be split into smaller, more focused modules?**
  _Cohesion score 0.06413730803974707 - nodes in this community are weakly interconnected._