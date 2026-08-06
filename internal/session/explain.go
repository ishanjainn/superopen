package session

import (
	"fmt"
	"strings"

	"github.com/superopen/so/internal/llm"
)

// Explain builds a human-readable explanation of a session.
func Explain(meta Meta, footprint Footprint, client *llm.Client) (string, error) {
	var b strings.Builder
	title := meta.Title
	if title == "" {
		title = meta.PromptPreview
	}
	fmt.Fprintf(&b, "Session %s\n", meta.ID)
	fmt.Fprintf(&b, "  title:   %s\n", title)
	fmt.Fprintf(&b, "  vendor:  %s\n", meta.Vendor)
	if meta.Model != "" {
		fmt.Fprintf(&b, "  model:   %s\n", meta.Model)
	}
	fmt.Fprintf(&b, "  status:  %s\n", meta.Status)
	fmt.Fprintf(&b, "  tokens:  %d\n", meta.Tokens)
	if meta.CostUSD > 0 {
		fmt.Fprintf(&b, "  cost:    $%.4f\n", meta.CostUSD)
	}
	if meta.Branch != "" {
		fmt.Fprintf(&b, "  branch:  %s\n", meta.Branch)
	}
	if meta.HeadSHA != "" {
		fmt.Fprintf(&b, "  head:    %s\n", shortSHA(meta.HeadSHA))
	}
	if len(meta.Commits) > 0 {
		fmt.Fprintf(&b, "  commits:\n")
		for _, c := range meta.Commits {
			fmt.Fprintf(&b, "    - %s %s\n", shortSHA(c.SHA), truncate(c.Message, 60))
		}
	}
	if len(meta.PullRequests) > 0 {
		fmt.Fprintf(&b, "  pull requests:\n")
		for _, pr := range meta.PullRequests {
			fmt.Fprintf(&b, "    - #%d %s\n", pr.Number, pr.Title)
		}
	}
	if meta.Attribution != nil && meta.Attribution.Display != "" {
		fmt.Fprintf(&b, "  attribution: %s\n", meta.Attribution.Display)
	}
	if len(footprint.Files) > 0 {
		fmt.Fprintf(&b, "  files (%d):\n", len(footprint.Files))
		for i, f := range footprint.Files {
			if i >= 20 {
				fmt.Fprintf(&b, "    … %d more\n", len(footprint.Files)-20)
				break
			}
			fmt.Fprintf(&b, "    - [%s] %s\n", f.State, f.Path)
		}
	}
	if meta.Summary != "" {
		fmt.Fprintf(&b, "\nSummary:\n%s\n", meta.Summary)
	} else if client != nil && client.Available() && meta.PromptPreview != "" {
		sum, err := client.Complete(
			"Summarize this coding-agent session in 3 short bullets (intent, outcome, open items).",
			meta.PromptPreview,
		)
		if err == nil && strings.TrimSpace(sum) != "" {
			meta.Summary = strings.TrimSpace(sum)
			fmt.Fprintf(&b, "\nSummary:\n%s\n", meta.Summary)
		}
	}
	return b.String(), nil
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
