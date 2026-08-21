# Superopen reference: query recipes

Load this only when the task needs a recipe below. The stop-early loop in `SKILL.md`
answers most structural questions without it.

`__SO_BIN__` is the binary path from `SKILL.md`. Every command accepts `--json` for full
fidelity; the default output is the compact agent view.

## Callers, callees, and paths

```bash
__SO_BIN__ graph trace <qualified-name> --direction incoming   # who calls it
__SO_BIN__ graph trace <qualified-name> --direction outgoing   # what it calls
__SO_BIN__ graph trace <qualified-name> --direction both --depth 2
__SO_BIN__ graph trace <from> <to>                             # path between two symbols
```

Depth above 3 rarely adds signal and costs tokens. Start at 1-2.

## Change impact

```bash
__SO_BIN__ graph impact --base main          # blast radius of the current diff
__SO_BIN__ graph impact <symbol> [<symbol>]  # blast radius of named symbols
```

Use this instead of tracing each changed symbol by hand.

## Cypher subset

`graph cypher` runs a read-only subset. Always bound it with `LIMIT`.

```bash
# Routes and their handlers
__SO_BIN__ graph cypher "MATCH (r:Route)-[:CALLS]->(f) RETURN r.name, f.qualified_name LIMIT 50"

# Cross-service HTTP edges with their properties
__SO_BIN__ graph cypher "MATCH (a)-[e:HTTP_CALLS]->(b) RETURN a.name, b.name, e.confidence LIMIT 50"

# What configures a symbol
__SO_BIN__ graph cypher "MATCH (a)-[e:CONFIGURES]->(b) RETURN a.name, e.config_key, b.name LIMIT 50"

# Which tests cover a file
__SO_BIN__ graph cypher "MATCH (t)-[:TESTS_FILE]->(f:File) WHERE f.path =~ '.*handler.*' RETURN t.qualified_name, f.path LIMIT 50"

# Environment variables a package reads
__SO_BIN__ graph cypher "MATCH (f:Function)-[:USAGE]->(v:EnvVar) RETURN v.name, f.qualified_name LIMIT 50"
```

## Quality analysis

These are the claims that need a bounded scope, because the graph records
what it indexed, not what exists.

```bash
# Candidate dead code: no inbound CALLS. Verify entry points and reflection by hand.
__SO_BIN__ graph cypher "MATCH (f:Function) WHERE NOT ()-[:CALLS]->(f) RETURN f.qualified_name LIMIT 100"

# High fan-out: refactor candidates
__SO_BIN__ graph cypher "MATCH (f:Function)-[:CALLS]->(c) RETURN f.qualified_name, count(c) AS out ORDER BY out DESC LIMIT 25"

# High fan-in: hot dependencies worth not breaking
__SO_BIN__ graph cypher "MATCH (c)-[:CALLS]->(f:Function) RETURN f.qualified_name, count(c) AS in ORDER BY in DESC LIMIT 25"

# Files that historically change together
__SO_BIN__ graph cypher "MATCH (a:File)-[e:FILE_CHANGES_WITH]->(b:File) RETURN a.path, b.path LIMIT 50"
```

## Architecture and coverage

```bash
__SO_BIN__ graph architecture                       # packages, languages, fan-in/out
__SO_BIN__ graph architecture --aspect clusters     # Leiden communities with labels
__SO_BIN__ graph architecture --path internal/api   # scope to a subtree
__SO_BIN__ graph coverage                           # indexed vs missed files
```

Check `graph coverage` before any negative or exhaustive claim, then read source for the
ranges it reports as missed. A clean coverage result means no recorded gap, not proof of
completeness.
