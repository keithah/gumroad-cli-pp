package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const instructions = "Use the single gumroad tool with JavaScript. Read operations run directly. Mutations require {confirm: true}."
const maxCodeBytes = 64 << 10
const maxResultBytes = 64 << 10

type writerCloser struct{ io.Writer }

func (writerCloser) Close() error { return nil }

func NewCodeModeCmd(newRoot func() *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:               "codemode",
		Short:             "Serve Gumroad Code Mode over MCP stdio",
		Example:           "  gumroad codemode",
		Args:              cobra.NoArgs,
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(command *cobra.Command, _ []string) error {
			return NewServer(newRoot).Run(command.Context(), &sdk.IOTransport{Reader: io.NopCloser(command.InOrStdin()), Writer: writerCloser{command.OutOrStdout()}})
		},
	}
}

func NewServer(newRoot func() *cobra.Command) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "gumroad-codemode", Version: newRoot().Version}, &sdk.ServerOptions{Instructions: instructions})
	server.AddTool(&sdk.Tool{
		Name:        "gumroad",
		Description: "Run bounded Gumroad Code Mode JavaScript. Read operations execute directly; mutations require {confirm: true}.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"code": map[string]any{"type": "string"}}, "required": []string{"code"}, "additionalProperties": false},
	}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var input struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &input); err != nil || input.Code == "" {
			return result(map[string]any{"ok": false, "error": map[string]string{"code": "invalid_input", "message": "code must be a non-empty string"}}, true), nil
		}
		if len(input.Code) > maxCodeBytes {
			return result(map[string]any{"ok": false, "error": map[string]string{"code": "code_too_large", "message": "code exceeds the maximum size"}}, true), nil
		}
		value, err := NewRuntime(newRoot).Execute(ctx, input.Code)
		if err != nil {
			return result(map[string]any{"ok": false, "error": map[string]string{"code": "execution_error", "message": err.Error()}}, true), nil
		}
		return result(map[string]any{"ok": true, "value": value}, false), nil
	})
	return server
}

func result(value any, isError bool) *sdk.CallToolResult {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(fmt.Sprintf(`{"ok":false,"error":{"code":"serialization_error","message":"%s"}}`, err.Error()))
		isError = true
	}
	if len(encoded) > maxResultBytes {
		encoded, _ = json.Marshal(map[string]any{"ok": true, "truncated": true, "summary": map[string]int{"observed_bytes": len(encoded)}, "value": nil})
		isError = false
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded)}}, IsError: isError}
}
