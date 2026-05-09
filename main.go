package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// The destination where we want to send traffic (the backend)
	target := "http://google.com" 
	origin, _ := url.Parse(target)

	proxy := httputil.NewSingleHostReverseProxy(origin)

	// This function handles EVERY request
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("Intercepted request: %s %s\n", r.Method, r.URL.Path)
		
		// TODO: This is where we will eventually trigger the Wasm logic
		
		proxy.ServeHTTP(w, r)
	})

	fmt.Println("Aegis Engine listening on :8080...")
	http.ListenAndServe(":8080", nil)
}