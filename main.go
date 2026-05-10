package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time" //Required for debouncing

	"github.com/fsnotify/fsnotify"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// The Aegis Engine State
type AegisState struct {
	mu             sync.RWMutex
	activeMod      api.Module
	allocateMemory api.Function
	checkRequest   api.Function
}

var state AegisState
var runtime wazero.Runtime

func main() {
	ctx := context.Background()
	runtime = wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	// 1. Initial Load of the Wasm Module
	err := loadWasm(ctx, "plugins/auth.wasm")
	if err != nil {
		log.Fatalf("Fatal initial Wasm load: %v\n", err)
	}

	// 2. Start the background Hot-Swap Watcher
	go watchPlugins(ctx)

	target := "http://google.com"
	origin, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(origin)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathLen := uint64(len(path))

		// ACQUIRE READ LOCK: Allow thousands of concurrent requests, block hot-swaps
		state.mu.RLock()

		// Extract current pointers securely
		allocFn := state.allocateMemory
		checkFn := state.checkRequest
		mod := state.activeMod

		state.mu.RUnlock() // Release lock immediately after grabbing pointers

		// Execute Wasm Logic
		allocResults, err := allocFn.Call(ctx, pathLen)
		if err != nil {
			http.Error(w, "Middleware Allocation Error", http.StatusInternalServerError)
			return
		}
		pathPtr := allocResults[0]

		mod.Memory().Write(uint32(pathPtr), []byte(path))

		results, err := checkFn.Call(ctx, pathPtr, pathLen)
		if err != nil {
			http.Error(w, "Middleware Execution Error", http.StatusInternalServerError)
			return
		}

		if len(results) == 0 || results[0] == 0 {
			http.Error(w, "Unauthorized by Aegis Wasm Middleware", http.StatusForbidden)
			return
		}

		proxy.ServeHTTP(w, r)
	})

	fmt.Println("Aegis Gateway [Reactor Mode + Hot-Swap Engine] listening on :8080...")
	http.ListenAndServe(":8080", nil)
}

// loadWasm reads the binary, instantiates it, and safely swaps the global state
func loadWasm(ctx context.Context, filepath string) error {
	wasmBytes, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	// NEW: Inject a unique timestamp into the module name to prevent collisions.
	uniqueName := fmt.Sprintf("auth-module-%d", time.Now().UnixNano())

	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithStartFunctions().
		WithName(uniqueName) // The namespace overrider

	mod, err := runtime.InstantiateWithConfig(ctx, wasmBytes, config)
	if err != nil {
		return fmt.Errorf("instantiation failed: %w", err)
	}

	allocFn := mod.ExportedFunction("allocate_memory")
	checkFn := mod.ExportedFunction("check_request")

	if allocFn == nil || checkFn == nil {
		mod.Close(ctx)
		return fmt.Errorf("missing required exports in new wasm binary")
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeMod != nil {
		state.activeMod.Close(ctx)
	}

	state.activeMod = mod
	state.allocateMemory = allocFn
	state.checkRequest = checkFn

	fmt.Println(">> [HOT-SWAP] Successfully loaded new Wasm logic into memory.")
	return nil
}

// watchPlugins monitors the filesystem and triggers a reload on binary write
func watchPlugins(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	err = watcher.Add("plugins")
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write && event.Name == "plugins\\auth.wasm" {
				fmt.Println(">> [WATCHER] File write detected. Debouncing compiler lock...")

				// NEW: The Debounce. Give the TinyGo compiler 500ms to finish writing
				// the binary to disk before we attempt to read it.
				time.Sleep(500 * time.Millisecond)

				err := loadWasm(ctx, event.Name)
				if err != nil {
					fmt.Printf(">> [WATCHER] Hot-swap failed, maintaining previous state: %v\n", err)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Watcher error:", err)
		}
	}
}
