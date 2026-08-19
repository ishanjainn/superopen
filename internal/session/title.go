package session

import "strings"

func DisplayName(meta Meta) string {
	if title := strings.TrimSpace(meta.Title); !IsPlaceholderTitle(title, meta.ID) {
		return title
	}
	if preview := strings.TrimSpace(meta.PromptPreview); preview != "" {
		return preview
	}
	return meta.ID
}

func IsPlaceholderTitle(title, sessionID string) bool {
	title = strings.TrimSpace(title)
	return title == "" || title == sessionID || strings.EqualFold(title, "untitled")
}
