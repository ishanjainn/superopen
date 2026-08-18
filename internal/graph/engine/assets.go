package engine

import "embed"

// EngineAssets are the bundled Tree-sitter grammars and semantic model used by
// the native graph engine. They ship inside the single `so` binary.
//
//go:embed assets/grammars/* assets/model/*
var EngineAssets embed.FS
