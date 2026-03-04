package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Exact same endpoints as bench.httpdsl

func main() {
	mux := http.NewServeMux()

	// 1. Simple JSON
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "hello world"})
	})

	// 2. Path param (manual parsing to match httpdsl's router)
	mux.HandleFunc("/greet/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/greet/")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"greeting": "Hello, " + name + "!"})
	})

	// 3. JSON body parse + response
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		var body interface{}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"echo": body, "method": r.Method})
	})

	// 4. Computation (fibonacci)
	mux.HandleFunc("/fibonacci/", func(w http.ResponseWriter, r *http.Request) {
		nStr := strings.TrimPrefix(r.URL.Path, "/fibonacci/")
		n, _ := strconv.Atoi(nStr)
		seq := make([]int, 0, n+1)
		for i := 0; i <= n; i++ {
			seq = append(seq, fib(i))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"n": n, "value": fib(n), "sequence": seq})
	})

	// 5. Array building with loop
	mux.HandleFunc("/numbers", func(w http.ResponseWriter, r *http.Request) {
		nums := make([]int, 100)
		for i := 0; i < 100; i++ {
			nums[i] = i
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"numbers": nums})
	})

	fmt.Println("Go native server on :8001")
	http.ListenAndServe(":8001", mux)
}

func fib(n int) int {
	if n <= 1 {
		return n
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}
