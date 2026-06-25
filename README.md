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

---

## Open Points
```go
package main

import (
	"log"

	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {

    srv := server.NewServer(25000, "My ADS Server")

    // Custom ads read
    srv.OnRead = func(ig, io uint32, buf []byte) ads.ErrorCode {
    	srv.Log().Info("Ads Read", "ig", ig, "io", io, "len", len(buf))
        if ig == 1000 && io == 1 {
            binary.LittleEndian.PutUint16(buf, 42)
            return ads.NoError
        }

        return ads.NoError
    }

    // Start server
    if err := srv.Start(); err != nil {
        log.Fatalf("Failed to start: %v", err)
    }
 
    select {} // keep running
}
```

