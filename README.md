# Experimental ADS Server (Go)
This is an experimental implementation of a **Beckhoff ADS server** written in Go.  
It provides a lightweight, in-memory ADS endpoint that can be used for testing, simulation, and development without a real PLC.
---

## Features
- ADS TCP communication (via AMS Router)
- Supports core ADS commands:
  - Read
  - Write
  - ReadWrite
  - Read Device Info
  - Read State
- Structured logging to:
  - Terminal
  - File
    
## Open Points
- Ads Symbol support
