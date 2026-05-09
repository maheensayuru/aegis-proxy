package main

import (
	"unsafe"
)

//export allocate_memory
func allocate_memory(size uint32) *byte {
	// Allocate a byte slice of the requested size.
	buf := make([]byte, size)
	// Return the memory address of the first byte.
	return &buf[0]
}

//export check_request
func check_request(pathPtr uint32, pathLen uint32) uint32 {
	// Reconstruct the string from the raw memory pointer and length.
	pathBytes := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(pathPtr))), pathLen)
	path := string(pathBytes)

	// Evaluate the requested path.
	if path == "/admin" {
		return 0 // Unauthorized
	}

	return 1 // Authorized
}

func main() {}
