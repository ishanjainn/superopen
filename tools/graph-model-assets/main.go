// Command graph-model-assets installs the pinned, content-verified semantic
// model into internal/graph/engine/assets/model.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/engine"
)

func main() {
	var sourceTree, modelDir, output string
	flag.StringVar(&sourceTree, "source-tree", os.Getenv("SUPEROPEN_GRAPH_SOURCE"), "source tree containing model vectors (or SUPEROPEN_GRAPH_SOURCE)")
	flag.StringVar(&modelDir, "model-dir", "vendored/nomic", "model directory relative to source-tree")
	flag.StringVar(&output, "out", "", "output directory")
	flag.Parse()
	if err := run(context.Background(), sourceTree, modelDir, output); err != nil {
		fmt.Fprintln(os.Stderr, "graph-model-assets:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, sourceTree, modelDir, output string) error {
	if sourceTree == "" || output == "" {
		return errors.New("-source-tree and -out are required")
	}
	root, err := filepath.Abs(sourceTree)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD")
	commitBytes, err := command.Output()
	if err != nil {
		return err
	}
	commit := strings.TrimSpace(string(commitBytes))
	if commit != engine.AssetRevision {
		return fmt.Errorf("source-tree HEAD is %s, require asset revision %s", commit, engine.AssetRevision)
	}
	source := filepath.Join(root, modelDir)
	if _, _, err := engine.VerifyPinnedPretrainedAssets(os.DirFS(source), "code_tokens.txt", "code_vectors.bin"); err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"code_tokens.txt", "code_vectors.bin"} {
		body, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(output, name), body); err != nil {
			return err
		}
	}
	provenance := []byte(fmt.Sprintf("{\n  \"asset_revision\": %q,\n  \"model\": \"nomic-embed-code token vectors\",\n  \"tokens\": 40856,\n  \"dimensions\": 768\n}\n", commit))
	return atomicWrite(filepath.Join(output, "provenance.json"), provenance)
}

func atomicWrite(path string, body []byte) error {
	temporary := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(temporary, body, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
