// eval_tools.go provides extended utilities, standard library bridges, and introspection
// helpers for the "$" owner eval runtime, including fetch, exec, fs, crypto, store, and type reflection.
package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/thruqe/whatsrook"
)

// RegisterEvalExtendedTools registers HTTP (fetch, http, axios), shell execution (exec, sh),
// file system (fs), cryptography (crypto, base64, hex), system introspection (os, process, runtime),
// store database APIs, and reflection helpers (type, returnType, methods, inspect) into the Goja runtime.
func RegisterEvalExtendedTools(vm *goja.Runtime, ctx *Context, tracker *EvalContextTracker) {
	if vm == nil {
		return
	}

	// 1. Reflection & Introspection Tools (type, returnType, methods, props, inspect, help)
	setupIntrospectionTools(vm, tracker)

	// 2. HTTP Client (fetch, http, axios)
	setupHTTPTools(vm, ctx, tracker)

	// 3. Shell / Terminal Execution (exec, sh)
	setupShellTools(vm, ctx, tracker)

	// 4. File System (fs)
	setupFSTools(vm)

	// 5. Cryptography & Encoding (crypto, base64, hex, atob, btoa)
	setupCryptoTools(vm)

	// 6. System & Process Info (os, process, runtime)
	setupSystemTools(vm, ctx)

	// 7. Store / Database Access (store)
	setupStoreTools(vm, ctx)

	// 8. General Helpers (format, sprintf)
	setupGeneralHelpers(vm, tracker)

	// 9. Networking & Socket Tools (net)
	setupNetTools(vm)

	// 10. Buffer & Binary Tools (Buffer)
	setupBufferTools(vm)

	// 11. Path Tools (path)
	setupPathTools(vm)

	// 12. URL Tools (url)
	setupURLTools(vm)

	// 13. Time Tools (time)
	setupTimeTools(vm)

	// 14. WhatsRook Package Bridge (whatsrook)
	setupWhatsrookBridge(vm)

	// 15. CommonJS Module Resolver (require)
	setupRequire(vm)
}

func setupIntrospectionTools(vm *goja.Runtime, tracker *EvalContextTracker) {
	// type(val) / typeOf(val): Returns human-readable type or function signature
	typeHelper := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("undefined")
		}
		arg := call.Arguments[0]
		if goja.IsUndefined(arg) {
			return vm.ToValue("undefined")
		}
		if goja.IsNull(arg) {
			return vm.ToValue("null")
		}

		if meta := ExtractFunctionMetadata(arg); meta != nil && meta.Type != "" {
			return vm.ToValue(meta.Type)
		}

		exported := arg.Export()
		if exported == nil {
			return vm.ToValue(arg.String())
		}

		rVal := reflect.ValueOf(exported)
		if rVal.Kind() == reflect.Func {
			meta := BuildFunctionMetadataFromType(rVal.Type().String(), rVal.Type())
			return vm.ToValue(meta.Type)
		}

		return vm.ToValue(rVal.Type().String())
	}
	tagAndSet(vm, vm.GlobalObject(), "type", FunctionMetadata{
		Name:       "type",
		Signature:  "(val any)",
		Type:       "func(val any) string",
		ReturnType: "string",
		ParamTypes: []string{"any"},
	}, typeHelper, tracker)
	_ = vm.Set("typeOf", vm.Get("type"))

	// returnType(fn) / returnTypeOf(fn): Returns return type of a function
	returnTypeHelper := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		arg := call.Arguments[0]
		if meta := ExtractFunctionMetadata(arg); meta != nil && meta.ReturnType != "" {
			return vm.ToValue(meta.ReturnType)
		}

		exported := arg.Export()
		if exported != nil {
			rVal := reflect.ValueOf(exported)
			if rVal.Kind() == reflect.Func {
				meta := BuildFunctionMetadataFromType(rVal.Type().String(), rVal.Type())
				return vm.ToValue(meta.ReturnType)
			}
		}
		return vm.ToValue("")
	}
	tagAndSet(vm, vm.GlobalObject(), "returnType", FunctionMetadata{
		Name:       "returnType",
		Signature:  "(fn function)",
		Type:       "func(fn function) string",
		ReturnType: "string",
		ParamTypes: []string{"function"},
	}, returnTypeHelper, tracker)
	_ = vm.Set("returnTypeOf", vm.Get("returnType"))

	// methods(obj): Lists all exported methods on any Go object or JS object with full signatures
	methodsHelper := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue([]string{})
		}
		arg := call.Arguments[0]
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			return vm.ToValue([]string{})
		}

		exported := arg.Export()
		var methodList []string

		if exported != nil {
			rVal := reflect.ValueOf(exported)
			rType := rVal.Type()
			for m := range rType.Methods() {
				if !m.IsExported() {
					continue
				}
				meta := BuildFunctionMetadataFromType(m.Name, m.Type)
				methodList = append(methodList, fmt.Sprintf("%s%s %s", m.Name, meta.Signature, meta.ReturnType))
			}
		}

		// Also check JS object properties
		obj := arg.ToObject(vm)
		if obj != nil {
			for _, k := range obj.Keys() {
				prop := obj.Get(k)
				if meta := ExtractFunctionMetadata(prop); meta != nil {
					sig := fmt.Sprintf("%s%s %s", k, meta.Signature, meta.ReturnType)
					if !containsString(methodList, sig) {
						methodList = append(methodList, sig)
					}
				}
			}
		}

		sort.Strings(methodList)
		return vm.ToValue(methodList)
	}
	tagAndSet(vm, vm.GlobalObject(), "methods", FunctionMetadata{
		Name:       "methods",
		Signature:  "(obj any)",
		Type:       "func(obj any) []string",
		ReturnType: "[]string",
		ParamTypes: []string{"any"},
	}, methodsHelper, tracker)

	// props(obj) / keys(obj): Lists all properties or fields of an object
	propsHelper := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue([]string{})
		}
		arg := call.Arguments[0]
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			return vm.ToValue([]string{})
		}

		obj := arg.ToObject(vm)
		if obj != nil {
			return vm.ToValue(obj.Keys())
		}
		return vm.ToValue([]string{})
	}
	tagAndSet(vm, vm.GlobalObject(), "props", FunctionMetadata{
		Name:       "props",
		Signature:  "(obj any)",
		Type:       "func(obj any) []string",
		ReturnType: "[]string",
		ParamTypes: []string{"any"},
	}, propsHelper, tracker)
	_ = vm.Set("keys", vm.Get("props"))

	// inspect(val, [depth]): Pretty-prints and details types, signatures, and values
	inspectHelper := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("undefined")
		}
		arg := call.Arguments[0]
		if goja.IsUndefined(arg) {
			return vm.ToValue("undefined")
		}
		if goja.IsNull(arg) {
			return vm.ToValue("null")
		}

		if meta := ExtractFunctionMetadata(arg); meta != nil {
			return vm.ToValue(fmt.Sprintf("[Function: %s]\nType: %s\nReturn Type: %s\nParameters: %s",
				meta.Name, meta.Type, meta.ReturnType, strings.Join(meta.ParamTypes, ", ")))
		}

		exported := arg.Export()
		rType := reflect.TypeOf(exported)
		typeStr := "<unknown>"
		if rType != nil {
			typeStr = rType.String()
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("<Type: %s>\n", typeStr))
		if jsonBytes, err := json.MarshalIndent(exported, "", "  "); err == nil {
			b.Write(jsonBytes)
		} else {
			b.WriteString(fmt.Sprintf("%+v", exported))
		}
		return vm.ToValue(b.String())
	}
	tagAndSet(vm, vm.GlobalObject(), "inspect", FunctionMetadata{
		Name:       "inspect",
		Signature:  "(val any)",
		Type:       "func(val any) string",
		ReturnType: "string",
		ParamTypes: []string{"any"},
	}, inspectHelper, tracker)

	// util.inspect compatibility object
	utilObj := vm.NewObject()
	_ = utilObj.Set("inspect", vm.Get("inspect"))
	_ = vm.Set("util", utilObj)
}

func setupHTTPTools(vm *goja.Runtime, ctx *Context, tracker *EvalContextTracker) {
	fetchHandler := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("fetch requires at least a URL argument"))
		}
		rawURL := call.Arguments[0].String()
		method := "GET"
		reqHeaders := make(map[string]string)
		var bodyReader io.Reader
		timeout := 3 * time.Second
		if strings.Contains(rawURL, "127.0.0.1") || strings.Contains(rawURL, "localhost") {
			timeout = 1 * time.Second
		}

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) && !goja.IsNull(call.Arguments[1]) {
			if optMap, ok := call.Arguments[1].Export().(map[string]any); ok {
				if m, ok := optMap["method"].(string); ok && m != "" {
					method = strings.ToUpper(m)
				}
				if hdrs, ok := optMap["headers"].(map[string]any); ok {
					for hk, hv := range hdrs {
						reqHeaders[hk] = fmt.Sprintf("%v", hv)
					}
				}
				if tVal, exists := optMap["timeout"]; exists && tVal != nil {
					var ms int64
					switch tv := tVal.(type) {
					case int64:
						ms = tv
					case int:
						ms = int64(tv)
					case float64:
						ms = int64(tv)
					case float32:
						ms = int64(tv)
					}
					if ms > 0 {
						timeout = time.Duration(ms) * time.Millisecond
					}
				}
				if bVal, exists := optMap["body"]; exists && bVal != nil {
					switch bv := bVal.(type) {
					case string:
						bodyReader = strings.NewReader(bv)
					case []byte:
						bodyReader = bytes.NewReader(bv)
					default:
						if jBytes, err := json.Marshal(bv); err == nil {
							bodyReader = bytes.NewReader(jBytes)
							if _, hasCT := reqHeaders["Content-Type"]; !hasCT {
								reqHeaders["Content-Type"] = "application/json"
							}
						}
					}
				}
			}
		}

		httpCtx, cancel := context.WithTimeout(ctx.GetSendContext(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(httpCtx, method, rawURL, bodyReader)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		for k, v := range reqHeaders {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "whatsrook-eval/1.0")
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("reading response body: %w", err)))
		}

		respObj := vm.NewObject()
		_ = respObj.Set("status", resp.StatusCode)
		_ = respObj.Set("statusText", resp.Status)
		_ = respObj.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
		_ = respObj.Set("size", len(respBytes))

		headerMap := make(map[string]string)
		for hk, hvs := range resp.Header {
			headerMap[strings.ToLower(hk)] = strings.Join(hvs, ", ")
		}
		_ = respObj.Set("headers", headerMap)

		_ = respObj.Set("text", func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(string(respBytes))
		})
		_ = respObj.Set("json", func(call goja.FunctionCall) goja.Value {
			var parsed any
			if err := json.Unmarshal(respBytes, &parsed); err != nil {
				panic(vm.NewGoError(fmt.Errorf("parsing response JSON: %w", err)))
			}
			return vm.ToValue(parsed)
		})
		_ = respObj.Set("buffer", func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(respBytes)
		})
		_ = respObj.Set("base64", func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(base64.StdEncoding.EncodeToString(respBytes))
		})
		_ = respObj.Set("toString", func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(string(respBytes))
		})

		return respObj
	}

	fetchMeta := FunctionMetadata{
		Name:        "fetch",
		Signature:   "(url string, [options object])",
		Type:        "func(url string, [options object]) Response",
		ReturnType:  "Response",
		ParamTypes:  []string{"string", "object?"},
		IsReflected: false,
	}
	tagAndSet(vm, vm.GlobalObject(), "fetch", fetchMeta, fetchHandler, tracker)

	// http and axios namespaces
	httpObj := vm.NewObject()
	_ = httpObj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("http.get requires a URL"))
		}
		var opts *goja.Object
		if len(call.Arguments) > 1 {
			opts = call.Arguments[1].ToObject(vm)
		}
		fetchArgs := []goja.Value{call.Arguments[0]}
		if opts != nil {
			fetchArgs = append(fetchArgs, opts)
		}
		return fetchHandler(goja.FunctionCall{Arguments: fetchArgs})
	})
	_ = httpObj.Set("post", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("http.post requires (url, body, [headers])"))
		}
		optMap := map[string]any{
			"method": "POST",
			"body":   call.Arguments[1].Export(),
		}
		if len(call.Arguments) > 2 {
			if hdrs, ok := call.Arguments[2].Export().(map[string]any); ok {
				optMap["headers"] = hdrs
			}
		}
		return fetchHandler(goja.FunctionCall{Arguments: []goja.Value{call.Arguments[0], vm.ToValue(optMap)}})
	})
	_ = vm.Set("http", httpObj)
	_ = vm.Set("axios", httpObj)
}

func setupShellTools(vm *goja.Runtime, ctx *Context, tracker *EvalContextTracker) {
	chatKey := ""
	if ctx != nil {
		chatKey = ctx.Chat.String()
	}

	execHandler := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("exec requires a command string"))
		}
		var cmdStr strings.Builder
		cmdStr.WriteString(call.Arguments[0].String())
		if len(call.Arguments) > 1 {
			if argsList, ok := call.Arguments[1].Export().([]any); ok {
				for _, a := range argsList {
					cmdStr.WriteString(" ")
					cmdStr.WriteString(fmt.Sprintf("%v", a))
				}
			}
		}
		workingDir := getShellWorkingDir(chatKey)

		shCtx, cancel := context.WithTimeout(ctx.GetSendContext(), 10*time.Second)
		defer cancel()

		var cmd *exec.Cmd
		if _, err := exec.LookPath("bash"); err == nil {
			cmd = exec.CommandContext(shCtx, "bash", "-c", cmdStr.String())
		} else {
			cmd = exec.CommandContext(shCtx, "sh", "-c", cmdStr.String())
		}
		cmd.Dir = workingDir

		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		err := cmd.Run()
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		} else if err != nil {
			exitCode = 1
		}

		stdoutClean := CleanShellOutput(stdoutBuf.String())
		stderrClean := CleanShellOutput(stderrBuf.String())
		combinedOutput := stdoutClean
		if stderrClean != "" {
			if combinedOutput != "" {
				combinedOutput += "\n" + stderrClean
			} else {
				combinedOutput = stderrClean
			}
		}

		resObj := vm.NewObject()
		_ = resObj.Set("stdout", stdoutClean)
		_ = resObj.Set("stderr", stderrClean)
		_ = resObj.Set("output", combinedOutput)
		_ = resObj.Set("exitCode", exitCode)
		_ = resObj.Set("ok", exitCode == 0)
		_ = resObj.Set("toString", func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(combinedOutput)
		})

		return resObj
	}

	execMeta := FunctionMetadata{
		Name:        "exec",
		Signature:   "(cmd string)",
		Type:        "func(cmd string) ExecResult",
		ReturnType:  "ExecResult",
		ParamTypes:  []string{"string"},
		IsReflected: false,
	}
	tagAndSet(vm, vm.GlobalObject(), "exec", execMeta, execHandler, tracker)
	tagAndSet(vm, vm.GlobalObject(), "sh", execMeta, execHandler, tracker)
}

func setupFSTools(vm *goja.Runtime) {
	fsObj := vm.NewObject()

	// fs.readFile(path, [encoding])
	_ = fsObj.Set("readFile", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("fs.readFile requires a path"))
		}
		p := call.Arguments[0].String()
		data, err := os.ReadFile(p)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		enc := "utf8"
		if len(call.Arguments) > 1 {
			enc = strings.ToLower(call.Arguments[1].String())
		}
		if enc == "buffer" || enc == "bytes" {
			return vm.ToValue(data)
		}
		if enc == "base64" {
			return vm.ToValue(base64.StdEncoding.EncodeToString(data))
		}
		return vm.ToValue(string(data))
	})

	// fs.writeFile(path, data)
	_ = fsObj.Set("writeFile", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("fs.writeFile requires (path, data)"))
		}
		p := call.Arguments[0].String()
		var bytesToWrite []byte
		switch v := call.Arguments[1].Export().(type) {
		case []byte:
			bytesToWrite = v
		case string:
			bytesToWrite = []byte(v)
		default:
			bytesToWrite = []byte(fmt.Sprintf("%v", v))
		}
		dir := filepath.Dir(p)
		if dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
		if err := os.WriteFile(p, bytesToWrite, 0644); err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// fs.appendFile(path, data)
	_ = fsObj.Set("appendFile", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("fs.appendFile requires (path, data)"))
		}
		p := call.Arguments[0].String()
		var bytesToWrite []byte
		switch v := call.Arguments[1].Export().(type) {
		case []byte:
			bytesToWrite = v
		case string:
			bytesToWrite = []byte(v)
		default:
			bytesToWrite = []byte(fmt.Sprintf("%v", v))
		}
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		defer f.Close()
		if _, err := f.Write(bytesToWrite); err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// fs.readdir(path)
	_ = fsObj.Set("readdir", func(call goja.FunctionCall) goja.Value {
		p := "."
		if len(call.Arguments) > 0 {
			p = call.Arguments[0].String()
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return vm.ToValue(names)
	})

	// fs.exists(path)
	_ = fsObj.Set("exists", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		p := call.Arguments[0].String()
		_, err := os.Stat(p)
		return vm.ToValue(!os.IsNotExist(err))
	})

	// fs.stat(path)
	_ = fsObj.Set("stat", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("fs.stat requires a path"))
		}
		p := call.Arguments[0].String()
		info, err := os.Stat(p)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		statObj := vm.NewObject()
		_ = statObj.Set("size", info.Size())
		_ = statObj.Set("isDir", info.IsDir())
		_ = statObj.Set("mode", info.Mode().String())
		_ = statObj.Set("modTime", info.ModTime().Format(time.RFC3339))
		return statObj
	})

	// fs.unlink(path) / fs.remove(path)
	_ = fsObj.Set("unlink", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("fs.unlink requires a path"))
		}
		p := call.Arguments[0].String()
		if err := os.RemoveAll(p); err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})
	_ = fsObj.Set("remove", fsObj.Get("unlink"))

	// fs.mkdir(path)
	_ = fsObj.Set("mkdir", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("fs.mkdir requires a path"))
		}
		p := call.Arguments[0].String()
		if err := os.MkdirAll(p, 0755); err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	_ = vm.Set("fs", fsObj)
}

func setupCryptoTools(vm *goja.Runtime) {
	cryptoObj := vm.NewObject()

	toBytes := func(val any) []byte {
		switch v := val.(type) {
		case []byte:
			return v
		case string:
			return []byte(v)
		default:
			return []byte(fmt.Sprintf("%v", v))
		}
	}

	_ = cryptoObj.Set("md5", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		h := md5.Sum(toBytes(call.Arguments[0].Export()))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	_ = cryptoObj.Set("sha1", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		h := sha1.Sum(toBytes(call.Arguments[0].Export()))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	_ = cryptoObj.Set("sha256", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		h := sha256.Sum256(toBytes(call.Arguments[0].Export()))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	_ = cryptoObj.Set("sha512", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		h := sha512.Sum512(toBytes(call.Arguments[0].Export()))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	_ = cryptoObj.Set("randomBytes", func(call goja.FunctionCall) goja.Value {
		n := 16
		if len(call.Arguments) > 0 {
			if count := call.Arguments[0].ToInteger(); count > 0 && count <= 1048576 {
				n = int(count)
			}
		}
		buf := make([]byte, n)
		_, _ = rand.Read(buf)
		return vm.ToValue(buf)
	})

	_ = cryptoObj.Set("randomHex", func(call goja.FunctionCall) goja.Value {
		n := 16
		if len(call.Arguments) > 0 {
			if count := call.Arguments[0].ToInteger(); count > 0 && count <= 1048576 {
				n = int(count)
			}
		}
		buf := make([]byte, n)
		_, _ = rand.Read(buf)
		return vm.ToValue(hex.EncodeToString(buf))
	})

	_ = cryptoObj.Set("uuid", func(call goja.FunctionCall) goja.Value {
		var u [16]byte
		_, _ = rand.Read(u[:])
		u[6] = (u[6] & 0x0f) | 0x40
		u[8] = (u[8] & 0x3f) | 0x80
		return vm.ToValue(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:]))
	})

	_ = vm.Set("crypto", cryptoObj)

	// base64 object
	b64Obj := vm.NewObject()
	_ = b64Obj.Set("encode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString(toBytes(call.Arguments[0].Export())))
	})
	_ = b64Obj.Set("decode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		s := call.Arguments[0].String()
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(string(b))
	})
	_ = vm.Set("base64", b64Obj)

	// atob and btoa
	_ = vm.Set("btoa", b64Obj.Get("encode"))
	_ = vm.Set("atob", b64Obj.Get("decode"))

	// hex object
	hexObj := vm.NewObject()
	_ = hexObj.Set("encode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(hex.EncodeToString(toBytes(call.Arguments[0].Export())))
	})
	_ = hexObj.Set("decode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		b, err := hex.DecodeString(call.Arguments[0].String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(string(b))
	})
	_ = vm.Set("hex", hexObj)
}

func setupSystemTools(vm *goja.Runtime, ctx *Context) {
	// os object
	osObj := vm.NewObject()
	_ = osObj.Set("platform", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(runtime.GOOS)
	})
	_ = osObj.Set("arch", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(runtime.GOARCH)
	})
	_ = osObj.Set("cpus", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(runtime.NumCPU())
	})
	_ = osObj.Set("hostname", func(call goja.FunctionCall) goja.Value {
		h, _ := os.Hostname()
		return vm.ToValue(h)
	})
	_ = osObj.Set("uptime", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Since(GetStartupTime()).Seconds())
	})
	_ = osObj.Set("memory", func(call goja.FunctionCall) goja.Value {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memObj := vm.NewObject()
		_ = memObj.Set("alloc", m.Alloc)
		_ = memObj.Set("totalAlloc", m.TotalAlloc)
		_ = memObj.Set("sys", m.Sys)
		_ = memObj.Set("heapAlloc", m.HeapAlloc)
		_ = memObj.Set("numGC", m.NumGC)
		return memObj
	})
	_ = vm.Set("os", osObj)

	// process object
	processObj := vm.NewObject()
	_ = processObj.Set("pid", os.Getpid())
	_ = processObj.Set("platform", runtime.GOOS)
	_ = processObj.Set("arch", runtime.GOARCH)
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	_ = processObj.Set("env", envMap)
	_ = processObj.Set("cwd", func(call goja.FunctionCall) goja.Value {
		chatKey := ""
		if ctx != nil {
			chatKey = ctx.Chat.String()
		}
		return vm.ToValue(getShellWorkingDir(chatKey))
	})
	_ = vm.Set("process", processObj)

	// runtime object
	rtObj := vm.NewObject()
	_ = rtObj.Set("goVersion", runtime.Version())
	_ = rtObj.Set("numCPU", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(runtime.NumCPU())
	})
	_ = rtObj.Set("cpus", runtime.NumCPU())
	_ = rtObj.Set("numGoroutine", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(runtime.NumGoroutine())
	})
	_ = vm.Set("runtime", rtObj)
}

func setupStoreTools(vm *goja.Runtime, ctx *Context) {
	storeObj := vm.NewObject()

	sw, _ := GetSQLStore(ctx.Client)

	// getSetting(key)
	_ = storeObj.Set("getSetting", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue("")
		}
		key := call.Arguments[0].String()
		val, err := GetSetting(ctx.GetSendContext(), sw.SQLStore, key)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(val)
	})

	// setSetting(key, val)
	_ = storeObj.Set("setSetting", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue(false)
		}
		key := call.Arguments[0].String()
		val := call.Arguments[1].String()
		err := PutSetting(ctx.GetSendContext(), sw.SQLStore, key, val)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// deleteSetting(key)
	_ = storeObj.Set("deleteSetting", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue(false)
		}
		key := call.Arguments[0].String()
		err := DeleteSetting(ctx.GetSendContext(), sw.SQLStore, key)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// getChatSetting(chat, key)
	_ = storeObj.Set("getChatSetting", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue("")
		}
		chatJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		key := call.Arguments[1].String()
		val, err := GetSetting(ctx.GetSendContext(), sw.SQLStore, key+":"+chatJID.String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(val)
	})

	// setChatSetting(chat, key, val)
	_ = storeObj.Set("setChatSetting", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue(false)
		}
		chatJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		key := call.Arguments[1].String()
		val := call.Arguments[2].String()
		err = PutSetting(ctx.GetSendContext(), sw.SQLStore, key+":"+chatJID.String(), val)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// getFilter(trigger)
	_ = storeObj.Set("getFilter", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue("")
		}
		trigger := call.Arguments[0].String()
		filterProto, err := GetFilter(ctx.GetSendContext(), sw.SQLStore, trigger)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(filterProto)
	})

	// setFilter(trigger, messageProto)
	_ = storeObj.Set("setFilter", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue(false)
		}
		trigger := call.Arguments[0].String()
		msgProto := call.Arguments[1].String()
		err := PutFilter(ctx.GetSendContext(), sw.SQLStore, trigger, msgProto)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// deleteFilter(trigger)
	_ = storeObj.Set("deleteFilter", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue(false)
		}
		trigger := call.Arguments[0].String()
		err := DeleteFilter(ctx.GetSendContext(), sw.SQLStore, trigger)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(true)
	})

	// listFilters()
	_ = storeObj.Set("listFilters", func(call goja.FunctionCall) goja.Value {
		if sw == nil || sw.SQLStore == nil {
			return vm.ToValue([]string{})
		}
		filters, err := ListFilters(ctx.GetSendContext(), sw.SQLStore)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(filters)
	})

	// getLeaderboard(groupJID)
	_ = storeObj.Set("getLeaderboard", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || sw == nil || sw.SQLStore == nil {
			return vm.ToValue([]any{})
		}
		groupJID, err := resolveJID(call.Arguments[0].Export(), ctx)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		lb, err := GetGroupLeaderboard(ctx.GetSendContext(), sw.SQLStore, groupJID.String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(lb)
	})

	// raw gives access to SQLStore directly if present
	if sw != nil && sw.SQLStore != nil {
		_ = storeObj.Set("raw", sw.SQLStore)
	}

	_ = vm.Set("store", storeObj)
}

func setupGeneralHelpers(vm *goja.Runtime, tracker *EvalContextTracker) {
	// format(fmtStr, ...args): fmt.Sprintf
	formatHandler := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		fmtStr := call.Arguments[0].String()
		var args []any
		for _, arg := range call.Arguments[1:] {
			args = append(args, arg.Export())
		}
		return vm.ToValue(fmt.Sprintf(fmtStr, args...))
	}
	tagAndSet(vm, vm.GlobalObject(), "format", FunctionMetadata{
		Name:       "format",
		Signature:  "(fmt string, ...args any)",
		Type:       "func(fmt string, ...args any) string",
		ReturnType: "string",
		ParamTypes: []string{"string", "...any"},
	}, formatHandler, tracker)
	_ = vm.Set("sprintf", vm.Get("format"))

	// delay(ms): Promise-like sleep helper
	delayHandler := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			ms := call.Arguments[0].ToInteger()
			if ms > 0 && ms <= 10000 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
		return goja.Undefined()
	}
	tagAndSet(vm, vm.GlobalObject(), "delay", FunctionMetadata{
		Name:       "delay",
		Signature:  "(ms number)",
		Type:       "func(ms number) void",
		ReturnType: "void",
		ParamTypes: []string{"number"},
	}, delayHandler, tracker)
}

func tagAndSet(vm *goja.Runtime, target *goja.Object, name string, meta FunctionMetadata, fn func(goja.FunctionCall) goja.Value, tracker *EvalContextTracker) {
	wrapped := func(call goja.FunctionCall) goja.Value {
		if tracker != nil {
			var argsSummary []string
			for _, arg := range call.Arguments {
				argsSummary = append(argsSummary, truncateString(arg.String(), 40))
			}
			tracker.RecordCall(CallRecord{
				Name:        meta.Name,
				Type:        meta.Type,
				ReturnType:  meta.ReturnType,
				ParamTypes:  meta.ParamTypes,
				ArgsSummary: argsSummary,
			})
		}
		res := fn(call)
		if tracker != nil && tracker.lastCall != nil && tracker.lastCall.Name == meta.Name {
			if res != nil && !goja.IsUndefined(res) && !goja.IsNull(res) {
				tracker.lastCall.ResultType = GetValueTypeName(res.Export())
			} else {
				tracker.lastCall.ResultType = "void"
			}
		}
		return res
	}

	fnVal := vm.ToValue(wrapped)
	TagFunction(vm, fnVal, meta)
	_ = target.Set(name, fnVal)
}

func containsString(slice []string, val string) bool {
	return slices.Contains(slice, val)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// 9. Networking & Socket Tools (net)
func setupNetTools(vm *goja.Runtime) {
	netObj := vm.NewObject()

	// net.scanPort(target, port, [timeoutMs])
	_ = netObj.Set("scanPort", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			res := vm.NewObject()
			_ = res.Set("port", 0)
			_ = res.Set("status", "unknown")
			_ = res.Set("reason", "missing arguments")
			return res
		}
		target := call.Arguments[0].String()
		port := call.Arguments[1].ToInteger()
		timeout := 200 * time.Millisecond
		if len(call.Arguments) > 2 && call.Arguments[2].ToInteger() > 0 {
			timeout = time.Duration(call.Arguments[2].ToInteger()) * time.Millisecond
		}

		address := net.JoinHostPort(target, fmt.Sprintf("%d", port))
		conn, err := net.DialTimeout("tcp", address, timeout)
		res := vm.NewObject()
		_ = res.Set("port", port)
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "connection refused") {
				_ = res.Set("status", "closed")
			} else if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
				_ = res.Set("status", "filtered")
			} else {
				_ = res.Set("status", "open/malformed")
				_ = res.Set("details", errStr)
			}
			return res
		}
		_ = conn.Close()
		_ = res.Set("status", "open")
		_ = res.Set("reason", "TCP connect succeeded")
		return res
	})

	// net.dial(network, address, [timeoutMs])
	_ = netObj.Set("dial", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.NewGoError(fmt.Errorf("net.dial requires network and address")))
		}
		network := call.Arguments[0].String()
		addr := call.Arguments[1].String()
		timeout := 3 * time.Second
		if len(call.Arguments) > 2 && call.Arguments[2].ToInteger() > 0 {
			timeout = time.Duration(call.Arguments[2].ToInteger()) * time.Millisecond
		}
		conn, err := net.DialTimeout(network, addr, timeout)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		defer conn.Close()
		res := vm.NewObject()
		_ = res.Set("localAddr", conn.LocalAddr().String())
		_ = res.Set("remoteAddr", conn.RemoteAddr().String())
		return res
	})

	// net.lookupIP(host)
	_ = netObj.Set("lookupIP", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue([]string{})
		}
		host := call.Arguments[0].String()
		ips, err := net.LookupIP(host)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		var ipStrs []string
		for _, ip := range ips {
			ipStrs = append(ipStrs, ip.String())
		}
		return vm.ToValue(ipStrs)
	})

	// net.lookupHost(host)
	_ = netObj.Set("lookupHost", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue([]string{})
		}
		host := call.Arguments[0].String()
		addrs, err := net.LookupHost(host)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(addrs)
	})

	// net.isIP(str)
	_ = netObj.Set("isIP", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(0)
		}
		ip := net.ParseIP(call.Arguments[0].String())
		if ip == nil {
			return vm.ToValue(0)
		}
		if ip.To4() != nil {
			return vm.ToValue(4)
		}
		return vm.ToValue(6)
	})

	// net.isIPv4(str)
	_ = netObj.Set("isIPv4", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		ip := net.ParseIP(call.Arguments[0].String())
		return vm.ToValue(ip != nil && ip.To4() != nil)
	})

	// net.isIPv6(str)
	_ = netObj.Set("isIPv6", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		ip := net.ParseIP(call.Arguments[0].String())
		return vm.ToValue(ip != nil && ip.To4() == nil)
	})

	// net.interfaces()
	_ = netObj.Set("interfaces", func(call goja.FunctionCall) goja.Value {
		ifaces, err := net.Interfaces()
		if err != nil {
			panic(vm.NewGoError(err))
		}
		var list []map[string]any
		for _, iface := range ifaces {
			var addrsStrs []string
			addrs, _ := iface.Addrs()
			for _, a := range addrs {
				addrsStrs = append(addrsStrs, a.String())
			}
			list = append(list, map[string]any{
				"name":         iface.Name,
				"hardwareAddr": iface.HardwareAddr.String(),
				"flags":        iface.Flags.String(),
				"addresses":    addrsStrs,
			})
		}
		return vm.ToValue(list)
	})

	_ = vm.Set("net", netObj)
}

// 10. Buffer & Binary Tools (Buffer)
func setupBufferTools(vm *goja.Runtime) {
	bufferObj := vm.NewObject()

	// Buffer.from(val, [encoding])
	_ = bufferObj.Set("from", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue([]byte{})
		}
		first := call.Arguments[0]
		switch v := first.Export().(type) {
		case []byte:
			return vm.ToValue(v)
		case string:
			enc := "utf8"
			if len(call.Arguments) > 1 {
				enc = strings.ToLower(call.Arguments[1].String())
			}
			switch enc {
			case "hex":
				b, _ := hex.DecodeString(v)
				return vm.ToValue(b)
			case "base64":
				b, _ := base64.StdEncoding.DecodeString(v)
				return vm.ToValue(b)
			default:
				return vm.ToValue([]byte(v))
			}
		case []any:
			var b []byte
			for _, item := range v {
				if n, ok := item.(int64); ok {
					b = append(b, byte(n))
				} else if n, ok := item.(float64); ok {
					b = append(b, byte(n))
				}
			}
			return vm.ToValue(b)
		default:
			return vm.ToValue([]byte(first.String()))
		}
	})

	// Buffer.alloc(size, [fill])
	_ = bufferObj.Set("alloc", func(call goja.FunctionCall) goja.Value {
		size := int64(0)
		if len(call.Arguments) > 0 {
			size = call.Arguments[0].ToInteger()
		}
		if size < 0 || size > 10*1024*1024 {
			size = 0
		}
		buf := make([]byte, size)
		if len(call.Arguments) > 1 {
			fill := byte(call.Arguments[1].ToInteger())
			for i := range buf {
				buf[i] = fill
			}
		}
		return vm.ToValue(buf)
	})

	// Buffer.isBuffer(val)
	_ = bufferObj.Set("isBuffer", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		_, ok := call.Arguments[0].Export().([]byte)
		return vm.ToValue(ok)
	})

	// Buffer.concat(list)
	_ = bufferObj.Set("concat", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue([]byte{})
		}
		var result []byte
		if list, ok := call.Arguments[0].Export().([]any); ok {
			for _, item := range list {
				if b, ok := item.([]byte); ok {
					result = append(result, b...)
				}
			}
		}
		return vm.ToValue(result)
	})

	_ = vm.Set("Buffer", bufferObj)
}

// 11. Path Tools (path)
func setupPathTools(vm *goja.Runtime) {
	pathObj := vm.NewObject()

	_ = pathObj.Set("join", func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, arg.String())
		}
		return vm.ToValue(filepath.Join(parts...))
	})

	_ = pathObj.Set("resolve", func(call goja.FunctionCall) goja.Value {
		var parts []string
		for _, arg := range call.Arguments {
			parts = append(parts, arg.String())
		}
		p := filepath.Join(parts...)
		abs, err := filepath.Abs(p)
		if err != nil {
			return vm.ToValue(p)
		}
		return vm.ToValue(abs)
	})

	_ = pathObj.Set("basename", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(filepath.Base(call.Arguments[0].String()))
	})

	_ = pathObj.Set("dirname", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(filepath.Dir(call.Arguments[0].String()))
	})

	_ = pathObj.Set("extname", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(filepath.Ext(call.Arguments[0].String()))
	})

	_ = vm.Set("path", pathObj)
}

// 12. URL Tools (url)
func setupURLTools(vm *goja.Runtime) {
	urlObj := vm.NewObject()

	_ = urlObj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(nil)
		}
		u, err := url.Parse(call.Arguments[0].String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		res := vm.NewObject()
		_ = res.Set("href", u.String())
		_ = res.Set("protocol", u.Scheme)
		_ = res.Set("host", u.Host)
		_ = res.Set("hostname", u.Hostname())
		_ = res.Set("port", u.Port())
		_ = res.Set("pathname", u.Path)
		_ = res.Set("search", u.RawQuery)
		_ = res.Set("query", u.Query())
		_ = res.Set("hash", u.Fragment)
		return res
	})

	_ = vm.Set("url", urlObj)
}

// 13. Time Tools (time)
func setupTimeTools(vm *goja.Runtime) {
	timeObj := vm.NewObject()

	_ = timeObj.Set("now", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().UnixMilli())
	})

	_ = timeObj.Set("unix", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().Unix())
	})

	_ = timeObj.Set("iso", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().UTC().Format(time.RFC3339))
	})

	_ = timeObj.Set("sleep", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			ms := call.Arguments[0].ToInteger()
			if ms > 0 && ms <= 10000 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
		return goja.Undefined()
	})

	_ = vm.Set("time", timeObj)
}

// 14. WhatsRook Package Bridge (whatsrook)
func setupWhatsrookBridge(vm *goja.Runtime) {
	wrObj := vm.NewObject()

	_ = wrObj.Set("removeEmojis", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(whatsrook.RemoveEmojis(call.Arguments[0].String()))
	})

	_ = wrObj.Set("stripFormatting", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(whatsrook.StripWhatsAppFormatting(call.Arguments[0].String()))
	})

	_ = wrObj.Set("formatTextRaw", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		return vm.ToValue(whatsrook.FormatTextResponseRaw(call.Arguments[0].String()))
	})

	_ = wrObj.Set("defaultDataDir", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(whatsrook.DefaultDataDir())
	})

	_ = vm.Set("whatsrook", wrObj)
}

// 15. CommonJS Module Resolver (require)
func setupRequire(vm *goja.Runtime) {
	_ = vm.Set("require", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.NewGoError(fmt.Errorf("require() requires module name")))
		}
		mod := strings.ToLower(strings.TrimSpace(call.Arguments[0].String()))
		switch mod {
		case "net":
			return vm.Get("net")
		case "http", "https":
			return vm.Get("http")
		case "fs":
			return vm.Get("fs")
		case "path":
			return vm.Get("path")
		case "url":
			return vm.Get("url")
		case "os":
			return vm.Get("os")
		case "child_process", "exec":
			return vm.Get("exec")
		case "crypto":
			return vm.Get("crypto")
		case "buffer":
			return vm.Get("Buffer")
		case "time":
			return vm.Get("time")
		case "store":
			return vm.Get("store")
		case "whatsrook":
			return vm.Get("whatsrook")
		case "whatsmeow":
			return vm.Get("client")
		case "util":
			utilObj := vm.NewObject()
			_ = utilObj.Set("format", vm.Get("format"))
			_ = utilObj.Set("inspect", vm.Get("inspect"))
			return utilObj
		default:
			val := vm.Get(mod)
			if val != nil && !goja.IsUndefined(val) {
				return val
			}
			panic(vm.NewGoError(fmt.Errorf("cannot find module '%s'", mod)))
		}
	})
}
