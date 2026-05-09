package main

import (
	"fmt"
	"unsafe"
)

//export allocate_memory
func allocate_memory(size uint32) uint32 {
	buf := make([]byte, size)
	// Cast the memory address directly to a 32-bit integer to prevent offset corruption
	return uint32(uintptr(unsafe.Pointer(&buf[0])))
}

//export check_request
func check_request(pathPtr uint32, pathLen uint32) uint32 {
	// Reconstruct the string from the raw memory address
	pathBytes := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(pathPtr))), pathLen)
	path := string(pathBytes)

	// INSTRUMENTATION: Print exactly what Wasm is reading from memory
	fmt.Printf("[Wasm Sandbox] Reading memory at offset %d: '%s'\n", pathPtr, path)

	if path == "/admin" {
		return 0 // Unauthorized
	}

	return 1 // Authorized
}

func main() {}
