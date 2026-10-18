package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/spedious/spedious/testdata/mockserver"
)

func main() {
	port := flag.Int("port", 8089, "Port to listen on")
	delay := flag.Duration("delay", 0, "Default response delay")
	flag.Parse()

	srv := mockserver.NewServer(mockserver.ServerConfig{
		DefaultDelay: *delay,
		RequireAuth:  false,
		AuthToken:    "secret-jwt-mock-token",
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Spedious Mock Server listening on http://localhost%s (delay: %s)\n", addr, *delay)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
