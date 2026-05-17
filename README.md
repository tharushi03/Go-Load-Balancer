<<<<<<< HEAD
# Go Custom HTTP Reverse Proxy

A lightweight custom HTTP reverse proxy written in Go.

## Features

- Round-Robin load balancing
- Least-Connections load balancing
- Backend health checks via `/healthz`
- Backend selection with concurrent request tracking
- Built on `net/http` and `net/http/httputil`

## Usage

1. Start backend servers on different ports, for example `8081` and `8082`:

```bash
go run backend_server.go -port 8081
# in another terminal
go run backend_server.go -port 8082
```

2. Run the load balancer:

```bash
go run main.go -listen :8080 -mode roundrobin -backends http://localhost:8081,http://localhost:8082
```

3. Send traffic to `http://localhost:8080`.

## Parameters

- `-listen`: proxy listen address (default `:8080`)
- `-mode`: `roundrobin` or `leastconn`
- `-backends`: comma-separated backend URLs
- `-timeout`: server idle timeout
- `-health-interval`: backend health check interval (default `10s`)

## Example

```bash
go run main.go -listen :8080 -mode leastconn -health-interval 10s -backends http://localhost:8081,http://localhost:8082
```
=======
# Go-Load-Balancer
>>>>>>> c8355bfaa7d14e8015931733be0ecd660c529261
