# Beckhoff ADS Server for Go

A lightweight Beckhoff ADS (Automation Device Specification) server implementation written in Go.

This project provides:

* ADS TCP communication
* AMS packet handling
* Symbol table with ADS handles
* Read / Write / ReadWrite ADS commands
* Generic memory-backed ADS variables
* Concurrent packet processing using worker pools
* Thread-safe symbol management

## Features

### ADS Commands

Supported ADS commands:

* Read Device Info
* Read
* Write
* ReadWrite
* Symbol Handle Lookup

### Symbol Table

The server includes a thread-safe symbol table:

```go
server.Symbol().Add("MAIN.Counter", []byte{0,0,0,0})
```

Retrieve a handle:

```go
handle, err := server.Symbol().GetHandle("MAIN.Counter")
```

Read or write through ADS handles:

```go
server.Symbol().Write(handle, data)

data, err := server.Symbol().Read(handle)
```

### Generic Memory Storage

The server automatically stores values using:

* IndexGroup
* IndexOffset

Example:

```go
server.Write(1000, 0, []byte{1,2,3,4})
```

Read:

```go
data, err := server.Read(1000, 0, 4)
```

## Installation

Clone the repository:

```bash
git clone https://github.com/PeterZerlauth/beckhoff.git
cd beckhoff
```

Install dependencies:

```bash
go mod tidy
```

Build:

```bash
go build ./...
```

## Example

```go
package main

import (
    "log"

    "github.com/PeterZerlauth/beckhoff/server"
)

func main() {

    ads := server.New(851)

    ads.Symbol().Add(
        "MAIN.Counter",
        []byte{0, 0, 0, 0},
    )

    if err := ads.Start(); err != nil {
        log.Fatal(err)
    }

    select {}
}
```

## Architecture

```text
TwinCAT Client
      │
      ▼
ADS Router (48898)
      │
      ▼
Go ADS Server
      │
 ┌────┴────┐
 │ Worker  │
 │ Pool    │
 └────┬────┘
      │
      ▼
 Symbol Table
      │
      ▼
 Memory Store
```

## Thread Safety

The implementation uses:

* sync.RWMutex for symbol access
* sync.Mutex for socket writes
* worker goroutines for concurrent packet processing
* buffer pooling for reduced allocations

## Current Status

Implemented:

* ADS Router Registration
* ADS Read
* ADS Write
* ADS ReadWrite
* ADS Symbol Handles
* Symbol Read/Write
* Generic ADS Memory

Planned:

* Symbol Upload Info
* Symbol Enumeration
* Notifications
* Device State Handling
* PLC Datatype Support

## License

MIT License
