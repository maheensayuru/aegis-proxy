package main

// This pragma directive tells the Go compiler to expose this function
// to the host environment (proxy engine).
//export check_request
func check_request() uint32 {
	// 1 = Authorized, 0 = Unauthorized
	// hardcoding the authorization for this initial test.
	return 1
}

// WASI requires a main function to act as the entry point for initialization,
func main() {}
