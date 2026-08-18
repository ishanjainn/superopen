// Package client is the in-process native graph client used by `so`.
package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/version"
)

// Client invokes the native graph engine in-process.
type Client struct{}

// Resolve returns the in-process graph client.
func Resolve() (Client, error) {
	return Client{}, nil
}

// Call runs a graph operation against the embedded native engine.
func (c Client) Call(ctx context.Context, operation api.Operation, params any, result any) error {
	paramBody, err := json.Marshal(params)
	if err != nil {
		return err
	}
	response := (engine.Server{
		EngineVersion: version.Display(),
		Assets:        engine.EngineAssets,
	}).Handle(ctx, api.Request{
		Protocol:  api.ProtocolVersion,
		Operation: operation,
		Params:    paramBody,
	})
	if !response.OK {
		if response.Error == nil {
			return fmt.Errorf("graph %s failed without an error payload", operation)
		}
		return fmt.Errorf("graph %s: %s", response.Error.Code, response.Error.Message)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("decode graph result: %w", err)
	}
	return nil
}
