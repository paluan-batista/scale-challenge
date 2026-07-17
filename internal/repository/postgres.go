// Package repository adapts generated SQLC queries to the T02 domain.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"scale-challenge/internal/database/sqlc"
	"scale-challenge/internal/domain"
)

type Postgres struct{ queries *sqlc.Queries }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{queries: sqlc.New(pool)} }

func (r *Postgres) CreateBranch(ctx context.Context, value domain.Branch) (domain.Branch, error) {
	row, err := r.queries.CreateBranch(ctx, sqlc.CreateBranchParams{ID: value.ID, Code: value.Code, Name: value.Name})
	return branch(row), classify(err)
}
func (r *Postgres) GetBranch(ctx context.Context, id string) (domain.Branch, error) {
	row, err := r.queries.GetBranch(ctx, id)
	return branch(row), classify(err)
}
func (r *Postgres) GetActiveBranch(ctx context.Context, id string) (domain.Branch, error) {
	row, err := r.queries.GetActiveBranch(ctx, id)
	return branch(row), classify(err)
}
func (r *Postgres) ListBranches(ctx context.Context) ([]domain.Branch, error) {
	rows, err := r.queries.ListBranches(ctx)
	return branches(rows), classify(err)
}
func (r *Postgres) UpdateBranch(ctx context.Context, value domain.Branch) (domain.Branch, error) {
	row, err := r.queries.UpdateBranch(ctx, sqlc.UpdateBranchParams{ID: value.ID, Code: value.Code, Name: value.Name})
	return branch(row), classify(err)
}
func (r *Postgres) DeactivateBranch(ctx context.Context, id string) (domain.Branch, error) {
	row, err := r.queries.DeactivateBranch(ctx, id)
	return branch(row), classify(err)
}

func (r *Postgres) CreateScale(ctx context.Context, value domain.Scale) (domain.Scale, error) {
	row, err := r.queries.CreateScale(ctx, sqlc.CreateScaleParams{ID: value.ID, BranchID: value.BranchID, ScaleID: value.ScaleID, Name: value.Name, ApiKeyHash: value.APIKeyHash})
	return scale(row), classify(err)
}
func (r *Postgres) GetScale(ctx context.Context, id string) (domain.Scale, error) {
	row, err := r.queries.GetScale(ctx, id)
	return scale(row), classify(err)
}
func (r *Postgres) GetActiveScale(ctx context.Context, id string) (domain.Scale, error) {
	row, err := r.queries.GetActiveScale(ctx, id)
	return scale(row), classify(err)
}
func (r *Postgres) ListScales(ctx context.Context) ([]domain.Scale, error) {
	rows, err := r.queries.ListScales(ctx)
	return scales(rows), classify(err)
}
func (r *Postgres) UpdateScale(ctx context.Context, value domain.Scale) (domain.Scale, error) {
	row, err := r.queries.UpdateScale(ctx, sqlc.UpdateScaleParams{ID: value.ID, BranchID: value.BranchID, ScaleID: value.ScaleID, Name: value.Name, ApiKeyHash: value.APIKeyHash})
	return scale(row), classify(err)
}
func (r *Postgres) DeactivateScale(ctx context.Context, id string) (domain.Scale, error) {
	row, err := r.queries.DeactivateScale(ctx, id)
	return scale(row), classify(err)
}

func (r *Postgres) CreateTruck(ctx context.Context, value domain.Truck) (domain.Truck, error) {
	row, err := r.queries.CreateTruck(ctx, sqlc.CreateTruckParams{ID: value.ID, Plate: value.Plate, TareWeightGrams: value.TareWeightGrams})
	return truck(row), classify(err)
}
func (r *Postgres) GetTruck(ctx context.Context, id string) (domain.Truck, error) {
	row, err := r.queries.GetTruck(ctx, id)
	return truck(row), classify(err)
}
func (r *Postgres) GetActiveTruck(ctx context.Context, id string) (domain.Truck, error) {
	row, err := r.queries.GetActiveTruck(ctx, id)
	return truck(row), classify(err)
}
func (r *Postgres) ListTrucks(ctx context.Context) ([]domain.Truck, error) {
	rows, err := r.queries.ListTrucks(ctx)
	return trucks(rows), classify(err)
}
func (r *Postgres) UpdateTruck(ctx context.Context, value domain.Truck) (domain.Truck, error) {
	row, err := r.queries.UpdateTruck(ctx, sqlc.UpdateTruckParams{ID: value.ID, Plate: value.Plate, TareWeightGrams: value.TareWeightGrams})
	return truck(row), classify(err)
}
func (r *Postgres) DeactivateTruck(ctx context.Context, id string) (domain.Truck, error) {
	row, err := r.queries.DeactivateTruck(ctx, id)
	return truck(row), classify(err)
}

func (r *Postgres) CreateGrainType(ctx context.Context, value domain.GrainType) (domain.GrainType, error) {
	row, err := r.queries.CreateGrainType(ctx, sqlc.CreateGrainTypeParams{ID: value.ID, Code: value.Code, Name: value.Name, PurchasePriceMinor: value.PurchasePriceMinor, InventoryTargetGrams: value.InventoryTargetGrams, MarginPolicyBps: value.MarginPolicyBPS})
	return grain(row), classify(err)
}
func (r *Postgres) GetGrainType(ctx context.Context, id string) (domain.GrainType, error) {
	row, err := r.queries.GetGrainType(ctx, id)
	return grain(row), classify(err)
}
func (r *Postgres) GetActiveGrainType(ctx context.Context, id string) (domain.GrainType, error) {
	row, err := r.queries.GetActiveGrainType(ctx, id)
	return grain(row), classify(err)
}
func (r *Postgres) ListGrainTypes(ctx context.Context) ([]domain.GrainType, error) {
	rows, err := r.queries.ListGrainTypes(ctx)
	return grains(rows), classify(err)
}
func (r *Postgres) UpdateGrainType(ctx context.Context, value domain.GrainType) (domain.GrainType, error) {
	row, err := r.queries.UpdateGrainType(ctx, sqlc.UpdateGrainTypeParams{ID: value.ID, Code: value.Code, Name: value.Name, PurchasePriceMinor: value.PurchasePriceMinor, InventoryTargetGrams: value.InventoryTargetGrams, MarginPolicyBps: value.MarginPolicyBPS})
	return grain(row), classify(err)
}
func (r *Postgres) DeactivateGrainType(ctx context.Context, id string) (domain.GrainType, error) {
	row, err := r.queries.DeactivateGrainType(ctx, id)
	return grain(row), classify(err)
}

func (r *Postgres) CreateTransportTransaction(ctx context.Context, value domain.TransportTransaction) (domain.TransportTransaction, error) {
	row, err := r.queries.CreateTransportTransaction(ctx, sqlc.CreateTransportTransactionParams{ID: value.ID, BranchID: value.BranchID, TruckID: value.TruckID, GrainTypeID: value.GrainTypeID, PurchasePriceMinorSnapshot: value.PurchasePriceMinorSnapshot, MarginPolicyBpsSnapshot: value.MarginPolicyBPSSnapshot})
	return transport(row), classify(err)
}
func (r *Postgres) GetTransportTransaction(ctx context.Context, id string) (domain.TransportTransaction, error) {
	row, err := r.queries.GetTransportTransaction(ctx, id)
	return transport(row), classify(err)
}
func (r *Postgres) ListTransportTransactions(ctx context.Context) ([]domain.TransportTransaction, error) {
	rows, err := r.queries.ListTransportTransactions(ctx)
	return transports(rows), classify(err)
}
func (r *Postgres) CancelTransportTransaction(ctx context.Context, id string) (domain.TransportTransaction, error) {
	row, err := r.queries.CancelTransportTransaction(ctx, id)
	return transport(row), classify(err)
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && (databaseError.Code == "23505" || databaseError.Code == "23503" || databaseError.Code == "23514") {
		return domain.ErrConflict
	}
	return err
}
func timestamp(value pgtype.Timestamptz) time.Time { return value.Time.UTC() }
func branch(value sqlc.Branch) domain.Branch {
	return domain.Branch{ID: value.ID, Code: value.Code, Name: value.Name, Active: value.Active, CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt)}
}
func branches(values []sqlc.Branch) []domain.Branch {
	result := make([]domain.Branch, len(values))
	for i, v := range values {
		result[i] = branch(v)
	}
	return result
}
func scale(value sqlc.Scale) domain.Scale {
	return domain.Scale{ID: value.ID, BranchID: value.BranchID, ScaleID: value.ScaleID, Name: value.Name, APIKeyHash: value.ApiKeyHash, Active: value.Active, CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt)}
}
func scales(values []sqlc.Scale) []domain.Scale {
	result := make([]domain.Scale, len(values))
	for i, v := range values {
		result[i] = scale(v)
	}
	return result
}
func truck(value sqlc.Truck) domain.Truck {
	return domain.Truck{ID: value.ID, Plate: value.Plate, TareWeightGrams: value.TareWeightGrams, Active: value.Active, CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt)}
}
func trucks(values []sqlc.Truck) []domain.Truck {
	result := make([]domain.Truck, len(values))
	for i, v := range values {
		result[i] = truck(v)
	}
	return result
}
func grain(value sqlc.GrainType) domain.GrainType {
	return domain.GrainType{ID: value.ID, Code: value.Code, Name: value.Name, PurchasePriceMinor: value.PurchasePriceMinor, InventoryTargetGrams: value.InventoryTargetGrams, MarginPolicyBPS: value.MarginPolicyBps, Active: value.Active, CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt)}
}
func grains(values []sqlc.GrainType) []domain.GrainType {
	result := make([]domain.GrainType, len(values))
	for i, v := range values {
		result[i] = grain(v)
	}
	return result
}
func transport(value sqlc.TransportTransaction) domain.TransportTransaction {
	return domain.TransportTransaction{ID: value.ID, BranchID: value.BranchID, TruckID: value.TruckID, GrainTypeID: value.GrainTypeID, Status: value.Status, PurchasePriceMinorSnapshot: value.PurchasePriceMinorSnapshot, MarginPolicyBPSSnapshot: value.MarginPolicyBpsSnapshot, CreatedAt: timestamp(value.CreatedAt), UpdatedAt: timestamp(value.UpdatedAt)}
}
func transports(values []sqlc.TransportTransaction) []domain.TransportTransaction {
	result := make([]domain.TransportTransaction, len(values))
	for i, v := range values {
		result[i] = transport(v)
	}
	return result
}
