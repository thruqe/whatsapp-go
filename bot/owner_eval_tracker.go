// eval_tracker.go tracks function invocations and metadata in the JavaScript runtime.
// It records function signatures, return types, and argument details to enrich
// eval inspection and debugging in the "$" command.
package main

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// FunctionMetadata describes a callable function's name, full signature, return type, and arguments.
type FunctionMetadata struct {
	Name        string   `json:"name"`
	Signature   string   `json:"signature"`
	Type        string   `json:"type"`
	ReturnType  string   `json:"returnType"`
	ParamTypes  []string `json:"paramTypes"`
	IsReflected bool     `json:"isReflected"`
}

// CallRecord captures an individual function invocation that occurred during eval execution.
type CallRecord struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	ReturnType  string   `json:"returnType"`
	ParamTypes  []string `json:"paramTypes"`
	ResultType  string   `json:"resultType"`
	ArgsSummary []string `json:"argsSummary"`
}

// EvalContextTracker tracks function invocations throughout a single eval execution run.
type EvalContextTracker struct {
	mu       sync.Mutex
	calls    []CallRecord
	lastCall *CallRecord
}

// NewEvalTracker constructs an empty tracker for an eval session.
func NewEvalTracker() *EvalContextTracker {
	return &EvalContextTracker{}
}

// RecordCall registers a function call execution.
func (t *EvalContextTracker) RecordCall(c CallRecord) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, c)
	t.lastCall = &c
}

// LastCall returns the most recent function call recorded during the run, or nil if none occurred.
func (t *EvalContextTracker) LastCall() *CallRecord {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastCall == nil {
		return nil
	}
	cp := *t.lastCall
	return &cp
}

// TagFunction attaches metadata properties (.name, .type, .returnType, .paramTypes, .signature, .toString)
// onto a Goja function object.
func TagFunction(vm *goja.Runtime, fnVal goja.Value, meta FunctionMetadata) *goja.Object {
	if vm == nil || fnVal == nil {
		return nil
	}
	obj := fnVal.ToObject(vm)
	if obj == nil {
		return nil
	}

	_ = obj.Set("name", meta.Name)
	_ = obj.Set("type", meta.Type)
	_ = obj.Set("returnType", meta.ReturnType)
	_ = obj.Set("paramTypes", meta.ParamTypes)
	_ = obj.Set("signature", meta.Signature)
	_ = obj.Set("_meta", map[string]any{
		"name":        meta.Name,
		"signature":   meta.Signature,
		"type":        meta.Type,
		"returnType":  meta.ReturnType,
		"paramTypes":  meta.ParamTypes,
		"isReflected": meta.IsReflected,
	})

	toStringFn := func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(fmt.Sprintf("[Function: %s %s -> %s]", meta.Name, meta.Signature, meta.ReturnType))
	}
	_ = obj.Set("toString", toStringFn)

	return obj
}

// BuildFunctionMetadataFromType extracts comprehensive signature metadata from a Go reflect.Type.
func BuildFunctionMetadataFromType(name string, mType reflect.Type) FunctionMetadata {
	numIn := mType.NumIn()
	var inTypes []string
	for i := range numIn {
		t := mType.In(i)
		inTypes = append(inTypes, t.String())
	}

	numOut := mType.NumOut()
	var outTypes []string
	for i := range numOut {
		t := mType.Out(i)
		outTypes = append(outTypes, t.String())
	}

	returnTypeStr := "void"
	if len(outTypes) == 1 {
		returnTypeStr = outTypes[0]
	} else if len(outTypes) > 1 {
		returnTypeStr = "(" + strings.Join(outTypes, ", ") + ")"
	}

	sig := fmt.Sprintf("(%s)", strings.Join(inTypes, ", "))
	fullType := fmt.Sprintf("func%s %s", sig, returnTypeStr)

	return FunctionMetadata{
		Name:        name,
		Signature:   sig,
		Type:        fullType,
		ReturnType:  returnTypeStr,
		ParamTypes:  inTypes,
		IsReflected: true,
	}
}

// ExtractFunctionMetadata attempts to extract attached metadata from a Goja function object or reflect.Type.
func ExtractFunctionMetadata(val goja.Value) *FunctionMetadata {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	obj, ok := val.(*goja.Object)
	if ok && obj != nil {
		if metaVal := obj.Get("_meta"); metaVal != nil && !goja.IsUndefined(metaVal) {
			if m, ok := metaVal.Export().(map[string]any); ok {
				meta := &FunctionMetadata{
					Name:       fmt.Sprintf("%v", m["name"]),
					Signature:  fmt.Sprintf("%v", m["signature"]),
					Type:       fmt.Sprintf("%v", m["type"]),
					ReturnType: fmt.Sprintf("%v", m["returnType"]),
				}
				if pts, ok := m["paramTypes"].([]any); ok {
					for _, p := range pts {
						meta.ParamTypes = append(meta.ParamTypes, fmt.Sprintf("%v", p))
					}
				}
				return meta
			}
		}

		typeVal := obj.Get("type")
		retVal := obj.Get("returnType")
		nameVal := obj.Get("name")
		if typeVal != nil && !goja.IsUndefined(typeVal) && retVal != nil && !goja.IsUndefined(retVal) {
			return &FunctionMetadata{
				Name:       nameVal.String(),
				Type:       typeVal.String(),
				ReturnType: retVal.String(),
			}
		}
	}

	// Fallback to Go reflection if it's a native Go function
	exp := val.Export()
	if exp != nil {
		rVal := reflect.ValueOf(exp)
		if rVal.Kind() == reflect.Func {
			meta := BuildFunctionMetadataFromType(rVal.Type().String(), rVal.Type())
			return &meta
		}
	}

	return nil
}

// GetValueTypeName returns a human-readable Go and JS type string for any exported or reflect value.
func GetValueTypeName(val any) string {
	if val == nil {
		return "nil"
	}
	rType := reflect.TypeOf(val)
	return rType.String()
}
