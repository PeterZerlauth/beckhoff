package main

import (
	"log"

	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {
	
	server := server.NewServer(25000, "Ads server")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start ADS server: %v", err)
	}

	select {}
}
