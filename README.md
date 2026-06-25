# Experimental ADS Server (Go)

---

This is an experimental implementation of a **Beckhoff ADS server** written in Go.

It provides a lightweight, in-memory ADS endpoint that can be used for testing, simulation, and development without requiring a real PLC.

---

## Features

* ADS TCP communication (via AMS Router)
* Supports core ADS commands:

  * Read
  * Write
  * ReadWrite
  * Read Device Info
  * Read State
* Structured logging to:

  * Terminal
  * File

---

## Open Points

* ADS Symbol support
* ADS router for standalone operation on linux
* ADS client 

---

## Example main
```go
package main

import (
	"log"

	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {

    srv := server.NewServer(25000, "My ADS Server")

    // Start server
    if err := srv.Start(); err != nil {
        log.Fatalf("Failed to start: %v", err)
    }
 
    select {} // keep running
}
```
## Ads read
```go
    // Custom ads read
    srv.OnRead = func(indexGroup, indexOffset uint32, readData []byte) ads.ErrorCode {
    	srv.Log().Info("Ads Read", "ig", indexGroup, "io", indexOffset, "len", len(readData))
        if indexGroup == 1000 && indexOffset == 1 {
            binary.LittleEndian.PutUint16(buf, 42)
            return ads.NoError
        }

        return ads.NoError
    }
```
## Ads write
```go
    // Custom ads read
    srv.OnRead = func(indexGroup, io uint32, readData []byte) ads.ErrorCode {
    	srv.Log().Info("Ads Read", "ig", indexGroup, "io", indexOffset, "len", len(readData))
        if indexGroup == 1000 && indexOffset == 1 {
            binary.LittleEndian.PutUint16(readData, 42)
            return ads.NoError
        }

        return ads.NoError
    }
```
## Ads read/write
```go
// Custom ADS ReadWrite
srv.OnReadWrite = func(indexGroup, io uint32, readData []byte, writeData []byte) ads.ErrorCode {
	srv.Log().Info("Ads ReadWrite", "ig", indexGroup, "io", indexOffset, "readLen", len(readData), "writeLen", len(writeData))

	if indexGroup == 1000 && indexOffset == 3 {
		// Example: use write input to compute response
		var input uint16
		if len(writeData) >= 2 {
			input = binary.LittleEndian.Uint16(writeData)
		}

		result := input * 2 // simple processing

		if len(readData) >= 2 {
			binary.LittleEndian.PutUint16(readData, result)
		}

		return ads.NoError
	}

	return ads.NoError
}
```

## Contributing

Contributions are welcome and appreciated — especially since this is an experimental ADS server implementation.

### Ways to contribute

- Improve protocol compatibility with TwinCAT ADS
- Add Linux/standalone AMS router
- Implement missing ADS features (symbols support, notifications)
- Extend client-side tooling for testing
- Fix edge cases in packet parsing or error handling
- Improve logging, debugging, and tracing
- Add unit and integration tests

