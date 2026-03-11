package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"title-parser/pkg/serve"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	configDir := flag.String("config", "configs", "config directory")
	flag.Parse()

	logger := log.New(os.Stderr, "[serve] ", log.LstdFlags)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := serve.NewServer(*configDir, os.Stdin, os.Stdout, logger)
	if err := s.Run(ctx); err != nil {
		logger.Printf("serve stopped with error: %v", err)
		os.Exit(1)
	}
	logger.Println("serve stopped")
}
