# Graph Report - /var/folders/fr/660k4_4n1gs3g_d3c939lcm40000gn/T/so-graphify-3792547858  (2026-08-07)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 3227 nodes · 7662 edges · 177 communities (155 shown, 22 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 960 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- release-packages.py
- Store
- cn
- cli.go
- LLMTurn
- Remove
- trace.ts
- sessions.ts
- entitlement.go
- memory.ts
- CityScene.tsx
- harness-yaml.ts
- soPath
- codex/handle.go
- misc.ts
- handle
- Paths
- Store
- types.ts
- session-timeline.tsx
- Resolve
- projectIdFromRequest
- playground-shell.tsx
- agentlinks.go
- receiver.go
- title.go
- file-content-view.tsx
- home
- Emitter
- PortableSession
- agentconfig/config.go
- Context
- emitGitArtifactsCodex
- MapView.tsx
- .Create
- Input
- recommend.go
- compilerOptions
- judge.ts
- sessions/page.tsx
- attrs.go
- Profile
- guardrails.go
- graph.go
- citymap.ts
- sessions/[id]/page.tsx
- Config
- backend.go
- evals.ts
- sessionTraceContext
- attributes.go
- Out
- Hud.tsx
- codex/transcript.go
- shell/sidebar.tsx
- cursorHooksFile
- ArmResume
- utils.ts
- devDependencies
- cmdGitHook
- run
- HarnessID
- reducer.ts
- discover.go
- handle
- RemoveAll
- wsCollector
- dependencies
- cmds_dev.go
- finalizeSession
- Load
- [...path]/route.ts
- client.go
- .Verify
- vendor_config_scrub_test.go
- nodeio.ts
- cmds_extra.go
- InstallVendor
- Lookup
- Run
- ResolveForVendor
- repoRoot
- handle
- userpaths.go
- CursorImport
- .Write
- Refresh
- harness_hooks.go
- codex/handle_test.go
- stripCursorHooks
- NewStore
- Init
- guardrails-dashboard.ts
- .paths
- NewStore
- StateStore
- AgentsPanel.tsx
- Default
- Status
- harness-files-page.tsx
- theme-provider.tsx
- Classify
- DefaultPolicy
- rec
- Rebuild
- portToHub
- Run
- npm/package.json
- Timeline.tsx
- claudecode/transcript.go
- defaultSampler
- harnessvalid.go
- dropdown.tsx
- .EmitEditDecision
- Prune
- Explain
- treeLayout.ts
- dirLabels.ts
- blame.go
- config_test.go
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
- .ResolveLLM
- Run
- axi_test.go
- So
- install.go
- web/package.json
- process_windows.go
- TestResolveAndEnsureDirs
- tailwind.config.ts
- Client
- evaluations/page.tsx
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
- .Write
- cmdk

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

## Communities (177 total, 22 thin omitted)

### Community 0 - "release-packages.py"
Cohesion: 0.06
Nodes (46): Any, apply_version_files(), body_for_component_evidence(), build_pr_evidence(), bump(), chunk_prs(), collect_prs(), collect_single_pr() (+38 more)

### Community 1 - "Store"
Cohesion: 0.08
Nodes (41): Event, Append(), Time, List(), Path(), splitLines(), contains(), T (+33 more)

### Community 2 - "cn"
Cohesion: 0.08
Nodes (38): EvaluationsInner(), GUARD_DASH_COLS, GuardrailsInner(), GuardRow, LOG_COLS, LogFilter, LogRow, PageTab (+30 more)

### Community 3 - "cli.go"
Cohesion: 0.06
Nodes (51): claudeEnvelope, Result, Runner, activitySignals, Result, codexExecArgs(), codexModel(), Detect() (+43 more)

### Community 4 - "LLMTurn"
Cohesion: 0.06
Nodes (24): recordingEmitter, recordingEmitter, recordingEmitter, Session, ToolCall, Session, ToolCall, Session (+16 more)

### Community 5 - "Remove"
Cohesion: 0.08
Nodes (51): InstallOptions, InstallResult, UninstallResult, Apply(), Brief(), EnsureGlobalSkill(), EnsureSkills(), fileExists() (+43 more)

### Community 6 - "trace.ts"
Cohesion: 0.08
Nodes (50): dynamic, GET(), runtime, dynamic, GET(), runtime, dynamic, GET() (+42 more)

### Community 7 - "sessions.ts"
Cohesion: 0.09
Nodes (52): dynamic, GET(), runtime, dynamic, GET(), runtime, fileExists(), applySessionQuery() (+44 more)

### Community 8 - "entitlement.go"
Cohesion: 0.06
Nodes (35): Status, Clear(), CloudOTLPEnabled(), configDir(), Time, Load(), LoginPaid(), path() (+27 more)

### Community 9 - "memory.ts"
Cohesion: 0.10
Nodes (47): dynamic, GET(), POST(), refreshActive(), runtime, Lesson, MemoryPage(), Pack (+39 more)

### Community 10 - "CityScene.tsx"
Cohesion: 0.09
Nodes (41): useThemeOptional(), applyCitySceneTheme(), attentionColumnGeometry(), attentionHeight(), baseColor(), centerFor(), CityScene(), colors (+33 more)

### Community 11 - "harness-yaml.ts"
Cohesion: 0.10
Nodes (40): FileContentView(), Domain, FILE, HarnessItemDetailPage(), hydrate(), KIND_BLURB, CONFIG, EvaluationComposer() (+32 more)

### Community 12 - "soPath"
Cohesion: 0.10
Nodes (35): dynamic, formatYamlValue(), GET(), isTopLevelKey(), PatchBody, PUT(), runtime, setYamlPath() (+27 more)

### Community 13 - "codex/handle.go"
Cohesion: 0.12
Nodes (41): codexPayload, applyPatchBody(), boolField(), buildSession(), buildToolCall(), canonicalStatus(), clearTurnFragment(), commandFromToolInput() (+33 more)

### Community 14 - "misc.ts"
Cohesion: 0.10
Nodes (37): DELETE(), dynamic, GET(), runtime, dynamic, GET(), runtime, dynamic (+29 more)

### Community 15 - "handle"
Cohesion: 0.13
Nodes (39): cursorAttachment, cursorEdit, cursorPayload, agentIDFromPath(), applyAccumulatedTokens(), buildPromptTurn(), buildResponseTurn(), buildSession() (+31 more)

### Community 16 - "Paths"
Cohesion: 0.14
Nodes (37): Paths, Delta, ledgerEntry, ledgerFile, pendingFile, Result, Trigger, maybeHarvestOnSessionEnd() (+29 more)

### Community 17 - "Store"
Cohesion: 0.12
Nodes (17): IndexEntry, LoadMap(), ListItem, Store, IsEmptyListItem(), countCheckpointDirs(), Client, Store (+9 more)

### Community 18 - "types.ts"
Cohesion: 0.06
Nodes (35): AgentKind, AgentLinkMethod, AgentLinkQuality, AgentStatus, CityDir, Coverage, DimensionName, Observability (+27 more)

### Community 19 - "session-timeline.tsx"
Cohesion: 0.08
Nodes (33): basename(), buildTimeline(), buildTimelineFromPortableTurns(), ChatMinimap(), classifyTool(), decodeFiltersParam(), DEFAULT_FILTERS, encodeFiltersParam() (+25 more)

### Community 20 - "Resolve"
Cohesion: 0.20
Nodes (35): cmdCheckpoint(), cmdImport(), cmdProjects(), cmdStatus(), cmdHarvest(), attachSessionsStart(), cmdAudit(), cmdLearn() (+27 more)

### Community 21 - "projectIdFromRequest"
Cohesion: 0.14
Nodes (29): dynamic, POST(), runtime, dynamic, GET(), POST(), runtime, DELETE() (+21 more)

### Community 22 - "playground-shell.tsx"
Cohesion: 0.12
Nodes (23): BreadcrumbContext, BreadcrumbContextValue, BreadcrumbCrumb, BreadcrumbProvider(), useBreadcrumb(), HeaderContextRow(), Option, pageTitle() (+15 more)

### Community 23 - "agentlinks.go"
Cohesion: 0.15
Nodes (33): Entry, fileDoc, pendingDoc, PendingSpawn, AllowRegister(), ClaimPending(), DiscoverCursorParent(), ExtractAgentID() (+25 more)

### Community 24 - "receiver.go"
Cohesion: 0.11
Nodes (28): AnyValue, anyValueString(), KeyValue, Span, Store, IsSubagentAttrs(), kvMap(), NewReceiver() (+20 more)

### Community 25 - "title.go"
Cohesion: 0.12
Nodes (31): humanizePromptPreview(), previewFromMessages(), DisplayName(), EnsureTitle(), generateTitle(), Client, Meta, loadCodexTitles() (+23 more)

### Community 26 - "file-content-view.tsx"
Cohesion: 0.08
Nodes (29): GuardrailsDashboardView(), buildEvalRows(), buildGuardRows(), EVAL_COLUMNS, EVAL_VISIBILITY, EvalColumnKey, EvalRow, EvalRowKind (+21 more)

### Community 27 - "home"
Cohesion: 0.09
Nodes (15): ClaudeExport, ClaudeImport, CursorExport, OpenCodeExport, OpenCodeImport, encodeClaudeProjectDir(), SessionRef, writeTranscript() (+7 more)

### Community 28 - "Emitter"
Cohesion: 0.11
Nodes (20): Emitter, perEventSpansAllowed(), detectHostFromBinary(), detectHostFromProcessTree(), drainCodeCounters(), drainCounters(), Mutex, Session (+12 more)

### Community 29 - "PortableSession"
Cohesion: 0.21
Nodes (16): Meta, portableSessionFromHub(), peekClaude(), codexCallArgs(), loadWorkingStateSidecar(), ensureMeta(), firstLine(), parseTimeMs() (+8 more)

### Community 30 - "agentconfig/config.go"
Cohesion: 0.12
Nodes (26): Defaults, Flags, Resolved, builtinDefaults(), configDir(), isLocalHost(), isSecretKey(), Load() (+18 more)

### Community 31 - "Context"
Cohesion: 0.09
Nodes (17): adapter, adapter, Adapter, Context, NormalizeRepoURL(), remoteURL(), run(), Snapshot() (+9 more)

### Community 32 - "emitGitArtifactsCodex"
Cohesion: 0.15
Nodes (28): PatchLineCounts, CountInlineDiff(), CountPatchLines(), ExtractCommitMessage(), ExtractCommitSHA(), ExtractPRTitle(), ExtractPRURLAndNumber(), firstGroup() (+20 more)

### Community 33 - "MapView.tsx"
Cohesion: 0.14
Nodes (25): apiURL(), describeError(), getAgentTrace(), getJSON(), getSessionAgents(), getSessionReport(), getSessionSnapshot(), listSessions() (+17 more)

### Community 34 - ".Create"
Cohesion: 0.13
Nodes (19): Meta, Store, containsDotDot(), Time, NewStore(), splitSlash(), stringsHasDotDot(), T (+11 more)

### Community 35 - "Input"
Cohesion: 0.16
Nodes (26): claudePayload, Emitter, bashStdout(), countMultiEditLines(), drainAssistantTurns(), drainRejectedPendingEdits(), emitOneAssistantTurn(), emitSession() (+18 more)

### Community 36 - "recommend.go"
Cohesion: 0.20
Nodes (27): advisoryGuardrailBody(), Apply(), containsStr(), Dismiss(), EnqueuePending(), FingerprintKey(), Generate(), Result (+19 more)

### Community 37 - "compilerOptions"
Cohesion: 0.07
Nodes (28): dom, dom.iterable, esnext, .next/dev/types/**/*.ts, next-env.d.ts, .next/types/**/*.ts, node_modules, **/*.ts (+20 more)

### Community 38 - "judge.ts"
Cohesion: 0.12
Nodes (26): dynamic, POST(), runtime, dynamic, GET(), runtime, availableJudgeClis(), digestTrace() (+18 more)

### Community 39 - "sessions/page.tsx"
Cohesion: 0.15
Nodes (21): dateGroupLabel(), modelLabel(), Session, SessionsPage(), vendorLabel(), joinQuery(), KNOWN_AGENTS, KNOWN_TOOLS (+13 more)

### Community 40 - "attrs.go"
Cohesion: 0.19
Nodes (23): scrubFn, bodyAllowed(), buildInputMessagesJSON(), buildOutputMessagesJSON(), Session, Span, ToolCall, inferProvider() (+15 more)

### Community 41 - "Profile"
Cohesion: 0.15
Nodes (24): Profile, ExtractJSON(), dedupeRules(), enrichArchitecture(), enrichConventions(), T, TestSeedUpgradesOnlyGeneratedDefaultGitignore(), TestSeedUpgradesPreviousDefaultToIgnoreSessions() (+16 more)

### Community 42 - "guardrails.go"
Cohesion: 0.15
Nodes (20): Decision, Engine, File, Policy, Rule, CheckCommandString(), CheckPathString(), EnsureDefaults() (+12 more)

### Community 43 - "graph.go"
Cohesion: 0.17
Nodes (25): Result, stubGraph, Build(), buildStub(), buildWithGraphify(), copyDir(), countGraph(), ensureGraphifyCommunitySidecars() (+17 more)

### Community 44 - "citymap.ts"
Cohesion: 0.13
Nodes (25): dynamic, GET(), runtime, applyTreemap(), buildTree(), cachePath(), capAspect(), CityDir (+17 more)

### Community 45 - "sessions/[id]/page.tsx"
Cohesion: 0.11
Nodes (22): GraphPage(), MapView, NestedSession, projectFromURL(), SessionDetailInner(), Tab, TabButton(), SettingsPage() (+14 more)

### Community 46 - "Config"
Cohesion: 0.13
Nodes (15): Config, CostConfig, EvalsConfig, ExporterConfig, GraphConfig, GuardrailsConfig, InjectConfig, LLMConfig (+7 more)

### Community 47 - "backend.go"
Cohesion: 0.13
Nodes (16): ListItem, ComputeAttribution(), ListItem, Meta, Store, Time, metaHasCommit(), metaHasPR() (+8 more)

### Community 48 - "evals.ts"
Cohesion: 0.14
Nodes (24): buildDailySeries(), buildEvaluatorStats(), dayKey(), EvalBadge, EvalFailurePoint, EvalRun, evalsConfigPath(), evaluationScope() (+16 more)

### Community 49 - "sessionTraceContext"
Cohesion: 0.19
Nodes (21): sessionIDGenerator, sessionRootMarker, sessionRootMarkerKey, InMemoryExporter, deriveSessionRootSpanID(), deriveSessionTraceID(), randomSpanID(), randomTraceID() (+13 more)

### Community 50 - "attributes.go"
Cohesion: 0.17
Nodes (18): KeyValue, Span, NewAttributeBuilder(), SetBoolAttribute(), SetFloat64Attribute(), SetInt64Attribute(), SetIntAttribute(), SetJSONAttribute() (+10 more)

### Community 51 - "Out"
Cohesion: 0.15
Nodes (13): Error, Flags, Out, Bind(), chainPreRun(), envTruthy(), ExitCode(), Fail() (+5 more)

### Community 52 - "Hud.tsx"
Cohesion: 0.11
Nodes (12): SessionRail(), SessionRailDrawer(), SessionRailProps, SessionRailTool, ActionCounts, MetricObservability, ACTION_ORDER, ChurnEntry (+4 more)

### Community 53 - "codex/transcript.go"
Cohesion: 0.20
Nodes (22): codexLine, codexReasoningItem, codexReasoningSummaryPart, codexTokenSnapshot, codexTokenUsage, codexTokenUsageInfo, sessionMeta, RawMessage (+14 more)

### Community 54 - "shell/sidebar.tsx"
Cohesion: 0.12
Nodes (23): FeaturePageHeader(), FeaturePageHeaderProps, iconForPath(), iconForTitle(), flatItems(), isActive(), NavigationLink(), PrimaryItem() (+15 more)

### Community 55 - "cursorHooksFile"
Cohesion: 0.21
Nodes (18): cursorHooksFile, RawMessage, installCursorHooks(), isOurHookEntry(), mergeCursorHooks(), readCursorHooksFile(), commandsFor(), RawMessage (+10 more)

### Community 56 - "ArmResume"
Cohesion: 0.19
Nodes (18): T, TestMaybeInjectMemory_PortResumeWithoutMemory(), ArmResume(), consumeLegacyCursorPending(), ConsumePendingResume(), consumeSOPortPending(), portRunDir(), T (+10 more)

### Community 57 - "utils.ts"
Cohesion: 0.16
Nodes (14): actorLabel(), decisionDialogTitle(), decisionLabel(), decisionPlaceholder(), RecDetailPage(), BACKENDS, SafeConfig, THEME_OPTIONS (+6 more)

### Community 58 - "devDependencies"
Cohesion: 0.10
Nodes (21): autoprefixer, eslint, eslint-config-next, postcss, tailwindcss, @types/node, @types/react-dom, @types/three (+13 more)

### Community 59 - "cmdGitHook"
Cohesion: 0.11
Nodes (17): cmdGitHook(), firstLine(), finalizeLatestSession(), AppendTrailer(), ParseTrailers(), T, TestAppendAndParseTrailers(), BackfillFromGitLog() (+9 more)

### Community 60 - "run"
Cohesion: 0.18
Nodes (19): peekedContext, canonicalVendor(), firstHookEnv(), firstNonEmpty(), foreignParentID(), Adapter, isClaudeCodeVendor(), isRealClaudeCodeInvocation() (+11 more)

### Community 61 - "HarnessID"
Cohesion: 0.14
Nodes (13): NewRegistry(), RefreshMemoryAfterPort(), Orchestrator, SessionRef, Time, Event, ExportAdapter, HarnessID (+5 more)

### Community 62 - "reducer.ts"
Cohesion: 0.18
Nodes (15): FilePlayback, PlaybackEngine, touchRank, CitySceneProps, TreeSceneProps, CityMap, Target, Touch (+7 more)

### Community 63 - "discover.go"
Cohesion: 0.19
Nodes (18): AgentSource, GraphSummary, BuildProfile(), cleanMD(), CollectAgentFiles(), isNegativeBullet(), isNegativeHeading(), isSectionMarker() (+10 more)

### Community 64 - "handle"
Cohesion: 0.15
Nodes (15): accumulate(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), itoa() (+7 more)

### Community 65 - "RemoveAll"
Cohesion: 0.19
Nodes (17): Writer, NewCmd(), RemoveAll(), run(), equalSlice(), T, TestRemovePath(), TestVendorsFromArg() (+9 more)

### Community 66 - "wsCollector"
Cohesion: 0.17
Nodes (9): wsCollector, capCommands(), capStrings(), matchesAny(), normalizeToolName(), PortableTurn, RanCommand, SessionRef (+1 more)

### Community 67 - "dependencies"
Cohesion: 0.11
Nodes (19): class-variance-authority, clsx, lucide-react, next, @radix-ui/react-tooltip, react, react-is, recharts (+11 more)

### Community 68 - "cmds_dev.go"
Cohesion: 0.22
Nodes (18): cmdDev(), logFile(), maybeOpenUI(), pidFile(), readDevStatus(), runDev(), runDevForeground(), runDir() (+10 more)

### Community 69 - "finalizeSession"
Cohesion: 0.14
Nodes (17): Config, mineOnFinalize(), applyTracesDir(), demoSession(), finalizeSession(), Config, refreshSession(), Span (+9 more)

### Community 70 - "Load"
Cohesion: 0.25
Nodes (18): markCodexSessionRootEmitted(), accumulateStopTokens(), AddPendingEdit(), BumpCodeCounters(), DrainPendingEdits(), GC(), Duration, Time (+10 more)

### Community 71 - "[...path]/route.ts"
Cohesion: 0.22
Nodes (17): dynamic, GET(), POST(), runtime, assertEditable(), createHarnessFile(), defaultContent(), deleteHarnessFile() (+9 more)

### Community 72 - "client.go"
Cohesion: 0.22
Nodes (12): ResolvedLLM, Config, Duration, Client, New(), NewFromConfig(), NewFromResolved(), truncate() (+4 more)

### Community 73 - ".Verify"
Cohesion: 0.29
Nodes (5): Event, Orchestrator, RemapCWD(), HubFactory, VerifyResult

### Community 74 - "vendor_config_scrub_test.go"
Cohesion: 0.25
Nodes (16): FileMode, writeFileAtomic(), stripClaudeMarketplaceJSON(), stripCodexConfigTOML(), stripCodexOwnedSections(), T, mustJSONRead(), mustJSONWrite() (+8 more)

### Community 75 - "nodeio.ts"
Cohesion: 0.22
Nodes (14): dynamic, GET(), POST(), PUT(), runtime, readJSONFile(), writeText(), getAllPrefs() (+6 more)

### Community 76 - "cmds_extra.go"
Cohesion: 0.17
Nodes (14): cmdBlame(), cmdLogin(), cmdLogout(), cmdWhy(), extendSessionsCmd(), harnessIDFromVendor(), materializeCursorResumePack(), parseFileLine() (+6 more)

### Community 77 - "InstallVendor"
Cohesion: 0.19
Nodes (13): installGenericVendor(), enableClaudeCodePlugin(), enableCodexPlugin(), extractEmbeddedDir(), materializeClaudeMarketplace(), patchManifestBytes(), resolveCodexBin(), resolveSoBin() (+5 more)

### Community 78 - "Lookup"
Cohesion: 0.23
Nodes (14): Lookup(), T, TestCostBasic(), TestCostCacheCreationFallback(), TestCostCacheCreationPremium(), TestCostWithCacheRead(), TestCostZeroRate(), TestLookupAnthropic() (+6 more)

### Community 79 - "Run"
Cohesion: 0.19
Nodes (13): Options, Report, FindRoot(), migrateDir(), migrateFile(), detectStack(), detectStructure(), findPlugins() (+5 more)

### Community 80 - "ResolveForVendor"
Cohesion: 0.23
Nodes (13): base64URLDecode(), claudeCodeEmail(), codexEmail(), decodeJWTSegment(), emailFromJWT(), FromGitConfig(), ResolveForVendor(), T (+5 more)

### Community 81 - "repoRoot"
Cohesion: 0.26
Nodes (12): dynamic, GET(), runtime, currentRepoMeta(), gitRemoteURL(), repoSlug(), slugFromRemoteURL(), enrich() (+4 more)

### Community 82 - "handle"
Cohesion: 0.21
Nodes (11): accumulateUsage(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), stampUsage() (+3 more)

### Community 83 - "userpaths.go"
Cohesion: 0.19
Nodes (13): shellQuote(), hooksDir(), Install(), writeHook(), CodexMarketplaceDir(), ConfigDir(), DataDir(), LegacyDataDirSO() (+5 more)

### Community 85 - ".Write"
Cohesion: 0.23
Nodes (6): PiExport, PiImport, SessionRef, peekPi(), piHome(), piText()

### Community 86 - "Refresh"
Cohesion: 0.27
Nodes (12): maxSharedMtime(), startRefreshWatcher(), writeRefreshStatus(), gitSHA(), Time, loadRefreshMarker(), Refresh(), refreshMarkerPath() (+4 more)

### Community 87 - "harness_hooks.go"
Cohesion: 0.25
Nodes (13): Decision, extractToolTargets(), findRepoRoot(), isSessionEndEvent(), isSessionStartEvent(), isToolGateEvent(), isTurnBoundaryHarvestEvent(), maybeAuditApproval() (+5 more)

### Community 88 - "codex/handle_test.go"
Cohesion: 0.32
Nodes (13): T, TestApplyPatchBodySupportsCodexHookCommand(), TestApplyPatchBodyUnwrapsCodeMode(), TestCodeModeApplyPatchEmitsFileDecisions(), TestCodexEndToEndOneTurn(), TestCodexMetadataModeDropsBodies(), TestCodexReasoningObservedWithEmptySummary(), TestCodexReasoningSummaryFromRollout() (+5 more)

### Community 89 - "stripCursorHooks"
Cohesion: 0.29
Nodes (12): RawMessage, isOurHookEntry(), stripCursorHooks(), T, TestStripCursorHooks_DeletesFileWhenOnlyOursAndVersion(), TestStripCursorHooks_KeepsFileWhenOtherTopLevelKeysExist(), TestStripCursorHooks_MissingFileIsNoOp(), TestStripCursorHooks_NoOpWhenNothingOurs() (+4 more)

### Community 90 - "NewStore"
Cohesion: 0.25
Nodes (11): NewStore(), T, TestBuildSessionContextAndLesson(), TestDeleteLesson(), TestIsStubMarkdown(), TestSeedReplacesStubOnly(), TestTemporaryMode(), defaultTemplate() (+3 more)

### Community 91 - "Init"
Cohesion: 0.25
Nodes (11): MeterProvider, SetCaptureMessageContent(), GetConfig(), Config, Resource, Init(), newMeterProvider(), newResource() (+3 more)

### Community 92 - "guardrails-dashboard.ts"
Cohesion: 0.25
Nodes (10): GET(), dynamic, GET(), runtime, AuditEvent, listAuditEvents(), classify(), dayKey() (+2 more)

### Community 93 - ".paths"
Cohesion: 0.23
Nodes (3): SOHubExport, SOHubImport, SessionRef

### Community 94 - "NewStore"
Cohesion: 0.28
Nodes (11): T, TestIsEmptyListItem(), TestSpansHaveActivity(), TestUpsertSkipsIdentityOnly(), T, TestResolveNestedParentClearsOrphanSubagentFlag(), TestUpsertActiveFromSpansDoesNotPoisonParentWithSubagentType(), TestUpsertActiveFromSpansNestsSubagents() (+3 more)

### Community 95 - "StateStore"
Cohesion: 0.32
Nodes (4): Time, Phase, State, StateStore

### Community 96 - "AgentsPanel.tsx"
Cohesion: 0.22
Nodes (11): AgentGraph, AgentNode, agentDetail(), AgentDetailPopover(), AgentDetailState, AgentRow(), AgentsPanel(), AgentsPanelProps (+3 more)

### Community 97 - "Default"
Cohesion: 0.24
Nodes (9): Default(), NormalizeModelSlug(), T, TestAutoApplyTiers(), TestModelForCLI(), TestNormalizeModelSlug(), TestNormalizeObservabilityLocalOnly(), T (+1 more)

### Community 98 - "Status"
Cohesion: 0.35
Nodes (9): hookBinaryAvailable(), Install(), Status(), T, TestStatusClaudeCodeDetectsStaleBinaryPath(), TestStatusClaudeCodeMissingManifest(), TestStatusClaudeCodeOKWithValidBinaryPath(), writeClaudeManifest() (+1 more)

### Community 99 - "harness-files-page.tsx"
Cohesion: 0.24
Nodes (5): FileEntry, fileNameFromPath(), HarnessFilesPage(), HarnessFilesPageInner(), matchEntry()

### Community 100 - "theme-provider.tsx"
Cohesion: 0.26
Nodes (9): metadata, applyDomTheme(), isPreference(), readStoredPreference(), ResolvedTheme, systemResolved(), ThemeContext, ThemeContextValue (+1 more)

### Community 101 - "Classify"
Cohesion: 0.29
Nodes (9): Classification, Inputs, Classify(), EnvAllowlist(), matchAllowlist(), SplitAllowlist(), T, TestClassify() (+1 more)

### Community 102 - "DefaultPolicy"
Cohesion: 0.44
Nodes (10): decideGuardrail(), T, TestDecideGuardrailAllowsEmptyTargets(), TestDecideGuardrailAllowsNormalPath(), TestDecideGuardrailDeniesCommand(), TestDecideGuardrailDeniesSensitivePath(), TestDecideGuardrailPathOnlyDoesNotUseZeroValueDeny(), TestIsSessionEndEventParity() (+2 more)

### Community 103 - "rec"
Cohesion: 0.22
Nodes (5): Session, T, ToolCall, TestOpenCodeMessageUsagePrefersHostCost(), rec

### Community 104 - "Rebuild"
Cohesion: 0.29
Nodes (9): QueryRetrieve(), indexPath(), Rebuild(), Search(), snippetAround(), T, TestRebuildAndSearch(), Hit (+1 more)

### Community 105 - "portToHub"
Cohesion: 0.19
Nodes (10): ClaudeCode(), Codex(), portToHub(), RegisterAll(), DefaultLedgerPath(), Mutex, Time, Ledger (+2 more)

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

### Community 111 - "harnessvalid.go"
Cohesion: 0.36
Nodes (9): SoftWrite, Applyable(), HasEvidence(), SafeJoin(), stripYAMLComments(), ValidateEvalsBody(), ValidateGuardrailsBody(), ValidatePath() (+1 more)

### Community 112 - "dropdown.tsx"
Cohesion: 0.22
Nodes (8): HARNESSES, PortWizard(), PortWizardProps, SessionRef, Dropdown(), DropdownOption, DropdownProps, MenuPos

### Community 113 - ".EmitEditDecision"
Cohesion: 0.47
Nodes (7): commonAttrs(), KeyValue, initMetrics(), recordCommit(), recordEditDecision(), recordLines(), recordPullRequest()

### Community 114 - "Prune"
Cohesion: 0.44
Nodes (8): Config, Time, Prune(), pruneAudit(), pruneEvalHistory(), pruneRecommendations(), pruneTraceFiles(), Report

### Community 115 - "Explain"
Cohesion: 0.25
Nodes (7): Explain(), Client, Meta, shortSHA(), truncate(), Footprint, FootprintFile

### Community 116 - "treeLayout.ts"
Cohesion: 0.33
Nodes (7): centerOf(), nearbyFiles(), Node, TreeDir, TreeEdge, TreeLayout, CityFile

### Community 117 - "dirLabels.ts"
Cohesion: 0.25
Nodes (4): DirLabel, DirLabelEntry, DirLabelSet, labelTexture()

### Community 118 - "blame.go"
Cohesion: 0.39
Nodes (7): LineInfo, WhyResult, File(), Store, isHex(), sessionIDForCommit(), Why()

### Community 119 - "config_test.go"
Cohesion: 0.39
Nodes (7): LLMSetupGuide(), T, TestGuardrailsEnabledEnv(), TestLLMSetupGuide(), TestLoadMigratesLegacyGovernanceKey(), TestResolveLLMLocal(), TestResolveLLMOpenRouter()

### Community 120 - "scripts"
Cohesion: 0.25
Nodes (8): scripts, build, dev, lint, start, test, test:watch, typecheck

### Community 121 - "inputBuilder"
Cohesion: 0.76
Nodes (6): T, inputBuilder(), TestClaudePreToolUseTaskLinksSubagent(), TestClaudeSessionDurationCachedAcrossInvocations(), TestClaudeSubagentStopWithoutPreToolUse(), withIsolatedCache()

### Community 122 - "marketplace/plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 123 - "launch.go"
Cohesion: 0.57
Nodes (6): agentBinary(), ensureInstalled(), NewCmd(), run(), vendorOf(), writeMemoryPack()

### Community 124 - "EstimateTokens"
Cohesion: 0.48
Nodes (5): EstimateTokens(), T, TestEstimateTokensEmpty(), TestEstimateTokensShort(), TestEstimateTokensWordFloor()

### Community 125 - "plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 126 - "Config"
Cohesion: 0.33
Nodes (4): Config, IDGenerator, Duration, Sampler

### Community 127 - "marketplace/plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 128 - "Display"
Cohesion: 0.40
Nodes (4): Display(), NewCmd(), T, TestDisplaySemver()

### Community 129 - "plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 130 - "install.sh script"
Cohesion: 0.67
Nodes (5): fatal(), info(), need(), install.sh script, warn()

### Community 131 - "adapter"
Cohesion: 0.40
Nodes (3): adapter, Adapter, New()

### Community 135 - "Run"
Cohesion: 0.70
Nodes (4): Check, detectAgentCLIs(), Format(), Run()

### Community 136 - "axi_test.go"
Cohesion: 0.60
Nodes (4): T, TestEmptyText(), TestRowsJSON(), TestTruncate()

### Community 138 - "install.go"
Cohesion: 0.83
Nodes (3): NewCmd(), run(), vendorsFromArg()

### Community 140 - "web/package.json"
Cohesion: 0.50
Nodes (3): name, private, version

### Community 152 - "evaluations/page.tsx"
Cohesion: 0.11
Nodes (19): EVAL_COLS, EvalRow, EvalsDashboardView(), fmtCost(), fmtTokens(), LOG_COLS, LogFilter, PageTab (+11 more)

### Community 175 - ".Write"
Cohesion: 0.23
Nodes (6): CodexExport, CodexImport, codexMessageText(), codexRoot(), SessionRef, peekCodex()

## Knowledge Gaps
- **293 isolated node(s):** `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter`, `Emitter`, `sessionRootMarkerKey` (+288 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **22 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Command` connect `cmds_dev.go` to `cmdGitHook`, `cmds_extra.go`, `Resolve`, `shell/sidebar.tsx`, `launch.go`?**
  _High betweenness centrality (0.401) - this node is a cross-community bridge._
- **Why does `cn()` connect `cn` to `sessions/page.tsx`, `memory.ts`, `harness-yaml.ts`, `sessions/[id]/page.tsx`, `dropdown.tsx`, `session-timeline.tsx`, `shell/sidebar.tsx`, `playground-shell.tsx`, `evaluations/page.tsx`, `utils.ts`, `file-content-view.tsx`?**
  _High betweenness centrality (0.208) - this node is a cross-community bridge._
- **Why does `Resolve()` connect `Resolve` to `cli.go`, `Run`, `TestResolveAndEnsureDirs`, `Paths`, `home`, `Emitter`, `.Create`, `recommend.go`, `Profile`, `guardrails.go`, `graph.go`, `backend.go`, `cmdGitHook`, `HarnessID`, `discover.go`, `cmds_dev.go`, `finalizeSession`, `cmds_extra.go`, `Run`, `CursorImport`, `Refresh`, `harness_hooks.go`, `NewStore`, `.paths`, `Default`, `Rebuild`, `Run`, `launch.go`?**
  _High betweenness centrality (0.200) - this node is a cross-community bridge._
- **Are the 78 inferred relationships involving `Resolve()` (e.g. with `.paths()` and `.soRoot()`) actually correct?**
  _`Resolve()` has 78 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter` to the rest of the system?**
  _293 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `release-packages.py` be split into smaller, more focused modules?**
  _Cohesion score 0.06155950752393981 - nodes in this community are weakly interconnected._
- **Should `Store` be split into smaller, more focused modules?**
  _Cohesion score 0.07589984350547731 - nodes in this community are weakly interconnected._