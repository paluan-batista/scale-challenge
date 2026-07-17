package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"scale-challenge/internal/bootstrap"
)

func main() {
	if err := bootstrap.RequiredEnvironment("DATABASE_URL", "REDIS_ADDR"); err != nil {
		log.Print(err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Print("worker bootstrap started; stream processing is scheduled for T04-T06")
	<-ctx.Done()
	log.Print("worker bootstrap stopped")
}
