# Graph Report - /var/folders/fr/660k4_4n1gs3g_d3c939lcm40000gn/T/so-graphify-2905586636  (2026-08-07)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 3151 nodes · 7458 edges · 173 communities (152 shown, 21 thin omitted)
- Extraction: 88% EXTRACTED · 12% INFERRED · 0% AMBIGUOUS · INFERRED: 923 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- release-packages.py
- Store
- cli.go
- LLMTurn
- cn
- InstallVendor
- entitlement.go
- codex/handle.go
- memory.ts
- CityScene.tsx
- sessions.ts
- harness-yaml.ts
- trace.ts
- Paths
- repoCwd
- Store
- projectIdFromRequest
- misc.ts
- file-content-view.tsx
- session-timeline.tsx
- Resolve
- types.ts
- agentlinks.go
- fileExists
- playground-shell.tsx
- receiver.go
- title.go
- agentconfig/config.go
- blame.go
- Context
- handle
- Input
- emitGitArtifactsCodex
- recommend.go
- compilerOptions
- shell/sidebar.tsx
- MapView.tsx
- attrs.go
- guardrails.go
- sessions/page.tsx
- graph.go
- Remove
- evals.ts
- Config
- citymap.ts
- sessions/[id]/page.tsx
- sessionTraceContext
- portToHub
- attributes.go
- Out
- inject.go
- backend.go
- Hud.tsx
- codex/transcript.go
- Profile
- judge.ts
- cmds_extra.go
- home
- devDependencies
- run
- finalizeSession
- discover.go
- Load
- RemoveAll
- dependencies
- cmds_dev.go
- .Create
- client.go
- .Port
- vendor_config_scrub_test.go
- harness-files-page.tsx
- utils.ts
- .Write
- PortableSession
- NewEmitter
- handle
- Lookup
- recs/page.tsx
- reducer.ts
- Run
- ResolveForVendor
- ArmResume
- handle
- html/route.ts
- prefs.ts
- .Write
- Refresh
- Emitter
- harness_hooks.go
- stripCursorHooks
- NewStore
- Init
- treeLayout.ts
- .Write
- .paths
- rec
- HarnessID
- NewStore
- StateStore
- theme-provider.tsx
- AgentsPanel.tsx
- Default
- Classify
- inputBuilder
- DefaultPolicy
- Rebuild
- Run
- npm/package.json
- Timeline.tsx
- claudecode/transcript.go
- defaultSampler
- config/route.ts
- .EmitEditDecision
- Prune
- Explain
- dirLabels.ts
- config_test.go
- scripts
- inputBuilder
- marketplace/plugins/opencode/superopen.ts
- launch.go
- EstimateTokens
- plugins/opencode/superopen.ts
- Config
- marketplace/plugins/pi/index.ts
- writeSkillBundle
- Display
- plugins/pi/index.ts
- install.sh script
- adapter
- dev_proc_unix.go
- dev_proc_windows.go
- .ResolveLLM
- Run
- axi_test.go
- So
- install.go
- web/package.json
- process_windows.go
- TestResolveAndEnsureDirs
- tailwind.config.ts
- cmdk
- Client
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
2. `Resolve()` - 77 edges
3. `cn()` - 70 edges
4. `fileExists()` - 59 edges
5. `projectIdFromRequest()` - 50 edges
6. `soPath()` - 41 edges
7. `repoRoot()` - 38 edges
8. `runWithProject()` - 37 edges
9. `main()` - 36 edges
10. `Input` - 36 edges

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

## Communities (173 total, 21 thin omitted)

### Community 0 - "release-packages.py"
Cohesion: 0.06
Nodes (46): Any, apply_version_files(), body_for_component_evidence(), build_pr_evidence(), bump(), chunk_prs(), collect_prs(), collect_single_pr() (+38 more)

### Community 1 - "Store"
Cohesion: 0.06
Nodes (50): Event, SoftWrite, Append(), Time, List(), Path(), splitLines(), Applyable() (+42 more)

### Community 2 - "cli.go"
Cohesion: 0.06
Nodes (51): claudeEnvelope, Result, Runner, activitySignals, Result, codexExecArgs(), codexModel(), Detect() (+43 more)

### Community 3 - "LLMTurn"
Cohesion: 0.06
Nodes (26): recordingEmitter, recordingEmitter, recordingEmitter, Session, ToolCall, Session, ToolCall, Session (+18 more)

### Community 4 - "cn"
Cohesion: 0.06
Nodes (45): EVAL_COLS, EvalRow, EvalsDashboardView(), EvaluationsInner(), fmtCost(), fmtTokens(), LOG_COLS, LogFilter (+37 more)

### Community 5 - "InstallVendor"
Cohesion: 0.07
Nodes (45): cursorHooksFile, hookBinaryAvailable(), Install(), Status(), writeEndpointConfig(), RawMessage, installCursorHooks(), isOurHookEntry() (+37 more)

### Community 6 - "entitlement.go"
Cohesion: 0.06
Nodes (36): Status, Clear(), CloudOTLPEnabled(), configDir(), Time, Load(), LoginPaid(), path() (+28 more)

### Community 7 - "codex/handle.go"
Cohesion: 0.09
Nodes (50): codexPayload, applyPatchBody(), boolField(), buildSession(), buildToolCall(), canonicalStatus(), clearTurnFragment(), commandFromToolInput() (+42 more)

### Community 8 - "memory.ts"
Cohesion: 0.11
Nodes (46): dynamic, GET(), POST(), runtime, Lesson, MemoryPage(), Pack, Section (+38 more)

### Community 9 - "CityScene.tsx"
Cohesion: 0.09
Nodes (41): useThemeOptional(), applyCitySceneTheme(), attentionColumnGeometry(), attentionHeight(), baseColor(), centerFor(), CityScene(), colors (+33 more)

### Community 10 - "sessions.ts"
Cohesion: 0.09
Nodes (45): dynamic, GET(), runtime, CHAT_ATTR_KEYS, clearAgentLink(), clearFalseNesting(), collectAllSessions(), countCheckpointDirs() (+37 more)

### Community 11 - "harness-yaml.ts"
Cohesion: 0.10
Nodes (39): FileContentView(), Domain, FILE, HarnessItemDetailPage(), hydrate(), KIND_BLURB, CONFIG, EvaluationComposer() (+31 more)

### Community 12 - "trace.ts"
Cohesion: 0.11
Nodes (39): dynamic, runtime, agentLabel(), buildAgentGraph(), buildAgentTrace(), eventCountFor(), linkMethodFor(), readMeta() (+31 more)

### Community 13 - "Paths"
Cohesion: 0.14
Nodes (37): Paths, Delta, ledgerEntry, ledgerFile, pendingFile, Result, Trigger, maybeHarvestOnSessionEnd() (+29 more)

### Community 14 - "repoCwd"
Cohesion: 0.12
Nodes (32): dynamic, POST(), runtime, dynamic, POST(), runtime, dynamic, GET() (+24 more)

### Community 15 - "Store"
Cohesion: 0.12
Nodes (18): IndexEntry, LoadMap(), ListItem, Store, IsEmptyListItem(), countCheckpointDirs(), Client, Store (+10 more)

### Community 16 - "projectIdFromRequest"
Cohesion: 0.09
Nodes (32): GET(), DELETE(), dynamic, GET(), POST(), PUT(), runtime, GET() (+24 more)

### Community 17 - "misc.ts"
Cohesion: 0.11
Nodes (36): DELETE(), dynamic, GET(), runtime, dynamic, GET(), runtime, compactPendingRecommendations() (+28 more)

### Community 18 - "file-content-view.tsx"
Cohesion: 0.06
Nodes (34): buildEvalRows(), buildGuardRows(), EVAL_COLUMNS, EVAL_VISIBILITY, EvalColumnKey, EvalRow, EvalRowKind, EvaluationsDocument() (+26 more)

### Community 19 - "session-timeline.tsx"
Cohesion: 0.08
Nodes (33): basename(), buildTimeline(), buildTimelineFromPortableTurns(), ChatMinimap(), classifyTool(), decodeFiltersParam(), DEFAULT_FILTERS, encodeFiltersParam() (+25 more)

### Community 20 - "Resolve"
Cohesion: 0.20
Nodes (35): cmdCheckpoint(), cmdImport(), cmdProjects(), cmdStatus(), cmdHarvest(), attachSessionsStart(), cmdAudit(), cmdLearn() (+27 more)

### Community 21 - "types.ts"
Cohesion: 0.07
Nodes (32): AgentKind, AgentLinkMethod, AgentLinkQuality, AgentStatus, CityDir, Coverage, DimensionName, Observability (+24 more)

### Community 22 - "agentlinks.go"
Cohesion: 0.14
Nodes (33): Entry, fileDoc, pendingDoc, PendingSpawn, AllowRegister(), ClaimPending(), DiscoverCursorParent(), ExtractAgentID() (+25 more)

### Community 23 - "fileExists"
Cohesion: 0.14
Nodes (26): GET(), dynamic, GET(), runtime, AuditEvent, listAuditEvents(), assertEditable(), createHarnessFile() (+18 more)

### Community 24 - "playground-shell.tsx"
Cohesion: 0.11
Nodes (24): BreadcrumbContext, BreadcrumbContextValue, BreadcrumbCrumb, BreadcrumbProvider(), useBreadcrumb(), HeaderContextRow(), Option, pageTitle() (+16 more)

### Community 25 - "receiver.go"
Cohesion: 0.11
Nodes (28): AnyValue, anyValueString(), KeyValue, Span, Store, IsSubagentAttrs(), kvMap(), NewReceiver() (+20 more)

### Community 26 - "title.go"
Cohesion: 0.12
Nodes (30): humanizePromptPreview(), DisplayName(), EnsureTitle(), generateTitle(), Client, Meta, loadCodexTitles(), loadOpenCodeTitles() (+22 more)

### Community 27 - "agentconfig/config.go"
Cohesion: 0.12
Nodes (26): Defaults, Flags, Resolved, builtinDefaults(), configDir(), isLocalHost(), isSecretKey(), Load() (+18 more)

### Community 28 - "blame.go"
Cohesion: 0.09
Nodes (24): LineInfo, WhyResult, File(), Store, isHex(), sessionIDForCommit(), Why(), AppendTrailer() (+16 more)

### Community 29 - "Context"
Cohesion: 0.09
Nodes (17): adapter, adapter, Adapter, Context, NormalizeRepoURL(), remoteURL(), run(), Snapshot() (+9 more)

### Community 30 - "handle"
Cohesion: 0.16
Nodes (29): cursorAttachment, cursorEdit, cursorPayload, agentIDFromPath(), applyAccumulatedTokens(), buildPromptTurn(), buildResponseTurn(), buildSession() (+21 more)

### Community 31 - "Input"
Cohesion: 0.16
Nodes (26): claudePayload, Emitter, bashStdout(), countMultiEditLines(), drainAssistantTurns(), drainRejectedPendingEdits(), emitOneAssistantTurn(), emitSession() (+18 more)

### Community 32 - "emitGitArtifactsCodex"
Cohesion: 0.16
Nodes (27): PatchLineCounts, CountInlineDiff(), CountPatchLines(), ExtractCommitMessage(), ExtractCommitSHA(), ExtractPRTitle(), ExtractPRURLAndNumber(), firstGroup() (+19 more)

### Community 33 - "recommend.go"
Cohesion: 0.20
Nodes (27): advisoryGuardrailBody(), Apply(), containsStr(), Dismiss(), EnqueuePending(), FingerprintKey(), Generate(), Result (+19 more)

### Community 34 - "compilerOptions"
Cohesion: 0.07
Nodes (28): dom, dom.iterable, esnext, .next/dev/types/**/*.ts, next-env.d.ts, .next/types/**/*.ts, node_modules, **/*.ts (+20 more)

### Community 35 - "shell/sidebar.tsx"
Cohesion: 0.12
Nodes (23): FeaturePageHeader(), FeaturePageHeaderProps, iconForPath(), iconForTitle(), flatItems(), isActive(), NavigationLink(), PrimaryItem() (+15 more)

### Community 36 - "MapView.tsx"
Cohesion: 0.15
Nodes (24): apiURL(), describeError(), getAgentTrace(), getJSON(), getSessionAgents(), getSessionReport(), getSessionSnapshot(), listSessions() (+16 more)

### Community 37 - "attrs.go"
Cohesion: 0.19
Nodes (23): scrubFn, bodyAllowed(), buildInputMessagesJSON(), buildOutputMessagesJSON(), Session, Span, ToolCall, inferProvider() (+15 more)

### Community 38 - "guardrails.go"
Cohesion: 0.15
Nodes (20): Decision, Engine, File, Policy, Rule, CheckCommandString(), CheckPathString(), EnsureDefaults() (+12 more)

### Community 39 - "sessions/page.tsx"
Cohesion: 0.16
Nodes (21): dateGroupLabel(), modelLabel(), Session, SessionsPage(), vendorLabel(), joinQuery(), KNOWN_AGENTS, KNOWN_TOOLS (+13 more)

### Community 40 - "graph.go"
Cohesion: 0.17
Nodes (25): Result, stubGraph, Build(), buildStub(), buildWithGraphify(), copyDir(), countGraph(), ensureGraphifyCommunitySidecars() (+17 more)

### Community 41 - "Remove"
Cohesion: 0.20
Nodes (25): Active(), configDir(), Get(), Time, idFor(), List(), load(), Path() (+17 more)

### Community 42 - "evals.ts"
Cohesion: 0.15
Nodes (25): buildDailySeries(), buildEvaluatorStats(), dayKey(), EvalBadge, EvalFailurePoint, EvalRun, evalsConfigPath(), evaluationScope() (+17 more)

### Community 43 - "Config"
Cohesion: 0.13
Nodes (15): Config, CostConfig, EvalsConfig, ExporterConfig, GraphConfig, GuardrailsConfig, InjectConfig, LLMConfig (+7 more)

### Community 44 - "citymap.ts"
Cohesion: 0.14
Nodes (24): dynamic, POST(), runtime, applyTreemap(), buildTree(), cachePath(), capAspect(), CityDir (+16 more)

### Community 45 - "sessions/[id]/page.tsx"
Cohesion: 0.11
Nodes (21): GraphPage(), MapView, NestedSession, projectFromURL(), SessionDetailInner(), Tab, TabButton(), SettingsPage() (+13 more)

### Community 46 - "sessionTraceContext"
Cohesion: 0.19
Nodes (21): sessionIDGenerator, sessionRootMarker, sessionRootMarkerKey, InMemoryExporter, deriveSessionRootSpanID(), deriveSessionTraceID(), randomSpanID(), randomTraceID() (+13 more)

### Community 47 - "portToHub"
Cohesion: 0.15
Nodes (16): ClaudeCode(), Codex(), portToHub(), RegisterAll(), DefaultLedgerPath(), Mutex, Time, NewLedger() (+8 more)

### Community 48 - "attributes.go"
Cohesion: 0.17
Nodes (18): KeyValue, Span, NewAttributeBuilder(), SetBoolAttribute(), SetFloat64Attribute(), SetInt64Attribute(), SetIntAttribute(), SetJSONAttribute() (+10 more)

### Community 49 - "Out"
Cohesion: 0.15
Nodes (13): Error, Flags, Out, Bind(), chainPreRun(), envTruthy(), ExitCode(), Fail() (+5 more)

### Community 50 - "inject.go"
Cohesion: 0.16
Nodes (21): InstallOptions, InstallResult, UninstallResult, Apply(), Brief(), EnsureGlobalSkill(), EnsureSkills(), fileExists() (+13 more)

### Community 51 - "backend.go"
Cohesion: 0.14
Nodes (15): ComputeAttribution(), ListItem, Meta, Store, Time, metaHasCommit(), metaHasPR(), AttributionSummary (+7 more)

### Community 52 - "Hud.tsx"
Cohesion: 0.11
Nodes (12): SessionRail(), SessionRailDrawer(), SessionRailProps, SessionRailTool, ActionCounts, MetricObservability, ACTION_ORDER, ChurnEntry (+4 more)

### Community 53 - "codex/transcript.go"
Cohesion: 0.20
Nodes (22): codexLine, codexReasoningItem, codexReasoningSummaryPart, codexTokenSnapshot, codexTokenUsage, codexTokenUsageInfo, sessionMeta, RawMessage (+14 more)

### Community 54 - "Profile"
Cohesion: 0.18
Nodes (20): Profile, ExtractJSON(), dedupeRules(), enrichArchitecture(), enrichConventions(), Seed(), seedEvals(), seedGuardrails() (+12 more)

### Community 55 - "judge.ts"
Cohesion: 0.16
Nodes (22): availableJudgeClis(), digestTrace(), errorPath(), extractJSON(), getReportStatus(), heuristicReport(), JUDGE_CLIS, judgePrompt() (+14 more)

### Community 56 - "cmds_extra.go"
Cohesion: 0.14
Nodes (19): cmdBlame(), cmdGitHook(), cmdLogin(), cmdLogout(), cmdWhy(), extendSessionsCmd(), firstLine(), Meta (+11 more)

### Community 57 - "home"
Cohesion: 0.14
Nodes (9): ClaudeExport, ClaudeImport, OpenCodeExport, OpenCodeImport, encodeClaudeProjectDir(), home(), exportOpenCodeSQLiteJSON(), SessionRef (+1 more)

### Community 58 - "devDependencies"
Cohesion: 0.10
Nodes (21): autoprefixer, eslint, eslint-config-next, postcss, tailwindcss, @types/node, @types/react-dom, @types/three (+13 more)

### Community 59 - "run"
Cohesion: 0.18
Nodes (19): peekedContext, canonicalVendor(), firstHookEnv(), firstNonEmpty(), foreignParentID(), Adapter, isClaudeCodeVendor(), isRealClaudeCodeInvocation() (+11 more)

### Community 60 - "finalizeSession"
Cohesion: 0.13
Nodes (17): Config, mineOnFinalize(), applyTracesDir(), demoSession(), finalizeSession(), Config, refreshSession(), Span (+9 more)

### Community 61 - "discover.go"
Cohesion: 0.19
Nodes (18): AgentSource, GraphSummary, BuildProfile(), cleanMD(), CollectAgentFiles(), isNegativeBullet(), isNegativeHeading(), isSectionMarker() (+10 more)

### Community 62 - "Load"
Cohesion: 0.24
Nodes (19): markCodexSessionRootEmitted(), stampTurnFragment(), accumulateStopTokens(), AddPendingEdit(), BumpCodeCounters(), DrainPendingEdits(), GC(), Duration (+11 more)

### Community 63 - "RemoveAll"
Cohesion: 0.19
Nodes (17): Writer, NewCmd(), RemoveAll(), run(), equalSlice(), T, TestRemovePath(), TestVendorsFromArg() (+9 more)

### Community 64 - "dependencies"
Cohesion: 0.11
Nodes (19): class-variance-authority, clsx, lucide-react, next, @radix-ui/react-tooltip, react, react-is, recharts (+11 more)

### Community 65 - "cmds_dev.go"
Cohesion: 0.22
Nodes (18): cmdDev(), logFile(), maybeOpenUI(), pidFile(), readDevStatus(), runDev(), runDevForeground(), runDir() (+10 more)

### Community 66 - ".Create"
Cohesion: 0.20
Nodes (9): Meta, Store, containsDotDot(), Time, NewStore(), splitSlash(), stringsHasDotDot(), T (+1 more)

### Community 67 - "client.go"
Cohesion: 0.22
Nodes (12): ResolvedLLM, Config, Duration, Client, New(), NewFromConfig(), NewFromResolved(), truncate() (+4 more)

### Community 68 - ".Port"
Cohesion: 0.14
Nodes (11): Event, RefreshMemoryAfterPort(), Orchestrator, SessionRef, Orchestrator, RemapCWD(), Event, HubFactory (+3 more)

### Community 69 - "vendor_config_scrub_test.go"
Cohesion: 0.25
Nodes (16): FileMode, writeFileAtomic(), stripClaudeMarketplaceJSON(), stripCodexConfigTOML(), stripCodexOwnedSections(), T, mustJSONRead(), mustJSONWrite() (+8 more)

### Community 70 - "harness-files-page.tsx"
Cohesion: 0.18
Nodes (10): FileEntry, fileNameFromPath(), HarnessFilesPage(), HarnessFilesPageInner(), matchEntry(), useBreadcrumbCrumb(), formatSearch(), hrefFor() (+2 more)

### Community 71 - "utils.ts"
Cohesion: 0.19
Nodes (12): actorLabel(), decisionDialogTitle(), decisionLabel(), decisionPlaceholder(), RecDetailPage(), BACKENDS, SafeConfig, THEME_OPTIONS (+4 more)

### Community 72 - ".Write"
Cohesion: 0.18
Nodes (9): CodexExport, CodexImport, codexMessageText(), codexRoot(), SessionRef, peekCodex(), writeJSONL(), ExportResult (+1 more)

### Community 73 - "PortableSession"
Cohesion: 0.27
Nodes (9): SessionRef, peekClaude(), ensureMeta(), firstLine(), parseTimeMs(), textFromContent(), NewPortableSession(), PortableSession (+1 more)

### Community 74 - "NewEmitter"
Cohesion: 0.21
Nodes (14): detectHostFromBinary(), detectHostFromProcessTree(), drainCodeCounters(), drainCounters(), Session, isSessionStart(), NewEmitter(), readProcessNameAndPPID() (+6 more)

### Community 75 - "handle"
Cohesion: 0.19
Nodes (13): accumulate(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), itoa() (+5 more)

### Community 76 - "Lookup"
Cohesion: 0.23
Nodes (14): Lookup(), T, TestCostBasic(), TestCostCacheCreationFallback(), TestCostCacheCreationPremium(), TestCostWithCacheRead(), TestCostZeroRate(), TestLookupAnthropic() (+6 more)

### Community 77 - "recs/page.tsx"
Cohesion: 0.18
Nodes (13): REC_COLS, REC_TABS, RecRowContent(), relativeTime(), SeverityBadge(), severityFor(), shortId(), shortPath() (+5 more)

### Community 78 - "reducer.ts"
Cohesion: 0.21
Nodes (12): FilePlayback, PlaybackEngine, touchRank, CitySceneProps, TreeSceneProps, CityMap, Target, Touch (+4 more)

### Community 79 - "Run"
Cohesion: 0.19
Nodes (13): Options, Report, FindRoot(), migrateDir(), migrateFile(), detectStack(), detectStructure(), findPlugins() (+5 more)

### Community 80 - "ResolveForVendor"
Cohesion: 0.23
Nodes (13): base64URLDecode(), claudeCodeEmail(), codexEmail(), decodeJWTSegment(), emailFromJWT(), FromGitConfig(), ResolveForVendor(), T (+5 more)

### Community 81 - "ArmResume"
Cohesion: 0.21
Nodes (12): T, TestMaybeInjectMemory_PortResumeWithoutMemory(), ArmResume(), consumeLegacyCursorPending(), ConsumePendingResume(), consumeSOPortPending(), Time, portRunDir() (+4 more)

### Community 82 - "handle"
Cohesion: 0.21
Nodes (11): accumulateUsage(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), stampUsage() (+3 more)

### Community 83 - "html/route.ts"
Cohesion: 0.27
Nodes (11): dynamic, GET(), HEAD(), loadGraphHtml(), runtime, themeFromRequest(), GraphHtmlStatus, inspectGraphHtml() (+3 more)

### Community 84 - "prefs.ts"
Cohesion: 0.27
Nodes (13): dynamic, GET(), POST(), PUT(), runtime, readJSONFile(), getAllPrefs(), getPref() (+5 more)

### Community 85 - ".Write"
Cohesion: 0.23
Nodes (4): CursorExport, CursorImport, SessionRef, writeTranscript()

### Community 86 - "Refresh"
Cohesion: 0.27
Nodes (12): maxSharedMtime(), startRefreshWatcher(), writeRefreshStatus(), gitSHA(), Time, loadRefreshMarker(), Refresh(), refreshMarkerPath() (+4 more)

### Community 87 - "Emitter"
Cohesion: 0.21
Nodes (6): Emitter, perEventSpansAllowed(), Mutex, Time, ToolCall, Tracer

### Community 88 - "harness_hooks.go"
Cohesion: 0.25
Nodes (13): Decision, extractToolTargets(), findRepoRoot(), isSessionEndEvent(), isSessionStartEvent(), isToolGateEvent(), isTurnBoundaryHarvestEvent(), maybeAuditApproval() (+5 more)

### Community 89 - "stripCursorHooks"
Cohesion: 0.29
Nodes (12): RawMessage, isOurHookEntry(), stripCursorHooks(), T, TestStripCursorHooks_DeletesFileWhenOnlyOursAndVersion(), TestStripCursorHooks_KeepsFileWhenOtherTopLevelKeysExist(), TestStripCursorHooks_MissingFileIsNoOp(), TestStripCursorHooks_NoOpWhenNothingOurs() (+4 more)

### Community 90 - "NewStore"
Cohesion: 0.25
Nodes (11): NewStore(), T, TestBuildSessionContextAndLesson(), TestDeleteLesson(), TestIsStubMarkdown(), TestSeedReplacesStubOnly(), TestTemporaryMode(), defaultTemplate() (+3 more)

### Community 91 - "Init"
Cohesion: 0.25
Nodes (11): MeterProvider, SetCaptureMessageContent(), GetConfig(), Config, Resource, Init(), newMeterProvider(), newResource() (+3 more)

### Community 92 - "treeLayout.ts"
Cohesion: 0.22
Nodes (11): centerOf(), nearbyFiles(), Node, TreeDir, TreeEdge, TreeLayout, CityFile, touchWord() (+3 more)

### Community 93 - ".Write"
Cohesion: 0.23
Nodes (6): PiExport, PiImport, SessionRef, peekPi(), piHome(), piText()

### Community 94 - ".paths"
Cohesion: 0.23
Nodes (3): SOHubExport, SOHubImport, SessionRef

### Community 95 - "rec"
Cohesion: 0.17
Nodes (5): Session, T, ToolCall, TestPiHostCostPreferred(), rec

### Community 96 - "HarnessID"
Cohesion: 0.29
Nodes (6): NewRegistry(), ExportAdapter, HarnessID, ImportAdapter, Registry, SessionRef

### Community 97 - "NewStore"
Cohesion: 0.28
Nodes (11): T, TestIsEmptyListItem(), TestSpansHaveActivity(), TestUpsertSkipsIdentityOnly(), T, TestResolveNestedParentClearsOrphanSubagentFlag(), TestUpsertActiveFromSpansDoesNotPoisonParentWithSubagentType(), TestUpsertActiveFromSpansNestsSubagents() (+3 more)

### Community 98 - "StateStore"
Cohesion: 0.32
Nodes (4): Time, Phase, State, StateStore

### Community 99 - "theme-provider.tsx"
Cohesion: 0.23
Nodes (10): metadata, applyDomTheme(), isPreference(), readStoredPreference(), ResolvedTheme, systemResolved(), ThemeContext, ThemeContextValue (+2 more)

### Community 100 - "AgentsPanel.tsx"
Cohesion: 0.22
Nodes (11): AgentGraph, AgentNode, agentDetail(), AgentDetailPopover(), AgentDetailState, AgentRow(), AgentsPanel(), AgentsPanelProps (+3 more)

### Community 101 - "Default"
Cohesion: 0.24
Nodes (9): Default(), NormalizeModelSlug(), T, TestAutoApplyTiers(), TestModelForCLI(), TestNormalizeModelSlug(), TestNormalizeObservabilityLocalOnly(), T (+1 more)

### Community 102 - "Classify"
Cohesion: 0.29
Nodes (9): Classification, Inputs, Classify(), EnvAllowlist(), matchAllowlist(), SplitAllowlist(), T, TestClassify() (+1 more)

### Community 103 - "inputBuilder"
Cohesion: 0.58
Nodes (10): T, inputBuilder(), TestCursorAfterAgentResponseNoEstimate(), TestCursorAfterAgentResponseOmitsTokensEvenWhenPresent(), TestCursorSessionIDPrefersConversation(), TestCursorStopAccumulatesIntoSessionEnd(), TestCursorStopStampsRealTokenAttrs(), TestCursorSubagentStartLinkage() (+2 more)

### Community 104 - "DefaultPolicy"
Cohesion: 0.44
Nodes (10): decideGuardrail(), T, TestDecideGuardrailAllowsEmptyTargets(), TestDecideGuardrailAllowsNormalPath(), TestDecideGuardrailDeniesCommand(), TestDecideGuardrailDeniesSensitivePath(), TestDecideGuardrailPathOnlyDoesNotUseZeroValueDeny(), TestIsSessionEndEventParity() (+2 more)

### Community 105 - "Rebuild"
Cohesion: 0.29
Nodes (9): QueryRetrieve(), indexPath(), Rebuild(), Search(), snippetAround(), T, TestRebuildAndSearch(), Hit (+1 more)

### Community 106 - "Run"
Cohesion: 0.24
Nodes (9): Run(), BuildCitymap(), BuildReplayFromFootprint(), Meta, Options, Citymap, CitymapNode, Replay (+1 more)

### Community 107 - "npm/package.json"
Cohesion: 0.18
Nodes (10): bin, so, description, engines, node, files, license, name (+2 more)

### Community 108 - "Timeline.tsx"
Cohesion: 0.25
Nodes (10): Action, Mark, Bucket, clock(), MARK_LABEL, MarkGroup, Speed, SPEEDS (+2 more)

### Community 109 - "claudecode/transcript.go"
Cohesion: 0.42
Nodes (9): assistantContentBlock, assistantMessage, assistantUsage, coalescedTurn, transcriptLine, coalesceAssistants(), RawMessage, mergeAssistantGroup() (+1 more)

### Community 110 - "defaultSampler"
Cohesion: 0.20
Nodes (6): spanNameSampler, firstEnv(), defaultSampler(), Sampler, SamplingParameters, SamplingResult

### Community 111 - "config/route.ts"
Cohesion: 0.31
Nodes (9): dynamic, formatYamlValue(), GET(), isTopLevelKey(), PatchBody, PUT(), runtime, setYamlPath() (+1 more)

### Community 112 - ".EmitEditDecision"
Cohesion: 0.47
Nodes (7): commonAttrs(), KeyValue, initMetrics(), recordCommit(), recordEditDecision(), recordLines(), recordPullRequest()

### Community 113 - "Prune"
Cohesion: 0.44
Nodes (8): Config, Time, Prune(), pruneAudit(), pruneEvalHistory(), pruneRecommendations(), pruneTraceFiles(), Report

### Community 114 - "Explain"
Cohesion: 0.25
Nodes (7): Explain(), Client, Meta, shortSHA(), truncate(), Footprint, FootprintFile

### Community 115 - "dirLabels.ts"
Cohesion: 0.25
Nodes (4): DirLabel, DirLabelEntry, DirLabelSet, labelTexture()

### Community 116 - "config_test.go"
Cohesion: 0.39
Nodes (7): LLMSetupGuide(), T, TestGuardrailsEnabledEnv(), TestLLMSetupGuide(), TestLoadMigratesLegacyGovernanceKey(), TestResolveLLMLocal(), TestResolveLLMOpenRouter()

### Community 117 - "scripts"
Cohesion: 0.25
Nodes (8): scripts, build, dev, lint, start, test, test:watch, typecheck

### Community 118 - "inputBuilder"
Cohesion: 0.76
Nodes (6): T, inputBuilder(), TestClaudePreToolUseTaskLinksSubagent(), TestClaudeSessionDurationCachedAcrossInvocations(), TestClaudeSubagentStopWithoutPreToolUse(), withIsolatedCache()

### Community 119 - "marketplace/plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 120 - "launch.go"
Cohesion: 0.57
Nodes (6): agentBinary(), ensureInstalled(), NewCmd(), run(), vendorOf(), writeMemoryPack()

### Community 121 - "EstimateTokens"
Cohesion: 0.48
Nodes (5): EstimateTokens(), T, TestEstimateTokensEmpty(), TestEstimateTokensShort(), TestEstimateTokensWordFloor()

### Community 122 - "plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 123 - "Config"
Cohesion: 0.33
Nodes (4): Config, IDGenerator, Duration, Sampler

### Community 124 - "marketplace/plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 125 - "writeSkillBundle"
Cohesion: 0.60
Nodes (5): writeSkillBundle(), T, TestRemoveSkillBundle_RemovesNewVendors(), TestWriteSkillBundle_GlobalVendors(), TestWriteSkillBundle_ProjectVendors()

### Community 126 - "Display"
Cohesion: 0.40
Nodes (4): Display(), NewCmd(), T, TestDisplaySemver()

### Community 127 - "plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 128 - "install.sh script"
Cohesion: 0.67
Nodes (5): fatal(), info(), need(), install.sh script, warn()

### Community 129 - "adapter"
Cohesion: 0.40
Nodes (3): adapter, Adapter, New()

### Community 133 - "Run"
Cohesion: 0.70
Nodes (4): Check, detectAgentCLIs(), Format(), Run()

### Community 134 - "axi_test.go"
Cohesion: 0.60
Nodes (4): T, TestEmptyText(), TestRowsJSON(), TestTruncate()

### Community 136 - "install.go"
Cohesion: 0.83
Nodes (3): NewCmd(), run(), vendorsFromArg()

### Community 138 - "web/package.json"
Cohesion: 0.50
Nodes (3): name, private, version

## Knowledge Gaps
- **289 isolated node(s):** `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter`, `Emitter`, `sessionRootMarkerKey` (+284 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **21 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Command` connect `cmds_dev.go` to `cmds_extra.go`, `shell/sidebar.tsx`, `Resolve`, `launch.go`?**
  _High betweenness centrality (0.395) - this node is a cross-community bridge._
- **Why does `cn()` connect `cn` to `shell/sidebar.tsx`, `utils.ts`, `memory.ts`, `sessions/page.tsx`, `harness-yaml.ts`, `sessions/[id]/page.tsx`, `recs/page.tsx`, `file-content-view.tsx`, `session-timeline.tsx`, `playground-shell.tsx`?**
  _High betweenness centrality (0.201) - this node is a cross-community bridge._
- **Why does `Resolve()` connect `Resolve` to `cli.go`, `Run`, `TestResolveAndEnsureDirs`, `Paths`, `recommend.go`, `guardrails.go`, `graph.go`, `backend.go`, `Profile`, `cmds_extra.go`, `finalizeSession`, `discover.go`, `cmds_dev.go`, `.Create`, `.Port`, `NewEmitter`, `Run`, `.Write`, `Refresh`, `harness_hooks.go`, `NewStore`, `.paths`, `Default`, `Rebuild`, `Run`, `launch.go`?**
  _High betweenness centrality (0.187) - this node is a cross-community bridge._
- **Are the 75 inferred relationships involving `Resolve()` (e.g. with `.paths()` and `.soRoot()`) actually correct?**
  _`Resolve()` has 75 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter` to the rest of the system?**
  _289 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `release-packages.py` be split into smaller, more focused modules?**
  _Cohesion score 0.06155950752393981 - nodes in this community are weakly interconnected._
- **Should `Store` be split into smaller, more focused modules?**
  _Cohesion score 0.06413730803974707 - nodes in this community are weakly interconnected._