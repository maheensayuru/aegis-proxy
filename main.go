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
	// Initialize runtime and WASI environment.
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Load and instantiate the compiled Wasm binary.
	wasmBytes, err := os.ReadFile("plugins/auth.wasm")
	if err != nil {
		fmt.Printf("Failed to read Wasm file: %v\n", err)
		return
	}

	mod, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		fmt.Printf("Failed to instantiate Wasm: %v\n", err)
		return
	}

	// Extract both required functions from the guest module.
	allocateMemory := mod.ExportedFunction("allocate_memory")
	checkRequest := mod.ExportedFunction("check_request")

	target := "http://google.com"
	origin, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(origin)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathLen := uint64(len(path))

		// Step 1: Request memory allocation from the Wasm module.
		allocResults, err := allocateMemory.Call(ctx, pathLen)
		if err != nil {
			http.Error(w, "Middleware Allocation Error", http.StatusInternalServerError)
			return
		}
		pathPtr := allocResults[0] // The memory address pointer

		// Step 2: Write the HTTP path string directly into Wasm linear memory.
		mod.Memory().Write(uint32(pathPtr), []byte(path))

		// Step 3: Execute the Wasm logic, passing the pointer and length.
		results, err := checkRequest.Call(ctx, pathPtr, pathLen)

		// Evaluate the return code.
		if err != nil || len(results) == 0 || results[0] == 0 {
			http.Error(w, "Unauthorized by Aegis Wasm Middleware", http.StatusForbidden)
			return
		}

		fmt.Printf("Wasm Middleware: Authorized path %s. Forwarding...\n", path)
		proxy.ServeHTTP(w, r)
	})

	fmt.Println("Aegis Engine (Memory-Injected) listening on :8080...")
	http.ListenAndServe(":8080", nil)
}
