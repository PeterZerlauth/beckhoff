package main

import (
	"log"

	"github.com/PeterZerlauth/beckhoff/server"
)

func main() {

	// logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	// 	Level: slog.LevelDebug,
	// }))

	server := server.NewServer(25000, "Ads server")

	// // ✅ add BOOL symbol
	// server.Symbol().Add("Main.bTest", []byte{1})  // true
	// server.Symbol().Add("Main.bTest2", []byte{1}) // true
	// server.Symbol().Add("Main.bValue", []byte{1}) // true

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start ADS server: %v", err)
	}

	select {}
}
