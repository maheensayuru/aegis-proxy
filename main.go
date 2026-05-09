package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	wasmBytes, err := os.ReadFile("plugins/auth.wasm")
	if err != nil {
		fmt.Printf("Failed to read Wasm file: %v\n", err)
		return
	}

	// --- NEW: THE REACTOR CONFIGURATION ---
	// 1. WithStdout: Pipes the Wasm fmt.Printf directly to our terminal.
	// 2. WithStartFunctions(): Passing nothing overrides the default WASI behavior,
	//    preventing the _start function from closing our module.
	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithStartFunctions()

	// Instantiate using the new config instead of the default.
	mod, err := r.InstantiateWithConfig(ctx, wasmBytes, config)
	if err != nil {
		fmt.Printf("Failed to instantiate Wasm: %v\n", err)
		return
	}

	allocateMemory := mod.ExportedFunction("allocate_memory")
	if allocateMemory == nil {
		fmt.Println("CRITICAL FATAL: 'allocate_memory' is missing.")
		return
	}

	checkRequest := mod.ExportedFunction("check_request")
	if checkRequest == nil {
		fmt.Println("CRITICAL FATAL: 'check_request' is missing.")
		return
	}

	target := "http://google.com"
	origin, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(origin)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathLen := uint64(len(path))

		allocResults, err := allocateMemory.Call(ctx, pathLen)
		if err != nil {
			// --- NEW: INSTRUMENTATION ---
			// If Wasm crashes, log the exact memory fault to our console.
			fmt.Printf("[Host Engine] WASM Allocation Trapped: %v\n", err)
			http.Error(w, "Middleware Allocation Error", http.StatusInternalServerError)
			return
		}
		pathPtr := allocResults[0]

		mod.Memory().Write(uint32(pathPtr), []byte(path))

		results, err := checkRequest.Call(ctx, pathPtr, pathLen)
		if err != nil {
			fmt.Printf("[Host Engine] WASM Execution Trapped: %v\n", err)
			http.Error(w, "Middleware Execution Error", http.StatusInternalServerError)
			return
		}

		if len(results) == 0 || results[0] == 0 {
			http.Error(w, "Unauthorized by Aegis Wasm Middleware", http.StatusForbidden)
			return
		}

		fmt.Printf("Wasm Middleware: Authorized path %s. Forwarding...\n", path)
		proxy.ServeHTTP(w, r)
	})

	fmt.Println("Aegis Engine (Reactor Mode) listening on :8080...")
	http.ListenAndServe(":8080", nil)
}
