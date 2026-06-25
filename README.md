# Experimental ADS Server (Go)

This is an experimental implementation of a **Beckhoff ADS server** written in Go.  
It provides a lightweight, in-memory ADS endpoint that can be used for testing, simulation, and development without a real PLC.

---

## 🚀 Features

- ✅ ADS TCP communication (via AMS Router)
- ✅ Supports core ADS commands:
  - Read (`CmdRead`)
  - Write (`CmdWrite`)
  - ReadWrite (`CmdReadWrite`)
  - Read Device Info
- ✅ In-memory data storage (IndexGroup → IndexOffset → bytes)
- ✅ Concurrent request handling (Task-style with bounded concurrency)
- ✅ Structured logging (`slog`) to:
  - Terminal
  - File (`logger.log`)
- ✅ Thread-safe memory access

