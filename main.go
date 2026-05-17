package main

import (
    "flag"
    "fmt"
    "log"
    "net"
    "net/http"
    "net/http/httputil"
    "net/url"
    "strings"
    "sync/atomic"
    "time"
)

type Backend struct {
    URL                *url.URL
    CurrentConnections int64
    Healthy            atomic.Bool
    Proxy              *httputil.ReverseProxy
}

type LoadBalancer struct {
    backends []*Backend
    mode     string
    rr       uint64
}

func NewBackend(rawurl string) (*Backend, error) {
    parsed, err := url.Parse(rawurl)
    if err != nil {
        return nil, err
    }

    proxy := httputil.NewSingleHostReverseProxy(parsed)
    originalDirector := proxy.Director
    proxy.Director = func(req *http.Request) {
        originalDirector(req)
        req.Host = parsed.Host
        req.Header.Set("X-Forwarded-Host", req.Host)
        req.Header.Set("X-Forwarded-For", clientIP(req))
        req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
    }
    proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
        log.Printf("proxy error for backend %s: %v", parsed, err)
        rw.WriteHeader(http.StatusBadGateway)
        _, _ = rw.Write([]byte("502 bad gateway"))
    }

    backend := &Backend{URL: parsed, Proxy: proxy}
    backend.Healthy.Store(true)
    return backend, nil
}

func (b *Backend) IsHealthy() bool {
    return b.Healthy.Load()
}

func (b *Backend) SetHealthy(value bool) {
    b.Healthy.Store(value)
}

func clientIP(r *http.Request) string {
    if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
        return ip
    }

    host, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return host
}

func NewLoadBalancer(backends []*Backend, mode string) *LoadBalancer {
    return &LoadBalancer{backends: backends, mode: strings.ToLower(mode)}
}

func (lb *LoadBalancer) NextBackend() *Backend {
    switch lb.mode {
    case "leastconn", "least-connections":
        return lb.nextBackendLeastConnections()
    default:
        return lb.nextBackendRoundRobin()
    }
}

func (lb *LoadBalancer) nextBackendRoundRobin() *Backend {
    backendCount := len(lb.backends)
    if backendCount == 0 {
        return nil
    }

    for i := 0; i < backendCount; i++ {
        index := int((atomic.AddUint64(&lb.rr, 1) - 1) % uint64(backendCount))
        backend := lb.backends[index]
        if backend.IsHealthy() {
            return backend
        }
    }
    return nil
}

func (lb *LoadBalancer) nextBackendLeastConnections() *Backend {
    var selected *Backend
    minConn := int64(-1)
    for _, backend := range lb.backends {
        if !backend.IsHealthy() {
            continue
        }

        conn := atomic.LoadInt64(&backend.CurrentConnections)
        if selected == nil || conn < minConn {
            selected = backend
            minConn = conn
        }
    }
    return selected
}

func (lb *LoadBalancer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
    backend := lb.NextBackend()
    if backend == nil {
        http.Error(rw, "no backend available", http.StatusServiceUnavailable)
        return
    }

    atomic.AddInt64(&backend.CurrentConnections, 1)
    defer atomic.AddInt64(&backend.CurrentConnections, -1)

    backend.Proxy.ServeHTTP(rw, req)
}

func parseBackends(value string) ([]*Backend, error) {
    if value == "" {
        return nil, fmt.Errorf("no backends provided")
    }

    hostStrings := strings.Split(value, ",")
    backends := make([]*Backend, 0, len(hostStrings))
    for _, item := range hostStrings {
        item = strings.TrimSpace(item)
        if item == "" {
            continue
        }
        if !strings.Contains(item, "://") {
            item = "http://" + item
        }
        backend, err := NewBackend(item)
        if err != nil {
            return nil, err
        }
        backends = append(backends, backend)
    }
    return backends, nil
}

func checkBackendHealth(client *http.Client, backend *Backend) bool {
    healthURL := backend.URL.ResolveReference(&url.URL{Path: "/healthz"})
    req, err := http.NewRequest(http.MethodGet, healthURL.String(), nil)
    if err != nil {
        return false
    }
    req.Header.Set("User-Agent", "go-load-balancer-healthcheck")

    resp, err := client.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}

func startHealthChecks(backends []*Backend, interval time.Duration) {
    client := &http.Client{Timeout: 3 * time.Second}
    updateHealth := func() {
        for _, backend := range backends {
            healthy := checkBackendHealth(client, backend)
            if backend.IsHealthy() != healthy {
                backend.SetHealthy(healthy)
                log.Printf("backend %s healthy=%t", backend.URL, healthy)
            }
        }
    }

    updateHealth()
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            updateHealth()
        }
    }()
}

func main() {
    listen := flag.String("listen", ":8080", "address to listen on")
    mode := flag.String("mode", "roundrobin", "load balancer mode: roundrobin or leastconn")
    backends := flag.String("backends", "http://localhost:8081,http://localhost:8082", "comma-separated list of backend URLs")
    timeout := flag.Duration("timeout", 30*time.Second, "idle connection timeout for the reverse proxy")
    healthInterval := flag.Duration("health-interval", 10*time.Second, "backend health check interval")
    flag.Parse()

    parsedBackends, err := parseBackends(*backends)
    if err != nil {
        log.Fatalf("invalid backends: %v", err)
    }

    startHealthChecks(parsedBackends, *healthInterval)

    lb := NewLoadBalancer(parsedBackends, *mode)
    server := &http.Server{
        Addr:         *listen,
        Handler:      lb,
        IdleTimeout:  *timeout,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    log.Printf("starting load balancer on %s using %s", *listen, *mode)
    log.Printf("configured backends: %s", *backends)
    log.Printf("health check interval: %s", healthInterval.String())

    if err := server.ListenAndServe(); err != nil {
        if err != http.ErrServerClosed {
            log.Fatalf("server failed: %v", err)
        }
    }
}
