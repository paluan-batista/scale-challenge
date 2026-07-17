package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"scale-challenge/internal/bootstrap"
	"scale-challenge/internal/migrations"
)

func main() {
	if err := bootstrap.RequiredEnvironment("DATABASE_URL"); err != nil {
		log.Print(err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err == nil {
		defer database.Close()
		err = migrations.Apply(ctx, database)
	}
	if err != nil {
		log.Printf("apply migrations: %v", err)
		os.Exit(1)
	}
	log.Print("migrations applied")
}
