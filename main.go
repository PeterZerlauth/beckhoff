package main

import (
	"log"

	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {
	server := server.New(25000)

	// ✅ add BOOL symbol
	server.Symbol().Add("Main.bTest", []byte{1})  // true
	server.Symbol().Add("Main.bTest2", []byte{1}) // true
	server.Symbol().Add("Main.bValue", []byte{1}) // true

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start ADS server: %v", err)
	}

	select {}
}
