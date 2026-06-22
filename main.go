package main

import (
	"log"

	"github.com/peterzerlauth/beckhoff/server"
)

func main() {
	srv := server.New(25000)

	// ✅ add BOOL symbol
	srv.Symbol().Add("Main.bTest", []byte{1})  // true
	srv.Symbol().Add("Main.bTest2", []byte{1}) // true
	srv.Symbol().Add("Main.bValue", []byte{1}) // true

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start ADS server: %v", err)
	}

	select {}
}
