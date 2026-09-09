package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	catalog "github.com/antiwork/gumroad-cli/internal/cmd/mcp"
	"github.com/dop251/goja"
	"github.com/spf13/cobra"
)

const executionTimeout = 5 * time.Second

type interrupted struct{}

type Runtime struct {
	catalog *catalog.Catalog
}

func NewRuntime(newRoot func() *cobra.Command) Runtime {
	return Runtime{catalog: catalog.NewCatalog(newRoot)}
}

func (r Runtime) Execute(ctx context.Context, source string) (any, error) {
	runtime := goja.New()
	gumroad := runtime.NewObject()
	for _, operation := range r.catalog.Operations() {
		r.installOperation(runtime, gumroad, ctx, operation)
	}
	if err := gumroad.Set("help", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 1 {
			panic(runtime.NewTypeError("help accepts zero or one operation name"))
		}
		if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
			operations := make([]string, 0)
			for _, operation := range r.catalog.Operations() {
				operations = append(operations, strings.Join(operation.Path, "."))
			}
			return runtime.ToValue(operations)
		}
		name, ok := call.Argument(0).Export().(string)
		if !ok {
			panic(runtime.NewTypeError("help operation name must be a string"))
		}
		for _, operation := range r.catalog.Operations() {
			if strings.Join(operation.Path, ".") == name {
				return runtime.ToValue(map[string]any{"operation": name, "read_only": operation.ReadOnly, "schema": operation.Tool.InputSchema})
			}
		}
		panic(runtime.NewTypeError("unknown operation"))
	}); err != nil {
		return nil, err
	}
	if err := runtime.Set("gumroad", gumroad); err != nil {
		return nil, err
	}
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "await ") {
		source = "return " + source
	}
	executionContext, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		select {
		case <-executionContext.Done():
			runtime.Interrupt(interrupted{})
		case <-done:
		}
	}()
	result, err := runtime.RunString("(async () => { return await (async () => {\n" + source + "\n})() })()")
	close(done)
	if executionContext.Err() != nil {
		return nil, fmt.Errorf("execution timeout")
	}
	if err != nil {
		var interruptedError *goja.InterruptedError
		if errors.As(err, &interruptedError) {
			return nil, fmt.Errorf("execution interrupted")
		}
		return nil, fmt.Errorf("invalid JavaScript")
	}
	promise, ok := result.Export().(*goja.Promise)
	if !ok {
		return result.Export(), nil
	}
	if promise.State() == goja.PromiseStateRejected {
		return nil, fmt.Errorf("JavaScript execution failed")
	}
	if promise.State() != goja.PromiseStateFulfilled {
		return nil, fmt.Errorf("JavaScript did not complete")
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("execution timeout")
	default:
	}
	return promise.Result().Export(), nil
}

func (r Runtime) installOperation(runtime *goja.Runtime, root *goja.Object, ctx context.Context, operation catalog.Operation) {
	object := root
	for _, segment := range operation.Path[:len(operation.Path)-1] {
		name := strings.ReplaceAll(segment, "-", "_")
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) {
			next := runtime.NewObject()
			_ = object.Set(name, next)
			object = next
			continue
		}
		object = value.ToObject(runtime)
	}
	name := strings.ReplaceAll(operation.Path[len(operation.Path)-1], "-", "_")
	_ = object.Set(name, func(call goja.FunctionCall) goja.Value {
		raw := json.RawMessage(`{}`)
		if len(call.Arguments) > 2 {
			panic(runtime.NewTypeError("operation accepts arguments and options"))
		}
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			arguments, ok := call.Argument(0).Export().(map[string]any)
			if !ok {
				panic(runtime.NewTypeError("arguments must be an object"))
			}
			encoded, err := json.Marshal(arguments)
			if err != nil {
				panic(runtime.NewTypeError("arguments are not serializable"))
			}
			raw = encoded
		}
		confirmed := false
		if len(call.Arguments) == 2 && !goja.IsUndefined(call.Argument(1)) {
			options, ok := call.Argument(1).Export().(map[string]any)
			if !ok || len(options) != 1 {
				panic(runtime.NewTypeError("options must be {confirm: true}"))
			}
			value, exists := options["confirm"]
			confirmed, ok = value.(bool)
			if !exists || !ok {
				panic(runtime.NewTypeError("options must be {confirm: true}"))
			}
		}
		if operation.ReadOnly {
			if len(call.Arguments) == 2 {
				panic(runtime.NewTypeError("read-only operations do not accept options"))
			}
		} else if !confirmed {
			var arguments map[string]any
			_ = json.Unmarshal(raw, &arguments)
			return runtime.ToValue(map[string]any{"executed": false, "requires_confirmation": true, "operation": strings.Join(operation.Path, "."), "arguments": arguments})
		}
		output, err := r.catalog.Execute(ctx, operation, raw)
		if err != nil {
			panic(runtime.NewTypeError("operation failed"))
		}
		var value any
		if err := json.Unmarshal(output, &value); err != nil {
			panic(runtime.NewTypeError("operation returned invalid JSON"))
		}
		return runtime.ToValue(value)
	})
}
