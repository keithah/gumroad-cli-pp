package codemode_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmd"
	codemode "github.com/antiwork/gumroad-cli/internal/cmd/codemode"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func connect(t *testing.T) (*sdk.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := codemode.NewServer(cmd.NewRootCmd).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, ctx
}

func call(t *testing.T, client *sdk.ClientSession, ctx context.Context, code string) (string, bool) {
	t.Helper()
	result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "gumroad", Arguments: map[string]any{"code": code}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("result = %#v", result)
	}
	return result.Content[0].(*sdk.TextContent).Text, result.IsError
}

func TestOversizedResultIsExplicit(t *testing.T) {
	client, ctx := connect(t)
	text, isError := call(t, client, ctx, "return 'x'.repeat(70000)")
	if isError || !strings.Contains(text, `"truncated":true`) || !strings.Contains(text, `"value":null`) {
		t.Fatalf("isError=%v response=%s", isError, text)
	}
}

func TestServerRejectsOversizedCode(t *testing.T) {
	client, ctx := connect(t)
	text, isError := call(t, client, ctx, strings.Repeat("x", 64<<10+1))
	if !isError || !strings.Contains(text, `"code_too_large"`) {
		t.Fatalf("isError=%v response=%s", isError, text)
	}
}

func TestServerRejectsMissingCode(t *testing.T) {
	client, ctx := connect(t)
	result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "gumroad", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].(*sdk.TextContent).Text, `"invalid_input"`) {
		t.Fatalf("result=%#v", result)
	}
}

func TestCodeModeCommandServesStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer serverRead.Close()
	defer serverWrite.Close()
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"codemode"})
	root.SetIn(serverRead)
	root.SetOut(serverWrite)
	root.SetErr(io.Discard)
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &sdk.IOTransport{Reader: clientRead, Writer: clientWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "gumroad" {
		t.Fatalf("tools = %#v", result.Tools)
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not stop")
	}
}

func TestRuntimeStopsAtContextDeadline(t *testing.T) {
	runtime := codemode.NewRuntime(cmd.NewRootCmd)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := runtime.Execute(ctx, "while (true) {}")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err=%v", err)
	}
}

func TestRuntimeHelpAndInvalidSource(t *testing.T) {
	runtime := codemode.NewRuntime(cmd.NewRootCmd)
	for _, source := range []string{"await gumroad.help()", "await gumroad.help('products.list')"} {
		value, err := runtime.Execute(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		serialized, err := json.Marshal(value)
		if err != nil || !strings.Contains(string(serialized), "products") {
			t.Fatalf("value=%v err=%v", value, err)
		}
	}
	for _, source := range []string{"const =", "await gumroad.help(1)", "await gumroad.help('missing.operation')", "await gumroad.products.list('bad')", "await gumroad.products.create({}, {confirm: 'yes'})"} {
		if _, err := runtime.Execute(context.Background(), source); err == nil {
			t.Fatalf("source %q unexpectedly succeeded", source)
		}
	}
}

func TestRuntimeErrorsAreStructured(t *testing.T) {
	client, ctx := connect(t)
	for _, source := range []string{"process.exit()", "gumroad.missing.operation()", "await gumroad.products.list({unknown: true})"} {
		text, isError := call(t, client, ctx, source)
		if !isError || !strings.Contains(text, `"ok":false`) {
			t.Fatalf("source=%q isError=%v response=%s", source, isError, text)
		}
	}
}

func TestMutationExecutesWithExactConfirmation(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/products" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		testutil.RawJSON(t, writer, `{"success":true,"product":{"id":"product-id"}}`)
	})
	client, ctx := connect(t)
	text, isError := call(t, client, ctx, "await gumroad.products.create({name: 'Draft'}, {confirm: true})")
	if isError || !strings.Contains(text, `"ok":true`) || requests != 1 {
		t.Fatalf("isError=%v requests=%d response=%s", isError, requests, text)
	}
}

func TestReadRejectsConfirmationOptions(t *testing.T) {
	client, ctx := connect(t)
	text, isError := call(t, client, ctx, "await gumroad.products.list({}, {confirm: true})")
	if !isError || !strings.Contains(text, `"execution_error"`) {
		t.Fatalf("isError=%v response=%s", isError, text)
	}
}

func TestMutationPlansWithoutConfirmation(t *testing.T) {
	testutil.Setup(t, func(_ http.ResponseWriter, request *http.Request) {
		t.Fatalf("unexpected mutation request: %s %s", request.Method, request.URL.Path)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := codemode.NewServer(cmd.NewRootCmd).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "gumroad", Arguments: map[string]any{"code": "await gumroad.products.create({name: 'Draft'})"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %#v", result)
	}
	text := result.Content[0].(*sdk.TextContent).Text
	for _, want := range []string{`"executed":false`, `"requires_confirmation":true`, `"operation":"products.create"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}

func TestReadExecutesWithoutConfirmation(t *testing.T) {
	testutil.Setup(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/products" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		testutil.RawJSON(t, writer, `{"success":true,"products":[]}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := codemode.NewServer(cmd.NewRootCmd).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "gumroad", Arguments: map[string]any{"code": "await gumroad.products.list()"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %#v", result)
	}
	text := result.Content[0].(*sdk.TextContent).Text
	if !strings.Contains(text, `"ok":true`) {
		t.Fatal(text)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		t.Fatal(err)
	}
}

func TestEnumerationExposesOnlyGumroadTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := codemode.NewServer(cmd.NewRootCmd).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	result, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "gumroad" {
		t.Fatalf("tools = %#v", result.Tools)
	}
}
