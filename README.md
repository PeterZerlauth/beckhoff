# Beckhoff ADS Server for Go

A lightweight, high-performance Beckhoff ADS (Automation Device Specification) server written in Go. This project provides a clean and extensible foundation for building custom ADS devices, simulations, or middleware that communicate with TwinCAT through the ADS protocol.

---

## Features

### Core ADS Support

- ADS TCP communication (AMS Router compatible)
- AMS packet parsing and serialization
- TwinCAT-compatible request/response handling

### Supported ADS Commands

- Read Device Info
- Read
- Write
- ReadWrite
- Symbol Handle Lookup (Name → Handle)

### Symbol Table (Thread-safe)

Dynamic symbol registration with handle-based access and concurrent-safe read/write operations.

```go
// Register a symbol
server.Symbol().Add("MAIN.Counter", []byte{0, 0, 0, 0})

// Get handle
handle, err := server.Symbol().GetHandle("MAIN.Counter")

// Read / Write
server.Symbol().Write(handle, data)
data, err := server.Symbol().Read(handle)
```

### Generic Memory Backend

Supports generic ADS memory using `IndexGroup` and `IndexOffset`.

```go
server.Write(1000, 0, []byte{1, 2, 3, 4})
data, err := server.Read(1000, 0, 4)
```

### High Performance Design

- Worker pool for parallel packet processing
- Buffer pooling for reduced allocations
- Optimized locking using `sync.RWMutex`
- Serialized socket writes for safety

---

## Architecture

```
TwinCAT PLC / Client
        │
        ▼
   ADS Router (Port 48898)
        │
        ▼
   Go ADS Server
        │
 ┌──────┴────────┐
 │ Worker Pool   │
 │ (goroutines)  │
 └──────┬────────┘
        │
  ┌─────▼─────────┐
  │ Command Layer │
  └─────┬─────────┘
        │
 ┌──────▼─────────┐
 │ Symbol Table   │
 └──────┬─────────┘
        │
 ┌──────▼─────────┐
 │ Memory Backend │
 └────────────────┘
```

---

## Installation

Clone the repository:

```shell
git clone https://github.com/PeterZerlauth/beckhoff.git
cd beckhoff
```

Install dependencies:

```shell
go mod tidy
```

Build:

```shell
go build ./...
```

Run:

```shell
go run .
```

---

## Quick Example

```go
package main

import (
    "log"
    "github.com/PeterZerlauth/beckhoff/server"
)

func main() {
    srv := server.New(851)

    // Add a test symbol
    srv.Symbol().Add("MAIN.Counter", []byte{0, 0, 0, 0})

    if err := srv.Start(); err != nil {
        log.Fatal(err)
    }

    select {}
}
```

---

## TwinCAT Setup

### 1. Configure Route

In TwinCAT: **AMS Router → Add Route**

- **NetID** — your server NetID
- **IP** — your server IP address

### 2. Access Variables

Use any of the following:

- PLC ADS function blocks
- TwinCAT System Manager
- ADS APIs (C#, Python, etc.)

---

## Extension Points

The server is designed as a library, allowing fully custom behavior.

### Custom Write Logic

```go
func (s *Server) OnWrite(indexGroup, indexOffset uint32, data []byte) ads.ErrorCode {
    // handle incoming data
    return ads.NoError
}
```

### Custom Read Logic

```go
func (s *Server) OnRead(indexGroup, indexOffset, length uint32) ([]byte, ads.ErrorCode) {
    return []byte{0}, ads.NoError
}
```

### Custom ReadWrite Logic

```go
func (s *Server) OnReadWrite(indexGroup, indexOffset, readLen uint32, writeData []byte) ([]byte, ads.ErrorCode) {
    return nil, ads.NoError
}
```

---

## Thread Safety

- `sync.RWMutex` — symbol and memory access
- `sync.Mutex` — TCP write synchronization
- Worker goroutines — parallel processing
- Buffer pooling — low memory pressure

---

## Status

### Implemented

- ADS Router registration
- ADS Read / Write / ReadWrite
- Symbol table and handle system
- Generic memory backend
- Concurrent worker processing

### Planned

- Symbol upload information (TwinCAT browsing)
- Symbol enumeration (`0xF00B`)
- ADS notifications
- Device state handling
- PLC datatypes (`BOOL`, `INT`, `REAL`)

---

## Use Cases

- PLC simulation server
- ADS middleware / gateway
- Testing ADS clients
- Industrial protocol prototyping
- Custom backend systems

---

## License

MIT License

---

## Contributing

Contributions are welcome! Areas of interest:

- Symbol upload support
- ADS notification implementation
- Performance tuning
- Extended protocol coverage
