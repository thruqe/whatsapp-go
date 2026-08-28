package external

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	utils "whatsrook/src"
	Logger "whatsrook/src/logger"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var (
	wasmRuntime     wazero.Runtime
	wasmRuntimeOnce sync.Once
	wasmModuleCache = make(map[string]wazero.CompiledModule)
	wasmCacheMu     sync.RWMutex
)

// getWASMRuntime returns the singleton wazero runtime with compilation caching enabled.
func getWASMRuntime(ctx context.Context) wazero.Runtime {
	wasmRuntimeOnce.Do(func() {
		config := wazero.NewRuntimeConfig().WithCompilationCache(wazero.NewCompilationCache())
		wasmRuntime = wazero.NewRuntimeWithConfig(ctx, config)
		wasi_snapshot_preview1.MustInstantiate(ctx, wasmRuntime)
	})
	return wasmRuntime
}

// isWASMFile checks if the given file has a .wasm extension or starts with the WebAssembly binary magic header (\x00asm).
func isWASMFile(path string) bool {
	if strings.HasSuffix(strings.ToLower(path), ".wasm") {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 4)
	n, err := f.Read(header)
	if err != nil || n < 4 {
		return false
	}
	return bytes.Equal(header, []byte{0x00, 0x61, 0x73, 0x6d})
}

// runWASMModule executes a WebAssembly/WASI plugin within the embedded wazero sandbox.
func (d *Dispatcher) runWASMModule(plugCtx *utils.PluginContext, path, name string, request Request) {
	liveCtx, liveCancel := context.WithTimeout(context.Background(), d.liveTimeout)
	defer liveCancel()

	plugCtx.Ctx = liveCtx
	sessionKey := d.sessionKey(request.Chat, name)

	// Start animated external loader
	loader := startLoader(plugCtx, name, 350*time.Millisecond)
	defer loader.Delete()

	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		loader.Delete()
		Logger.Error("failed to read WASM plugin binary", "plugin", name, "err", err)
		_ = plugCtx.Replyf("Failed to read WASM plugin %q: %v", name, err)
		return
	}

	r := getWASMRuntime(liveCtx)

	// Check or compile WASM module
	wasmCacheMu.RLock()
	compiled, exists := wasmModuleCache[path]
	wasmCacheMu.RUnlock()

	if !exists {
		var compileErr error
		compiled, compileErr = r.CompileModule(liveCtx, wasmBytes)
		if compileErr != nil {
			loader.Delete()
			Logger.Error("failed to compile WASM module", "plugin", name, "err", compileErr)
			_ = plugCtx.Replyf("Failed to compile WASM module %q: %v", name, err)
			return
		}
		wasmCacheMu.Lock()
		wasmModuleCache[path] = compiled
		wasmCacheMu.Unlock()
	}

	// Register live session for cancel handling
	d.registerSession(sessionKey, liveCancel, name)
	defer d.unregisterSession(sessionKey)

	// Prepare Request JSON line as stdin
	reqBytes, _ := json.Marshal(request)
	reqReader := bytes.NewReader(append(reqBytes, '\n'))

	stdoutReader, stdoutWriter := io.Pipe()

	args := append([]string{name}, request.Args...)
	modConfig := wazero.NewModuleConfig().
		WithArgs(args...).
		WithStdin(reqReader).
		WithStdout(stdoutWriter).
		WithStderr(io.Discard).
		WithEnv("WHATSROOK_PLUGIN", name).
		WithEnv("WHATSROOK_PREFIX", request.Prefix).
		WithEnv("WHATSROOK_BOT_NAME", request.BotName)

	// Launch WASM module in a background goroutine so we can stream stdout concurrently
	execDone := make(chan error, 1)
	go func() {
		defer stdoutWriter.Close()
		mod, instErr := r.InstantiateModule(liveCtx, compiled, modConfig)
		if instErr != nil {
			execDone <- instErr
			return
		}
		defer mod.Close(liveCtx)
		execDone <- nil
	}()

	scanner := bufio.NewScanner(stdoutReader)
	var firstLine string
	var isStreaming bool
	var readFirst bool

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !readFirst {
			readFirst = true
			firstLine = line
			if strings.HasPrefix(trimmed, "{\"action\"") {
				isStreaming = true
			} else {
				isStreaming = false
				break
			}
		}

		if isStreaming {
			// WASM plugins can emit action frames (reply, edit, react, delete, media, poll, done)
			if err := d.handleActionFrame(plugCtx, loader, nil, line); err != nil {
				Logger.Debug("WASM streaming action finished", "plugin", name, "err", err)
				break
			}
		}
	}

	if !isStreaming && readFirst {
		var sb strings.Builder
		sb.WriteString(firstLine)
		for scanner.Scan() {
			sb.WriteByte('\n')
			sb.WriteString(scanner.Text())
		}
		response := strings.TrimSpace(sb.String())
		if response != "" {
			_ = loader.Done(response)
		} else {
			loader.Delete()
		}
	}

	<-execDone
}
