package scope

import "testing"

func TestCurrentDefaultsTenant(t *testing.T) {
	t.Setenv("SUPEROPEN_TENANT", "")
	t.Setenv("SUPEROPEN_USER", "dev@example.com")
	got, err := Current("/repo/app")
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID != DefaultTenant || got.PrincipalID != "dev@example.com" || got.ProjectID == "" {
		t.Fatalf("%+v", got)
	}
}

func TestResolvePrincipalOrder(t *testing.T) {
	if got := resolvePrincipal("dev@example.com", "other", "alice", 501); got != "dev@example.com" {
		t.Fatalf("env user wins, got %q", got)
	}
	if got := resolvePrincipal("", "so-user", "alice", 501); got != "so-user" {
		t.Fatalf("SO_USER wins over username, got %q", got)
	}
	if got := resolvePrincipal("", "", "alice", 501); got != "alice" {
		t.Fatalf("username wins over uid, got %q", got)
	}
	if got := resolvePrincipal("", "", "", 501); got != "501" {
		t.Fatalf("uid is the fallback, got %q", got)
	}
	if got := resolvePrincipal("", "", "  ", -1); got != "" {
		t.Fatalf("no name and no uid must stay empty, got %q", got)
	}
}

func TestCheckRejectsEmpty(t *testing.T) {
	if err := Check(Scope{TenantID: "local"}); err == nil {
		t.Fatal("expected error")
	}
}
