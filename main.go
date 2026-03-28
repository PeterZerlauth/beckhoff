package main

import (
	"log"

	"github.com/peterzerlauth/beckhoff/internal/server"
)

func main() {
	s := server.New("My Server", 25000)

	if err := s.Start(); err != nil {
		log.Fatal(err)
	}

	log.Printf("%s running (Ctrl+C to stop)", "My Server")

	// Block forever (or replace with proper shutdown later)
	select {}
}
