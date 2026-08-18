package client

import (
	"context"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestCallRoundTrip(t *testing.T) {
	client, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	var result api.CapabilitySet
	if err := client.Call(context.Background(), api.OpCapabilities, struct{}{}, &result); err != nil {
		t.Fatal(err)
	}
	if result.AssetRevision == "" {
		t.Fatalf("unexpected empty capability set: %+v", result)
	}
}
