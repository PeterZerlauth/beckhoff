package main

import (
	"encoding/binary"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/router"
	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {

	router := router.NewRouter()

	if err := router.Start(); err != nil {
		log.Fatal(err)
	}

	srv := server.NewServer(25000, "My ADS Server")

	// Custom ads read
	srv.OnRead = func(indexGroup, indexOffset uint32, buf []byte) ads.ErrorCode {
		//	srv.Log().Info("Ads Read", "ig", indexGroup, "io", indexOffset, "len", len(buf))
		if indexGroup == 1000 && indexOffset == 1 {
			binary.LittleEndian.PutUint16(buf, 42)
			return ads.NoError
		}

		return ads.NoError
	}

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// wait for CTRL+C
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch

	srv.Close()

}
