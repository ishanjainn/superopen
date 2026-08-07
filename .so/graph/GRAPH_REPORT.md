# Graph Report - /var/folders/fr/660k4_4n1gs3g_d3c939lcm40000gn/T/so-graphify-774406421  (2026-08-07)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 3241 nodes · 7694 edges · 173 communities (151 shown, 22 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 965 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- release-packages.py
- trace.ts
- Store
- Config
- Remove
- sessions.ts
- misc.ts
- Run
- codex/handle.go
- LLMTurn
- entitlement.go
- CityScene.tsx
- cn
- session-timeline.tsx
- html/route.ts
- soPath
- agentlinks.go
- memory.ts
- playground-shell.tsx
- harvest.go
- harness-yaml.ts
- types.ts
- Resolve
- sessions/[id]/page.tsx
- receiver.go
- repoCwd
- Store
- title.go
- blame.go
- .Create
- Context
- memory/page.tsx
- MapView.tsx
- handle
- recommend.go
- compilerOptions
- file-content-view.tsx
- shell/sidebar.tsx
- Input
- graph.go
- cmds_extra.go
- InstallVendor
- recs/page.tsx
- .Parse
- attrs.go
- sessionTraceContext
- attributes.go
- Out
- emitGitArtifactsCodex
- HarnessID
- judge.ts
- cli.go
- codex/transcript.go
- Load
- NewPortableSession
- cursorHooksFile
- session/store.go
- citymap.ts
- devDependencies
- guardrails.go
- run
- harness-single-doc-page.tsx
- PortableSession
- RemoveAll
- backend.go
- fileExists
- home
- dependencies
- cmds_dev.go
- portToHub
- Refresh
- theme-provider.tsx
- .Parse
- Run
- vendor_config_scrub_test.go
- Hud.tsx
- agentconfig/config.go
- handle
- Lookup
- reducer.ts
- maybeHarvestOnSessionEnd
- ResolveForVendor
- Emitter
- handle
- finalizeSession
- NewEmitter
- stripCursorHooks
- NewStore
- Init
- treeLayout.ts
- .Parse
- codex/handle_test.go
- NewCompleterForBackend
- redact_test.go
- NewStore
- AgentsPanel.tsx
- adapters/codex.go
- Prune
- Classify
- Status
- inputBuilder
- DefaultPolicy
- rec
- rec
- Paths
- MineTranscript
- Run
- npm/package.json
- Timeline.tsx
- claudecode/transcript.go
- defaultSampler
- harnessvalid.go
- config/route.ts
- dirLabels.ts
- .Verify
- scripts
- inputBuilder
- marketplace/plugins/opencode/superopen.ts
- Command
- EstimateTokens
- EnsureDefaults
- plugins/opencode/superopen.ts
- Config
- marketplace/plugins/pi/index.ts
- Display
- plugins/pi/index.ts
- install.sh script
- adapter
- dev_proc_unix.go
- dev_proc_windows.go
- axi_test.go
- startRefreshWatcher
- So
- install.go
- TestPatchManifestUsesAbsoluteBinaryForRefresh
- web/package.json
- TestInferProvider
- TestDetectHostFromBinary
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

## Communities (173 total, 22 thin omitted)

### Community 0 - "release-packages.py"
Cohesion: 0.06
Nodes (46): Any, apply_version_files(), body_for_component_evidence(), build_pr_evidence(), bump(), chunk_prs(), collect_prs(), collect_single_pr() (+38 more)

### Community 1 - "trace.ts"
Cohesion: 0.06
Nodes (69): GET(), POST(), dynamic, GET(), runtime, dynamic, GET(), runtime (+61 more)

### Community 2 - "Store"
Cohesion: 0.08
Nodes (41): Event, Append(), Time, List(), Path(), splitLines(), contains(), T (+33 more)

### Community 3 - "Config"
Cohesion: 0.05
Nodes (46): Config, CostConfig, EvalsConfig, ExporterConfig, GraphConfig, GuardrailsConfig, InjectConfig, LLMConfig (+38 more)

### Community 4 - "Remove"
Cohesion: 0.06
Nodes (55): InstallOptions, InstallResult, UninstallResult, Apply(), Brief(), EnsureGlobalSkill(), EnsureSkills(), fileExists() (+47 more)

### Community 5 - "sessions.ts"
Cohesion: 0.07
Nodes (62): dynamic, GET(), runtime, joinQuery(), KNOWN_AGENTS, KNOWN_TOOLS, Props, SessionSearchBar() (+54 more)

### Community 6 - "misc.ts"
Cohesion: 0.08
Nodes (52): dynamic, GET(), runtime, DELETE(), dynamic, GET(), runtime, dynamic (+44 more)

### Community 7 - "Run"
Cohesion: 0.07
Nodes (52): AgentSource, GraphSummary, Profile, Options, Report, BuildProfile(), cleanMD(), CollectAgentFiles() (+44 more)

### Community 8 - "codex/handle.go"
Cohesion: 0.12
Nodes (39): codexPayload, boolField(), buildSession(), buildToolCall(), canonicalStatus(), clearTurnFragment(), commandFromToolInput(), durationMsOr() (+31 more)

### Community 9 - "LLMTurn"
Cohesion: 0.07
Nodes (21): recordingEmitter, recordingEmitter, recordingEmitter, Session, ToolCall, Session, ToolCall, Session (+13 more)

### Community 10 - "entitlement.go"
Cohesion: 0.06
Nodes (35): Status, Clear(), CloudOTLPEnabled(), configDir(), Time, Load(), LoginPaid(), path() (+27 more)

### Community 11 - "CityScene.tsx"
Cohesion: 0.09
Nodes (40): useThemeOptional(), applyCitySceneTheme(), attentionColumnGeometry(), attentionHeight(), baseColor(), centerFor(), CityScene(), colors (+32 more)

### Community 12 - "cn"
Cohesion: 0.07
Nodes (36): EVAL_COLS, EvalRow, EvalsDashboardView(), fmtCost(), fmtTokens(), LOG_COLS, LogFilter, PageTab (+28 more)

### Community 13 - "session-timeline.tsx"
Cohesion: 0.07
Nodes (37): SessionRail(), SessionRailDrawer(), SessionRailProps, SessionRailTool, basename(), buildTimeline(), buildTimelineFromPortableTurns(), ChatMinimap() (+29 more)

### Community 14 - "html/route.ts"
Cohesion: 0.09
Nodes (35): dynamic, GET(), HEAD(), loadGraphHtml(), runtime, themeFromRequest(), Body, dynamic (+27 more)

### Community 15 - "soPath"
Cohesion: 0.10
Nodes (38): GET(), AuditEvent, listAuditEvents(), buildDailySeries(), buildEvaluatorStats(), dayKey(), EvalBadge, EvalFailurePoint (+30 more)

### Community 16 - "agentlinks.go"
Cohesion: 0.12
Nodes (39): Entry, fileDoc, pendingDoc, PendingSpawn, agentIDFromPath(), linkCursorSubagentLifecycle(), AllowRegister(), ClaimPending() (+31 more)

### Community 17 - "memory.ts"
Cohesion: 0.15
Nodes (38): dynamic, GET(), POST(), runtime, deleteLessonLocal(), deletePreferenceItem(), deleteProjectItem(), ensureMemoryDirs() (+30 more)

### Community 18 - "playground-shell.tsx"
Cohesion: 0.09
Nodes (30): BreadcrumbContext, BreadcrumbContextValue, BreadcrumbCrumb, BreadcrumbProvider(), useBreadcrumb(), HeaderContextRow(), Option, pageTitle() (+22 more)

### Community 19 - "harvest.go"
Cohesion: 0.15
Nodes (33): Delta, ledgerEntry, ledgerFile, pendingFile, Result, Trigger, appendFile(), applyDelta() (+25 more)

### Community 20 - "harness-yaml.ts"
Cohesion: 0.12
Nodes (32): FileContentView(), Domain, FILE, HarnessItemDetailPage(), hydrate(), KIND_BLURB, HarnessSingleDocPage(), appendDeniedCommand() (+24 more)

### Community 21 - "types.ts"
Cohesion: 0.07
Nodes (32): AgentKind, AgentLinkMethod, AgentLinkQuality, AgentStatus, CityDir, Coverage, DimensionName, Observability (+24 more)

### Community 22 - "Resolve"
Cohesion: 0.21
Nodes (33): cmdCheckpoint(), cmdImport(), cmdProjects(), cmdStatus(), cmdHarvest(), attachSessionsStart(), cmdAudit(), cmdLearn() (+25 more)

### Community 23 - "sessions/[id]/page.tsx"
Cohesion: 0.09
Nodes (25): EvaluationsInner(), GuardrailsInner(), RecsPageInner(), MapView, NestedSession, projectFromURL(), SessionDetailInner(), Tab (+17 more)

### Community 24 - "receiver.go"
Cohesion: 0.11
Nodes (28): AnyValue, anyValueString(), KeyValue, Span, Store, IsSubagentAttrs(), kvMap(), NewReceiver() (+20 more)

### Community 25 - "repoCwd"
Cohesion: 0.14
Nodes (25): dynamic, POST(), runtime, dynamic, POST(), runtime, dynamic, GET() (+17 more)

### Community 26 - "Store"
Cohesion: 0.15
Nodes (10): IndexEntry, ListItem, Store, IsEmptyListItem(), Client, Store, Span, rank() (+2 more)

### Community 27 - "title.go"
Cohesion: 0.12
Nodes (30): humanizePromptPreview(), DisplayName(), EnsureTitle(), generateTitle(), Client, Meta, loadCodexTitles(), loadOpenCodeTitles() (+22 more)

### Community 28 - "blame.go"
Cohesion: 0.09
Nodes (24): LineInfo, WhyResult, File(), Store, isHex(), sessionIDForCommit(), Why(), AppendTrailer() (+16 more)

### Community 29 - ".Create"
Cohesion: 0.13
Nodes (20): Meta, Store, containsDotDot(), Time, NewStore(), splitSlash(), stringsHasDotDot(), T (+12 more)

### Community 30 - "Context"
Cohesion: 0.09
Nodes (17): adapter, adapter, Adapter, Context, NormalizeRepoURL(), remoteURL(), run(), Snapshot() (+9 more)

### Community 31 - "memory/page.tsx"
Cohesion: 0.10
Nodes (22): Lesson, MemoryPage(), Pack, Section, SECTIONS, VERB_OPTIONS, VerbBadge(), dateGroupLabel() (+14 more)

### Community 32 - "MapView.tsx"
Cohesion: 0.14
Nodes (25): apiURL(), describeError(), getAgentTrace(), getJSON(), getSessionAgents(), getSessionReport(), getSessionSnapshot(), listSessions() (+17 more)

### Community 33 - "handle"
Cohesion: 0.17
Nodes (27): cursorAttachment, cursorEdit, cursorPayload, applyAccumulatedTokens(), buildPromptTurn(), buildResponseTurn(), buildSession(), buildToolCallFailure() (+19 more)

### Community 34 - "recommend.go"
Cohesion: 0.20
Nodes (27): advisoryGuardrailBody(), Apply(), containsStr(), Dismiss(), EnqueuePending(), FingerprintKey(), Generate(), Result (+19 more)

### Community 35 - "compilerOptions"
Cohesion: 0.07
Nodes (28): dom, dom.iterable, esnext, .next/dev/types/**/*.ts, next-env.d.ts, .next/types/**/*.ts, node_modules, **/*.ts (+20 more)

### Community 36 - "file-content-view.tsx"
Cohesion: 0.09
Nodes (25): buildEvalRows(), buildGuardRows(), EVAL_COLUMNS, EVAL_VISIBILITY, EvalColumnKey, EvalRow, EvalRowKind, EvaluationsDocument() (+17 more)

### Community 37 - "shell/sidebar.tsx"
Cohesion: 0.12
Nodes (23): FeaturePageHeader(), FeaturePageHeaderProps, iconForPath(), iconForTitle(), flatItems(), isActive(), NavigationLink(), PrimaryItem() (+15 more)

### Community 38 - "Input"
Cohesion: 0.16
Nodes (26): claudePayload, Emitter, CountInlineDiff(), splitLines(), bashStdout(), countMultiEditLines(), drainAssistantTurns(), drainRejectedPendingEdits() (+18 more)

### Community 39 - "graph.go"
Cohesion: 0.17
Nodes (25): Result, stubGraph, Build(), buildStub(), buildWithGraphify(), copyDir(), countGraph(), ensureGraphifyCommunitySidecars() (+17 more)

### Community 40 - "cmds_extra.go"
Cohesion: 0.15
Nodes (18): cmdBlame(), cmdGitHook(), cmdLogin(), cmdLogout(), cmdWhy(), extendSessionsCmd(), firstLine(), Meta (+10 more)

### Community 41 - "InstallVendor"
Cohesion: 0.14
Nodes (21): installGenericVendor(), enableClaudeCodePlugin(), enableCodexPlugin(), extractEmbeddedDir(), materializeClaudeMarketplace(), patchManifestBytes(), resolveCodexBin(), resolveSoBin() (+13 more)

### Community 42 - "recs/page.tsx"
Cohesion: 0.12
Nodes (20): actorLabel(), decisionDialogTitle(), decisionLabel(), decisionPlaceholder(), RecDetailPage(), StatusPill(), REC_COLS, REC_TABS (+12 more)

### Community 43 - ".Parse"
Cohesion: 0.16
Nodes (12): wsCollector, codexCallArgs(), observeOpenCodeToolPart(), capCommands(), capStrings(), extractExitCode(), firstStringField(), matchesAny() (+4 more)

### Community 44 - "attrs.go"
Cohesion: 0.23
Nodes (21): scrubFn, bodyAllowed(), buildInputMessagesJSON(), buildOutputMessagesJSON(), Session, Span, ToolCall, inferProvider() (+13 more)

### Community 45 - "sessionTraceContext"
Cohesion: 0.19
Nodes (21): sessionIDGenerator, sessionRootMarker, sessionRootMarkerKey, InMemoryExporter, deriveSessionRootSpanID(), deriveSessionTraceID(), randomSpanID(), randomTraceID() (+13 more)

### Community 46 - "attributes.go"
Cohesion: 0.17
Nodes (18): KeyValue, Span, NewAttributeBuilder(), SetBoolAttribute(), SetFloat64Attribute(), SetInt64Attribute(), SetIntAttribute(), SetJSONAttribute() (+10 more)

### Community 47 - "Out"
Cohesion: 0.15
Nodes (13): Error, Flags, Out, Bind(), chainPreRun(), envTruthy(), ExitCode(), Fail() (+5 more)

### Community 48 - "emitGitArtifactsCodex"
Cohesion: 0.17
Nodes (26): PatchLineCounts, CountPatchLines(), ExtractCommitMessage(), ExtractCommitSHA(), ExtractPRTitle(), ExtractPRURLAndNumber(), firstGroup(), IsGitCommit() (+18 more)

### Community 49 - "HarnessID"
Cohesion: 0.14
Nodes (13): NewRegistry(), RefreshMemoryAfterPort(), Orchestrator, SessionRef, Time, Event, ExportAdapter, HarnessID (+5 more)

### Community 50 - "judge.ts"
Cohesion: 0.15
Nodes (23): availableJudgeClis(), digestTrace(), errorPath(), extractJSON(), getReportStatus(), heuristicReport(), JUDGE_CLIS, judgePrompt() (+15 more)

### Community 51 - "cli.go"
Cohesion: 0.17
Nodes (19): claudeEnvelope, Result, Runner, codexExecArgs(), codexModel(), Detect(), DetectAll(), ensureWorkDir() (+11 more)

### Community 52 - "codex/transcript.go"
Cohesion: 0.20
Nodes (22): codexLine, codexReasoningItem, codexReasoningSummaryPart, codexTokenSnapshot, codexTokenUsage, codexTokenUsageInfo, sessionMeta, RawMessage (+14 more)

### Community 53 - "Load"
Cohesion: 0.25
Nodes (18): markCodexSessionRootEmitted(), accumulateStopTokens(), AddPendingEdit(), BumpCodeCounters(), DrainPendingEdits(), GC(), Duration, Time (+10 more)

### Community 54 - "NewPortableSession"
Cohesion: 0.20
Nodes (19): T, TestMaybeInjectMemory_PortResumeWithoutMemory(), NewPortableSession(), ArmResume(), consumeLegacyCursorPending(), ConsumePendingResume(), consumeSOPortPending(), portRunDir() (+11 more)

### Community 55 - "cursorHooksFile"
Cohesion: 0.21
Nodes (18): cursorHooksFile, RawMessage, installCursorHooks(), isOurHookEntry(), mergeCursorHooks(), readCursorHooksFile(), commandsFor(), RawMessage (+10 more)

### Community 56 - "session/store.go"
Cohesion: 0.12
Nodes (18): ComputeAttribution(), Time, Explain(), Client, Meta, shortSHA(), countCheckpointDirs(), Time (+10 more)

### Community 57 - "citymap.ts"
Cohesion: 0.14
Nodes (21): applyTreemap(), buildTree(), cachePath(), capAspect(), CityDir, CityFile, CityMap, computeWeight() (+13 more)

### Community 58 - "devDependencies"
Cohesion: 0.10
Nodes (21): autoprefixer, eslint, eslint-config-next, postcss, tailwindcss, @types/node, @types/react-dom, @types/three (+13 more)

### Community 59 - "guardrails.go"
Cohesion: 0.20
Nodes (14): Decision, Engine, File, Policy, Rule, CheckCommandString(), CheckPathString(), FormatDecision() (+6 more)

### Community 60 - "run"
Cohesion: 0.18
Nodes (19): peekedContext, canonicalVendor(), firstHookEnv(), firstNonEmpty(), foreignParentID(), Adapter, isClaudeCodeVendor(), isRealClaudeCodeInvocation() (+11 more)

### Community 61 - "harness-single-doc-page.tsx"
Cohesion: 0.14
Nodes (15): BACKENDS, SafeConfig, THEME_OPTIONS, CONFIG, EvaluationComposer(), GuardrailComposer(), Kind, DialogContent (+7 more)

### Community 62 - "PortableSession"
Cohesion: 0.11
Nodes (13): CursorExport, SOHubExport, SOHubImport, loadWorkingStateSidecar(), writeTranscript(), writeWorkingStateSidecar(), writeJSONL(), SessionRef (+5 more)

### Community 63 - "RemoveAll"
Cohesion: 0.19
Nodes (17): Writer, NewCmd(), RemoveAll(), run(), equalSlice(), T, TestRemovePath(), TestVendorsFromArg() (+9 more)

### Community 64 - "backend.go"
Cohesion: 0.18
Nodes (11): ListItem, ListItem, Meta, Store, metaHasCommit(), metaHasPR(), Backend, Composite (+3 more)

### Community 65 - "fileExists"
Cohesion: 0.22
Nodes (18): DELETE(), dynamic, GET(), POST(), PUT(), runtime, assertEditable(), createHarnessFile() (+10 more)

### Community 66 - "home"
Cohesion: 0.14
Nodes (9): ClaudeExport, ClaudeImport, OpenCodeExport, OpenCodeImport, encodeClaudeProjectDir(), home(), exportOpenCodeSQLiteJSON(), SessionRef (+1 more)

### Community 67 - "dependencies"
Cohesion: 0.11
Nodes (19): class-variance-authority, clsx, lucide-react, next, @radix-ui/react-tooltip, react, react-is, recharts (+11 more)

### Community 68 - "cmds_dev.go"
Cohesion: 0.38
Nodes (12): cmdDev(), logFile(), maybeOpenUI(), pidFile(), readDevStatus(), runDev(), runDevForeground(), runDir() (+4 more)

### Community 69 - "portToHub"
Cohesion: 0.19
Nodes (11): ClaudeCode(), Codex(), portToHub(), RegisterAll(), DefaultLedgerPath(), Mutex, Time, NewLedger() (+3 more)

### Community 70 - "Refresh"
Cohesion: 0.22
Nodes (17): gitSHA(), Time, indexableFilesChanged(), isIndexablePath(), loadRefreshMarker(), Refresh(), refreshMarkerPath(), saveRefreshMarker() (+9 more)

### Community 71 - "theme-provider.tsx"
Cohesion: 0.16
Nodes (14): GraphPage(), SettingsPage(), metadata, GraphifyFrame(), applyDomTheme(), isPreference(), readStoredPreference(), ResolvedTheme (+6 more)

### Community 72 - ".Parse"
Cohesion: 0.21
Nodes (9): CursorImport, SessionRef, peekClaude(), SessionRef, ensureMeta(), firstLine(), parseTimeMs(), textAndToolsFromContent() (+1 more)

### Community 73 - "Run"
Cohesion: 0.21
Nodes (16): activitySignals, Result, appendHistory(), collectActivitySignals(), containsAny(), extractJSON(), Config, Span (+8 more)

### Community 74 - "vendor_config_scrub_test.go"
Cohesion: 0.25
Nodes (16): FileMode, writeFileAtomic(), stripClaudeMarketplaceJSON(), stripCodexConfigTOML(), stripCodexOwnedSections(), T, mustJSONRead(), mustJSONWrite() (+8 more)

### Community 75 - "Hud.tsx"
Cohesion: 0.12
Nodes (8): ActionCounts, MetricObservability, ACTION_ORDER, ChurnEntry, countActions(), Hud, HudTool, MapSceneView

### Community 76 - "agentconfig/config.go"
Cohesion: 0.23
Nodes (15): Defaults, Flags, Resolved, builtinDefaults(), configDir(), isLocalHost(), isSecretKey(), Load() (+7 more)

### Community 77 - "handle"
Cohesion: 0.19
Nodes (13): accumulate(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), itoa() (+5 more)

### Community 78 - "Lookup"
Cohesion: 0.23
Nodes (14): Lookup(), T, TestCostBasic(), TestCostCacheCreationFallback(), TestCostCacheCreationPremium(), TestCostWithCacheRead(), TestCostZeroRate(), TestLookupAnthropic() (+6 more)

### Community 79 - "reducer.ts"
Cohesion: 0.21
Nodes (12): FilePlayback, PlaybackEngine, touchRank, CitySceneProps, TreeSceneProps, CityMap, Target, Touch (+4 more)

### Community 80 - "maybeHarvestOnSessionEnd"
Cohesion: 0.24
Nodes (15): Decision, extractToolTargets(), findRepoRoot(), isSessionEndEvent(), isSessionStartEvent(), isToolGateEvent(), isTurnBoundaryHarvestEvent(), maybeAuditApproval() (+7 more)

### Community 81 - "ResolveForVendor"
Cohesion: 0.23
Nodes (13): base64URLDecode(), claudeCodeEmail(), codexEmail(), decodeJWTSegment(), emailFromJWT(), FromGitConfig(), ResolveForVendor(), T (+5 more)

### Community 82 - "Emitter"
Cohesion: 0.14
Nodes (13): Emitter, perEventSpansAllowed(), Mutex, Time, ToolCall, Tracer, commonAttrs(), KeyValue (+5 more)

### Community 83 - "handle"
Cohesion: 0.21
Nodes (11): accumulateUsage(), applyAccumulated(), capture(), firstNonEmpty(), RawMessage, Session, handle(), stampUsage() (+3 more)

### Community 84 - "finalizeSession"
Cohesion: 0.13
Nodes (18): applyTracesDir(), demoSession(), finalizeLatestSession(), finalizeSession(), Config, refreshSession(), runRoot(), NewLocalMulti() (+10 more)

### Community 85 - "NewEmitter"
Cohesion: 0.29
Nodes (12): detectHostFromBinary(), detectHostFromProcessTree(), drainCodeCounters(), drainCounters(), Session, isSessionStart(), NewEmitter(), readProcessNameAndPPID() (+4 more)

### Community 86 - "stripCursorHooks"
Cohesion: 0.29
Nodes (12): RawMessage, isOurHookEntry(), stripCursorHooks(), T, TestStripCursorHooks_DeletesFileWhenOnlyOursAndVersion(), TestStripCursorHooks_KeepsFileWhenOtherTopLevelKeysExist(), TestStripCursorHooks_MissingFileIsNoOp(), TestStripCursorHooks_NoOpWhenNothingOurs() (+4 more)

### Community 87 - "NewStore"
Cohesion: 0.25
Nodes (11): NewStore(), T, TestBuildSessionContextAndLesson(), TestDeleteLesson(), TestIsStubMarkdown(), TestSeedReplacesStubOnly(), TestTemporaryMode(), defaultTemplate() (+3 more)

### Community 88 - "Init"
Cohesion: 0.25
Nodes (11): MeterProvider, SetCaptureMessageContent(), GetConfig(), Config, Resource, Init(), newMeterProvider(), newResource() (+3 more)

### Community 89 - "treeLayout.ts"
Cohesion: 0.22
Nodes (11): centerOf(), nearbyFiles(), Node, TreeDir, TreeEdge, TreeLayout, CityFile, touchWord() (+3 more)

### Community 90 - ".Parse"
Cohesion: 0.24
Nodes (6): PiExport, PiImport, SessionRef, peekPi(), piHome(), piText()

### Community 91 - "codex/handle_test.go"
Cohesion: 0.27
Nodes (15): applyPatchBody(), patchLiteralFromCodeMode(), T, TestApplyPatchBodySupportsCodexHookCommand(), TestApplyPatchBodyUnwrapsCodeMode(), TestCodeModeApplyPatchEmitsFileDecisions(), TestCodexEndToEndOneTurn(), TestCodexMetadataModeDropsBodies() (+7 more)

### Community 92 - "NewCompleterForBackend"
Cohesion: 0.31
Nodes (9): Config, mineOnFinalize(), Config, newAgentCLI(), NewBestCompleter(), NewCompleterForBackend(), NewMemoryCompleter(), AgentCLI (+1 more)

### Community 93 - "redact_test.go"
Cohesion: 0.26
Nodes (11): ForCapture(), scrubExfilURLs(), String(), StringFull(), T, TestEmptyString(), TestForCaptureSelector(), TestPostgresURLKeepsHostDropsCreds() (+3 more)

### Community 94 - "NewStore"
Cohesion: 0.28
Nodes (11): T, TestIsEmptyListItem(), TestSpansHaveActivity(), TestUpsertSkipsIdentityOnly(), T, TestResolveNestedParentClearsOrphanSubagentFlag(), TestUpsertActiveFromSpansDoesNotPoisonParentWithSubagentType(), TestUpsertActiveFromSpansNestsSubagents() (+3 more)

### Community 95 - "AgentsPanel.tsx"
Cohesion: 0.22
Nodes (11): AgentGraph, AgentNode, agentDetail(), AgentDetailPopover(), AgentDetailState, AgentRow(), AgentsPanel(), AgentsPanelProps (+3 more)

### Community 96 - "adapters/codex.go"
Cohesion: 0.24
Nodes (6): CodexExport, CodexImport, codexMessageText(), codexRoot(), SessionRef, peekCodex()

### Community 97 - "Prune"
Cohesion: 0.29
Nodes (10): Config, Time, Prune(), pruneAudit(), pruneEvalHistory(), pruneRecommendations(), pruneTraceFiles(), T (+2 more)

### Community 98 - "Classify"
Cohesion: 0.29
Nodes (9): Classification, Inputs, Classify(), EnvAllowlist(), matchAllowlist(), SplitAllowlist(), T, TestClassify() (+1 more)

### Community 99 - "Status"
Cohesion: 0.35
Nodes (9): hookBinaryAvailable(), Install(), Status(), T, TestStatusClaudeCodeDetectsStaleBinaryPath(), TestStatusClaudeCodeMissingManifest(), TestStatusClaudeCodeOKWithValidBinaryPath(), writeClaudeManifest() (+1 more)

### Community 100 - "inputBuilder"
Cohesion: 0.58
Nodes (10): T, inputBuilder(), TestCursorAfterAgentResponseNoEstimate(), TestCursorAfterAgentResponseOmitsTokensEvenWhenPresent(), TestCursorSessionIDPrefersConversation(), TestCursorStopAccumulatesIntoSessionEnd(), TestCursorStopStampsRealTokenAttrs(), TestCursorSubagentStartLinkage() (+2 more)

### Community 101 - "DefaultPolicy"
Cohesion: 0.44
Nodes (10): decideGuardrail(), T, TestDecideGuardrailAllowsEmptyTargets(), TestDecideGuardrailAllowsNormalPath(), TestDecideGuardrailDeniesCommand(), TestDecideGuardrailDeniesSensitivePath(), TestDecideGuardrailPathOnlyDoesNotUseZeroValueDeny(), TestIsSessionEndEventParity() (+2 more)

### Community 102 - "rec"
Cohesion: 0.24
Nodes (5): Session, T, ToolCall, TestOpenCodeMessageUsagePrefersHostCost(), rec

### Community 103 - "rec"
Cohesion: 0.20
Nodes (5): Session, T, ToolCall, TestPiHostCostPreferred(), rec

### Community 104 - "Paths"
Cohesion: 0.16
Nodes (14): Paths, QueryRetrieve(), FindRoot(), migrateDir(), migrateFile(), ConsumeFinalizePending(), indexPath(), Rebuild() (+6 more)

### Community 105 - "MineTranscript"
Cohesion: 0.29
Nodes (9): heuristicSkillBody(), MineSessionFile(), MineTranscript(), slugify(), T, TestMineTranscriptWritesLessonAndRec(), truncate(), Lesson (+1 more)

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

### Community 112 - "config/route.ts"
Cohesion: 0.31
Nodes (9): dynamic, formatYamlValue(), GET(), isTopLevelKey(), PatchBody, PUT(), runtime, setYamlPath() (+1 more)

### Community 114 - "dirLabels.ts"
Cohesion: 0.25
Nodes (4): DirLabel, DirLabelEntry, DirLabelSet, labelTexture()

### Community 115 - ".Verify"
Cohesion: 0.29
Nodes (5): Event, Orchestrator, RemapCWD(), HubFactory, VerifyResult

### Community 116 - "scripts"
Cohesion: 0.25
Nodes (8): scripts, build, dev, lint, start, test, test:watch, typecheck

### Community 117 - "inputBuilder"
Cohesion: 0.76
Nodes (6): T, inputBuilder(), TestClaudePreToolUseTaskLinksSubagent(), TestClaudeSessionDurationCachedAcrossInvocations(), TestClaudeSubagentStopWithoutPreToolUse(), withIsolatedCache()

### Community 118 - "marketplace/plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 119 - "Command"
Cohesion: 0.23
Nodes (12): cmdOpen(), openBrowser(), findWebDir(), Cmd, startNextUI(), agentBinary(), ensureInstalled(), NewCmd() (+4 more)

### Community 120 - "EstimateTokens"
Cohesion: 0.48
Nodes (5): EstimateTokens(), T, TestEstimateTokensEmpty(), TestEstimateTokensShort(), TestEstimateTokensWordFloor()

### Community 121 - "EnsureDefaults"
Cohesion: 0.48
Nodes (6): EnsureDefaults(), Path(), T, TestDenyCommandAndSensitivePath(), TestEnsureDefaultsMigratesLegacy(), TestMatchPathDoesNotDenyNormalFiles()

### Community 122 - "plugins/opencode/superopen.ts"
Cohesion: 0.57
Nodes (6): fire(), parseDeny(), parseInject(), runFinalize(), soBin(), SuperopenPlugin()

### Community 123 - "Config"
Cohesion: 0.33
Nodes (4): Config, IDGenerator, Duration, Sampler

### Community 124 - "marketplace/plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 125 - "Display"
Cohesion: 0.40
Nodes (4): Display(), NewCmd(), T, TestDisplaySemver()

### Community 126 - "plugins/pi/index.ts"
Cohesion: 0.47
Nodes (3): fire(), runFinalize(), soBin()

### Community 127 - "install.sh script"
Cohesion: 0.67
Nodes (5): fatal(), info(), need(), install.sh script, warn()

### Community 128 - "adapter"
Cohesion: 0.40
Nodes (3): adapter, Adapter, New()

### Community 132 - "axi_test.go"
Cohesion: 0.60
Nodes (4): T, TestEmptyText(), TestRowsJSON(), TestTruncate()

### Community 133 - "startRefreshWatcher"
Cohesion: 0.83
Nodes (3): maxSharedMtime(), startRefreshWatcher(), writeRefreshStatus()

### Community 135 - "install.go"
Cohesion: 0.83
Nodes (3): NewCmd(), run(), vendorsFromArg()

### Community 136 - "TestPatchManifestUsesAbsoluteBinaryForRefresh"
Cohesion: 0.67
Nodes (3): T, TestCodexStopHookUsesSessionsRefresh(), TestPatchManifestUsesAbsoluteBinaryForRefresh()

### Community 138 - "web/package.json"
Cohesion: 0.50
Nodes (3): name, private, version

## Knowledge Gaps
- **293 isolated node(s):** `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter`, `Emitter`, `sessionRootMarkerKey` (+288 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **22 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Command` connect `Command` to `cmds_extra.go`, `cmds_dev.go`, `shell/sidebar.tsx`, `Resolve`?**
  _High betweenness centrality (0.395) - this node is a cross-community bridge._
- **Why does `cn()` connect `cn` to `file-content-view.tsx`, `sessions.ts`, `shell/sidebar.tsx`, `theme-provider.tsx`, `recs/page.tsx`, `session-timeline.tsx`, `playground-shell.tsx`, `harness-yaml.ts`, `sessions/[id]/page.tsx`, `harness-single-doc-page.tsx`, `memory/page.tsx`?**
  _High betweenness centrality (0.203) - this node is a cross-community bridge._
- **Why does `Resolve()` connect `Resolve` to `Config`, `startRefreshWatcher`, `Run`, `TestResolveAndEnsureDirs`, `harvest.go`, `.Create`, `recommend.go`, `graph.go`, `cmds_extra.go`, `HarnessID`, `PortableSession`, `backend.go`, `cmds_dev.go`, `Refresh`, `.Parse`, `Run`, `maybeHarvestOnSessionEnd`, `finalizeSession`, `NewEmitter`, `NewStore`, `Prune`, `Paths`, `MineTranscript`, `Run`, `Command`, `EnsureDefaults`?**
  _High betweenness centrality (0.190) - this node is a cross-community bridge._
- **Are the 78 inferred relationships involving `Resolve()` (e.g. with `.paths()` and `.soRoot()`) actually correct?**
  _`Resolve()` has 78 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/ishanjainn/superopen`, `claudeEnvelope`, `Adapter` to the rest of the system?**
  _293 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `release-packages.py` be split into smaller, more focused modules?**
  _Cohesion score 0.06155950752393981 - nodes in this community are weakly interconnected._
- **Should `trace.ts` be split into smaller, more focused modules?**
  _Cohesion score 0.05949367088607595 - nodes in this community are weakly interconnected._