package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmd"
	mcpcmd "github.com/antiwork/gumroad-cli/internal/cmd/mcp"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestCatalogMatchesGeneratedMCPToolNames(t *testing.T) {
	catalog := mcpcmd.NewCatalog(cmd.NewRootCmd)
	session, ctx := connect(t, cmd.NewRootCmd)
	tools := listTools(t, session, ctx)
	operations := catalog.Operations()
	if len(operations) != len(tools) {
		t.Fatalf("operation count = %d, tool count = %d", len(operations), len(tools))
	}
	for _, operation := range operations {
		if tools[operation.Name] == nil {
			t.Errorf("missing generated tool %s", operation.Name)
		}
	}
	for _, name := range []string{"products_list", "products_create", "licenses_verify", "sales_refund"} {
		operation, ok := catalog.Operation(name)
		if !ok {
			t.Fatalf("missing operation %s", name)
		}
		if got := operation.ReadOnly; got != (name == "products_list") {
			t.Errorf("%s read-only = %v", name, got)
		}
	}
}

func TestCatalogExecutesSelectedOperation(t *testing.T) {
	testutil.Setup(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/products" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		testutil.RawJSON(t, writer, `{"success":true,"products":[]}`)
	})
	catalog := mcpcmd.NewCatalog(cmd.NewRootCmd)
	operation, ok := catalog.Operation("products_list")
	if !ok {
		t.Fatal("missing products_list")
	}
	output, err := catalog.Execute(context.Background(), operation, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatal(err)
	}
	if value["success"] != true {
		t.Fatalf("output = %s", output)
	}
}
