package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {

	srv := server.NewServer(25000, "Ads server")

	srv.OnRead = func(ig, io uint32, buf []byte) ads.ErrorCode {
		fmt.Println("Custom OnRead:", ig, io)

		if ig == 1000 && io == 1 {
			binary.LittleEndian.PutUint16(buf, 1234)
			return ads.NoError
		}

		return ads.NoError
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start ADS server: %v", err)
	}

	select {}
}
