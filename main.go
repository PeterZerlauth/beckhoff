package main

import (
	"encoding/binary"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/peterzerlauth/beckhoff/server"
)

func main() {
	server := server.New("My Server", 25000)
	server.SetLogLevel("debug")

	if err := server.SetLogFile("log.txt"); err != nil {
		log.Fatal("failed to open log file:", err)
	}

	// SetRead is called when the ADS client requests data from the server.
	// ig = IndexGroup, io = IndexOffset — together they identify the target variable.
	// buf is pre-allocated to the size the client requested; fill it and return (n, 0).
	// Return (0, adsErr) to send an ADS error code back to the client.
	server.SetRead(func(ig, io uint32, buf []byte) (int, uint32) {
		if len(buf) < 4 {
			return 0, 0x06 // ADSERR_DEVICE_INVALIDSIZE
		}

		value := uint32(42)
		binary.LittleEndian.PutUint32(buf, value)
		return 4, 0
	})

	// SetWrite is called when the ADS client sends data to the server.
	// ig = IndexGroup, io = IndexOffset — together they identify the target variable.
	// data contains the raw bytes the client wrote; return 0 on success or an ADS error code.
	server.SetWrite(func(ig, io uint32, data []byte) uint32 {
		if len(data) >= 4 {
			_ = binary.LittleEndian.Uint32(data)
		}
		return 0
	})

	if err := server.Start(); err != nil {
		log.Fatal("failed to start server:", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	server.Stop()
}
