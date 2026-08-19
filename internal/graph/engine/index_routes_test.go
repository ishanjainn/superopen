package engine

import "testing"

func TestCanonicalRoutePathNormalizesFrameworkPlaceholders(t *testing.T) {
	for input, want := range map[string]string{
		"/api/files/${path}": "/api/files/{}",
		"/users/:id":         "/users/{}",
		"/users/{user_id}":   "/users/{}",
		"/users/<int:id>":    "/users/{}",
	} {
		if got := canonicalRoutePath(input); got != want {
			t.Errorf("canonicalRoutePath(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestPinnedGenericRouteValidation(t *testing.T) {
	for _, test := range []struct {
		path string
		want bool
	}{
		{"/api/files/{}", true},
		{"/tmp/safe.txt", true},
		{"/sessions", false},
		{"/a//b", false},
		{"/x/*", false},
	} {
		if got := validGenericArgURL(test.path); got != test.want {
			t.Errorf("validGenericArgURL(%q)=%v, want %v", test.path, got, test.want)
		}
	}
}

func TestHTTPRouteLiteralRejectsFilesystemPaths(t *testing.T) {
	for _, test := range []struct {
		path, callee string
		want         bool
	}{
		{"/api/files/config.json", "fetch", true},
		{"/.ssh/config", "fetch", false},
		{"/tmp/data", "http.Get", false},
		{"/users/42", "value.split", false},
		{"https://example.test/users", "fetch", true},
	} {
		if got := isHTTPRouteLiteral(test.path, test.callee); got != test.want {
			t.Errorf("isHTTPRouteLiteral(%q, %q)=%v, want %v", test.path, test.callee, got, test.want)
		}
	}
}

func TestMatchServiceRejectsDioInsideAudio(t *testing.T) {
	for _, identity := range []string{
		"expect(parsed.audioUrl).toBe",
		"AssemblyAIWrapper._parseAudioArgs",
		"AssemblyAIWrapper._commonAudioSetter",
	} {
		if kind, _ := matchService(identity); kind != serviceNone {
			t.Fatalf("matchService(%q)=%v, want none (dio must not match inside audio*)", identity, kind)
		}
	}
	if kind, _ := matchService("package:dio/dio.dart"); kind != serviceHTTP {
		t.Fatalf("real dio client must still match, got %v", kind)
	}
	if kind, _ := matchService("net/http.Client"); kind != serviceHTTP {
		t.Fatalf("net/http must still match, got %v", kind)
	}
}
