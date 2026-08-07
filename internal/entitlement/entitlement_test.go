package entitlement_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/entitlement"
)

func TestLoginPaidGate(t *testing.T) {
	dir := t.TempDir()
	entitlement.SetPathForTest(filepath.Join(dir, "auth.json"))

	if entitlement.CloudOTLPEnabled() {
		t.Fatal("expected disabled")
	}
	err := entitlement.LoginPaid("a@b.c", "tok", "http://collector", "http://query", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !entitlement.CloudOTLPEnabled() {
		t.Fatal("expected enabled")
	}
	st, err := entitlement.Load()
	if err != nil || st.Token != "tok" {
		t.Fatalf("token not persisted: %+v %v", st, err)
	}
	pub := st.Public()
	if pub.Token != "" {
		t.Fatal("public must strip token")
	}
	_ = entitlement.Clear()
	if entitlement.CloudOTLPEnabled() {
		t.Fatal("expected cleared")
	}
}
