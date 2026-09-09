package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/config"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Operation struct {
	Name     string
	Path     []string
	Flags    *pflag.FlagSet
	Tool     *sdk.Tool
	ReadOnly bool
}

type Catalog struct {
	newRoot    func() *cobra.Command
	operations map[string]Operation
	ordered    []string
}

func NewCatalog(newRoot func() *cobra.Command) *Catalog {
	catalog := &Catalog{newRoot: newRoot, operations: map[string]Operation{}}
	walk(newRoot(), nil, func(command *cobra.Command, path []string) {
		flags := commandFlags(command)
		tool := commandTool(command, path, flags)
		catalog.operations[tool.Name] = Operation{
			Name:     tool.Name,
			Path:     append([]string(nil), path...),
			Flags:    flags,
			Tool:     tool,
			ReadOnly: tool.Annotations != nil && tool.Annotations.ReadOnlyHint,
		}
		catalog.ordered = append(catalog.ordered, tool.Name)
	})
	sort.Strings(catalog.ordered)
	return catalog
}

func (c *Catalog) Operations() []Operation {
	operations := make([]Operation, 0, len(c.ordered))
	for _, name := range c.ordered {
		operations = append(operations, c.operations[name])
	}
	return operations
}

func (c *Catalog) Operation(name string) (Operation, bool) {
	operation, ok := c.operations[name]
	return operation, ok
}

func (c *Catalog) Execute(ctx context.Context, operation Operation, raw json.RawMessage) (json.RawMessage, error) {
	if _, err := config.ResolveToken(); err != nil {
		return nil, fmt.Errorf("authentication unavailable")
	}
	args, err := commandArgs(operation.Path, operation.Flags, raw)
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	command := c.newRoot()
	command.SetArgs(args)
	command.SetIn(unavailableInput{})
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	if err := command.ExecuteContext(ctx); err != nil {
		return nil, fmt.Errorf("command failed: %s", strings.TrimSpace(stderr.String()))
	}
	output := bytes.TrimSpace(stdout.Bytes())
	if !json.Valid(output) {
		return nil, fmt.Errorf("command returned invalid JSON")
	}
	return append(json.RawMessage(nil), output...), nil
}
