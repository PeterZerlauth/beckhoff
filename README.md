# Beckhoff ADS Server for Go

[![Go Report Card](https://goreportcard.com/badge/github.com/PeterZerlauth/beckhoff)](https://goreportcard.com/report/github.com/PeterZerlauth/beckhoff)
[![Go Version](https://img.shields.io/github/go-mod/go-version/PeterZerlauth/beckhoff)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A lightweight, high-performance Beckhoff ADS (Automation Device Specification) server written in Go. This project provides a clean and extensible foundation for building custom ADS devices, simulators, and middleware solutions for industrial automation.

---

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Example](#quick-example)
- [TwinCAT Setup](#twincatsetup)
- [API Reference](#api-reference)
- [Extension Points](#extension-points)
- [Thread Safety](#thread-safety)
- [Status](#status)
- [Use Cases](#use-cases)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

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

### High Performance Design

- Worker pool for parallel packet processing
- Buffer pooling for reduced allocations
- Optimized locking using `sync.RWMutex`
- Serialized socket writes for safety

---

## Prerequisites

- **Go 1.18** or higher
- **Beckhoff TwinCAT** installation (for full integration testing, optional for development)
- TCP port availability (default: 851)

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
    "encoding/binary"
    "log"

    "github.com/PeterZerlauth/beckhoff/server"
    "github.com/PeterZerlauth/beckhoff/ads"
)

func main() {
    srv := server.New(851)

    // Add a static symbol
    srv.Symbol().Add("MAIN.Counter", []byte{0, 0, 0, 0})

    // ✅ Override OnRead
    srv.OnRead = func(indexGroup, indexOffset, length uint32) ([]byte, ads.ErrorCode) {
        // Example: return a dynamic counter value
        // (simulating a PLC variable that changes)

        value := uint32(12345) // your dynamic value

        buf := make([]byte, 4)
        binary.LittleEndian.PutUint32(buf, value)

        // Respect requested length
        if int(length) < len(buf) {
            return buf[:length], ads.NoError
        }

        return buf, ads.NoError
    }

    if err := srv.Start(); err != nil {
        log.Fatal(err)
    }

    defer func() {
        if err := srv.Stop(); err != nil {
            log.Printf("error stopping server: %v", err)
        }
    }()

    log.Println("ADS Server running on port 851...")
    select {}
}
```

---

## TwinCAT Setup

### 1. Configure Route

In TwinCAT: **AMS Router → Add Route**

- **NetID** — your server NetID
- **IP** — your server IP address
- **Port** — 851 (default)

### 2. Access Variables

Use any of the following:

- PLC ADS function blocks
- ADS APIs (C#, Python, etc.)
- Visual Studio ADS debugging extensions

---

## API Reference

### Server Type

```go
type Server struct {
    // Port for the ADS server (default: 851)
    Port int
    
    // Custom read handler
    OnRead func(indexGroup, indexOffset, length uint32) ([]byte, ads.ErrorCode)
    
    // Custom write handler
    OnWrite func(indexGroup, indexOffset uint32, data []byte) ads.ErrorCode
    
    // Custom read/write handler
    OnReadWrite func(indexGroup, indexOffset, readLen uint32, writeData []byte) ([]byte, ads.ErrorCode)
}
```

### Core Methods

- `New(port int) *Server` — Create a new server instance
- `Start() error` — Start the ADS server
- `Stop() error` — Gracefully stop the server
- `Symbol() *SymbolTable` — Get the symbol table for registration

### Symbol Table

- `Add(name string, data []byte) error` — Register a symbol
- `Get(name string) ([]byte, bool)` — Retrieve symbol data
- `GetHandle(name string) (uint32, error)` — Get handle for symbol name
- `Remove(name string)` — Unregister a symbol

---

## Extension Points

The server is designed as a library, allowing fully custom behavior.

### Custom Write Logic

```go
srv.OnWrite = func(indexGroup, indexOffset uint32, data []byte) ads.ErrorCode {
    // Handle incoming data
    log.Printf("Write: group=%d, offset=%d, len=%d", indexGroup, indexOffset, len(data))
    return ads.NoError
}
```

### Custom Read Logic

```go
srv.OnRead = func(indexGroup, indexOffset, length uint32) ([]byte, ads.ErrorCode) {
    // Return custom data based on address
    log.Printf("Read: group=%d, offset=%d, len=%d", indexGroup, indexOffset, length)
    return []byte{0}, ads.NoError
}
```

### Custom ReadWrite Logic

```go
srv.OnReadWrite = func(indexGroup, indexOffset, readLen uint32, writeData []byte) ([]byte, ads.ErrorCode) {
    // Handle atomic read+write operations
    log.Printf("ReadWrite: group=%d, offset=%d", indexGroup, indexOffset)
    return nil, ads.NoError
}
```

---

## Thread Safety

- `sync.RWMutex` — symbol and memory access
- `sync.Mutex` — TCP write synchronization
- Worker goroutines — parallel packet processing
- Buffer pooling — low memory pressure

All handler functions are called concurrently and must be goroutine-safe.

---

## Status

### Implemented

- ✅ ADS Router registration
- ✅ ADS Read / Write / ReadWrite
- ✅ Symbol table and handle system
- ✅ Generic memory backend
- ✅ Concurrent worker processing

### Planned

- 📋 Symbol upload information (TwinCAT browsing)
- 📋 Symbol enumeration (`0xF00B`)
- 📋 ADS notifications
- 📋 Device state handling
- 📋 PLC datatypes (`BOOL`, `INT`, `REAL`)

---

## Use Cases

- PLC simulation server
- ADS middleware / gateway
- Testing ADS clients
- Industrial protocol prototyping
- Custom backend systems
- Device emulation for development

---

## Testing

Run the test suite:

```shell
go test ./...
```

Run with verbose output:

```shell
go test -v ./...
```

Run tests with coverage:

```shell
go test -cover ./...
```

---

## Troubleshooting

### Port Already in Use

**Error:** `listen tcp :851: bind: address already in use`

**Solution:**
- Change the port in your code: `server.New(8851)`
- Or kill the process using port 851:
  ```shell
  lsof -i :851
  kill -9 <PID>
  ```

### TwinCAT Route Not Found

**Error:** Route not registered in AMS Router

**Solution:**
- Ensure the server is running: `go run .`
- Verify the NetID matches your configuration
- Check firewall settings allowing port 851
- Restart AMS Router after adding the route

### Symbol Handle Not Found

**Error:** Symbol lookup returns error code

**Solution:**
- Ensure symbols are added before client requests: `srv.Symbol().Add("MAIN.Counter", data)`
- Verify symbol names match exactly (case-sensitive)
- Check that the symbol table is initialized: `srv.Symbol()`

### Connection Refused

**Error:** `dial tcp: connection refused`

**Solution:**
- Verify the server is running on the correct IP and port
- Check network connectivity between client and server
- Ensure no firewall is blocking the connection

---

## Contributing

Contributions are welcome! Areas of interest:

- Symbol upload support
- ADS notification implementation
- Performance tuning
- Extended protocol coverage
- Bug reports and fixes
- Documentation improvements

Please submit pull requests or open issues for discussion.

---

## License

MIT License

See [LICENSE](LICENSE) file for details.
