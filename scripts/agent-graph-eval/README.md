# Agent graph eval

Run:

```bash
go build -o /tmp/so ./cmd/so
python3 scripts/agent-graph-eval/run_eval.py --so-bin /tmp/so --arms vanilla,superopen
```

Optional peer comparison arms (binaries supplied via flags; no product names in-repo):

```bash
python3 scripts/agent-graph-eval/run_eval.py \
  --so-bin /tmp/so \
  --peer-cli "$PEER_CLI" \
  --peer-mcp "$PEER_MCP" \
  --arms vanilla,superopen,peer_cli,peer_mcp
```
