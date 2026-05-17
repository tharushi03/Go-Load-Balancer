package main

import (
    "flag"
    "fmt"
    "log"
    "net/http"
)

func main() {
    port := flag.Int("port", 8081, "port for the backend server")
    flag.Parse()

    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "backend server on port %d\n", *port)
    })
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })

    addr := fmt.Sprintf(":%d", *port)
    log.Printf("starting backend server on %s", addr)
    log.Fatal(http.ListenAndServe(addr, mux))
}
