package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"scale-challenge/internal/bootstrap"
)

func main() {
	if err := bootstrap.RequiredEnvironment("DATABASE_URL", "SEED_API_KEY"); err != nil {
		log.Print(err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Printf("connect PostgreSQL: %v", err)
		os.Exit(1)
	}
	defer database.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(os.Getenv("SEED_API_KEY")), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("hash seed API key: %v", err)
		os.Exit(1)
	}
	transaction, err := database.Begin(ctx)
	if err != nil {
		log.Printf("begin seed data: %v", err)
		os.Exit(1)
	}
	_, err = transaction.Exec(ctx, `INSERT INTO branches (id, code, name) VALUES ('seed-branch', 'SEED', 'Seed branch') ON CONFLICT (id) DO NOTHING`)
	if err == nil {
		_, err = transaction.Exec(ctx, `INSERT INTO scales (id, branch_id, scale_id, name, api_key_hash) VALUES ('seed-scale', 'seed-branch', 'scale-seed', 'Seed scale', $1) ON CONFLICT (id) DO NOTHING`, string(hash))
	}
	if err == nil {
		_, err = transaction.Exec(ctx, `INSERT INTO trucks (id, plate, tare_weight_grams) VALUES ('seed-truck', 'SEED123', 12000) ON CONFLICT (id) DO NOTHING`)
	}
	if err == nil {
		_, err = transaction.Exec(ctx, `INSERT INTO grain_types (id, code, name, purchase_price_minor, inventory_target_grams, margin_policy_bps) VALUES ('seed-grain', 'SOY', 'Soy', 125000, 100000000, 2000) ON CONFLICT (id) DO NOTHING`)
	}
	if err == nil {
		_, err = transaction.Exec(ctx, `INSERT INTO transport_transactions (id, branch_id, truck_id, grain_type_id, status, purchase_price_minor_snapshot, margin_policy_bps_snapshot) VALUES ('seed-transaction', 'seed-branch', 'seed-truck', 'seed-grain', 'OPEN', 125000, 2000) ON CONFLICT (id) DO NOTHING`)
	}
	if err != nil {
		_ = transaction.Rollback(ctx)
		log.Printf("seed data: %v", err)
		os.Exit(1)
	}
	if err := transaction.Commit(ctx); err != nil {
		log.Printf("commit seed data: %v", err)
		os.Exit(1)
	}
	log.Print("deterministic seed data applied")
}
