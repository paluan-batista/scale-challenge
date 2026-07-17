// Package application enforces T02 business rules before persistence.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"scale-challenge/internal/domain"
)

type Store interface {
	CreateBranch(context.Context, domain.Branch) (domain.Branch, error)
	GetBranch(context.Context, string) (domain.Branch, error)
	GetActiveBranch(context.Context, string) (domain.Branch, error)
	ListBranches(context.Context) ([]domain.Branch, error)
	UpdateBranch(context.Context, domain.Branch) (domain.Branch, error)
	DeactivateBranch(context.Context, string) (domain.Branch, error)
	CreateScale(context.Context, domain.Scale) (domain.Scale, error)
	GetScale(context.Context, string) (domain.Scale, error)
	GetActiveScale(context.Context, string) (domain.Scale, error)
	ListScales(context.Context) ([]domain.Scale, error)
	UpdateScale(context.Context, domain.Scale) (domain.Scale, error)
	DeactivateScale(context.Context, string) (domain.Scale, error)
	CreateTruck(context.Context, domain.Truck) (domain.Truck, error)
	GetTruck(context.Context, string) (domain.Truck, error)
	GetActiveTruck(context.Context, string) (domain.Truck, error)
	ListTrucks(context.Context) ([]domain.Truck, error)
	UpdateTruck(context.Context, domain.Truck) (domain.Truck, error)
	DeactivateTruck(context.Context, string) (domain.Truck, error)
	CreateGrainType(context.Context, domain.GrainType) (domain.GrainType, error)
	GetGrainType(context.Context, string) (domain.GrainType, error)
	GetActiveGrainType(context.Context, string) (domain.GrainType, error)
	ListGrainTypes(context.Context) ([]domain.GrainType, error)
	UpdateGrainType(context.Context, domain.GrainType) (domain.GrainType, error)
	DeactivateGrainType(context.Context, string) (domain.GrainType, error)
	CreateTransportTransaction(context.Context, domain.TransportTransaction) (domain.TransportTransaction, error)
	GetTransportTransaction(context.Context, string) (domain.TransportTransaction, error)
	ListTransportTransactions(context.Context) ([]domain.TransportTransaction, error)
	CancelTransportTransaction(context.Context, string) (domain.TransportTransaction, error)
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

type BranchInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
type ScaleInput struct {
	BranchID string `json:"branch_id"`
	ScaleID  string `json:"scale_id"`
	Name     string `json:"name"`
	APIKey   string `json:"api_key"`
}
type TruckInput struct {
	Plate           string `json:"plate"`
	TareWeightGrams int64  `json:"tare_weight_grams"`
}
type GrainTypeInput struct {
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	PurchasePriceMinor   int64  `json:"purchase_price_minor"`
	InventoryTargetGrams int64  `json:"inventory_target_grams"`
	MarginPolicyBPS      int32  `json:"margin_policy_bps"`
}
type TransportInput struct {
	BranchID    string `json:"branch_id"`
	TruckID     string `json:"truck_id"`
	GrainTypeID string `json:"grain_type_id"`
}

func (s *Service) CreateBranch(ctx context.Context, input BranchInput) (domain.Branch, error) {
	value, err := branchValue(uuid.NewString(), input)
	if err != nil {
		return domain.Branch{}, err
	}
	return s.store.CreateBranch(ctx, value)
}
func (s *Service) GetBranch(ctx context.Context, id string) (domain.Branch, error) {
	return s.store.GetBranch(ctx, id)
}
func (s *Service) ListBranches(ctx context.Context) ([]domain.Branch, error) {
	return s.store.ListBranches(ctx)
}
func (s *Service) UpdateBranch(ctx context.Context, id string, input BranchInput) (domain.Branch, error) {
	value, err := branchValue(id, input)
	if err != nil {
		return domain.Branch{}, err
	}
	return s.store.UpdateBranch(ctx, value)
}
func (s *Service) DeactivateBranch(ctx context.Context, id string) (domain.Branch, error) {
	return s.store.DeactivateBranch(ctx, id)
}

func (s *Service) CreateScale(ctx context.Context, input ScaleInput) (domain.Scale, error) {
	if err := validateScaleInput(input); err != nil {
		return domain.Scale{}, err
	}
	if _, err := s.store.GetActiveBranch(ctx, input.BranchID); err != nil {
		return domain.Scale{}, referenceError("branch", err)
	}
	hash, err := hashKey(input.APIKey)
	if err != nil {
		return domain.Scale{}, err
	}
	return s.store.CreateScale(ctx, domain.Scale{ID: uuid.NewString(), BranchID: input.BranchID, ScaleID: domain.NormalizeCode(input.ScaleID), Name: strings.TrimSpace(input.Name), APIKeyHash: hash})
}
func (s *Service) GetScale(ctx context.Context, id string) (domain.Scale, error) {
	return s.store.GetScale(ctx, id)
}
func (s *Service) ListScales(ctx context.Context) ([]domain.Scale, error) {
	return s.store.ListScales(ctx)
}
func (s *Service) UpdateScale(ctx context.Context, id string, input ScaleInput) (domain.Scale, error) {
	if err := validateScaleInput(input); err != nil {
		return domain.Scale{}, err
	}
	existing, err := s.store.GetScale(ctx, id)
	if err != nil {
		return domain.Scale{}, err
	}
	if _, err := s.store.GetActiveBranch(ctx, input.BranchID); err != nil {
		return domain.Scale{}, referenceError("branch", err)
	}
	hash := existing.APIKeyHash
	if strings.TrimSpace(input.APIKey) != "" {
		hash, err = hashKey(input.APIKey)
		if err != nil {
			return domain.Scale{}, err
		}
	}
	return s.store.UpdateScale(ctx, domain.Scale{ID: id, BranchID: input.BranchID, ScaleID: domain.NormalizeCode(input.ScaleID), Name: strings.TrimSpace(input.Name), APIKeyHash: hash})
}
func (s *Service) DeactivateScale(ctx context.Context, id string) (domain.Scale, error) {
	return s.store.DeactivateScale(ctx, id)
}

func (s *Service) CreateTruck(ctx context.Context, input TruckInput) (domain.Truck, error) {
	value, err := truckValue(uuid.NewString(), input)
	if err != nil {
		return domain.Truck{}, err
	}
	return s.store.CreateTruck(ctx, value)
}
func (s *Service) GetTruck(ctx context.Context, id string) (domain.Truck, error) {
	return s.store.GetTruck(ctx, id)
}
func (s *Service) ListTrucks(ctx context.Context) ([]domain.Truck, error) {
	return s.store.ListTrucks(ctx)
}
func (s *Service) UpdateTruck(ctx context.Context, id string, input TruckInput) (domain.Truck, error) {
	value, err := truckValue(id, input)
	if err != nil {
		return domain.Truck{}, err
	}
	return s.store.UpdateTruck(ctx, value)
}
func (s *Service) DeactivateTruck(ctx context.Context, id string) (domain.Truck, error) {
	return s.store.DeactivateTruck(ctx, id)
}

func (s *Service) CreateGrainType(ctx context.Context, input GrainTypeInput) (domain.GrainType, error) {
	value, err := grainValue(uuid.NewString(), input)
	if err != nil {
		return domain.GrainType{}, err
	}
	return s.store.CreateGrainType(ctx, value)
}
func (s *Service) GetGrainType(ctx context.Context, id string) (domain.GrainType, error) {
	return s.store.GetGrainType(ctx, id)
}
func (s *Service) ListGrainTypes(ctx context.Context) ([]domain.GrainType, error) {
	return s.store.ListGrainTypes(ctx)
}
func (s *Service) UpdateGrainType(ctx context.Context, id string, input GrainTypeInput) (domain.GrainType, error) {
	value, err := grainValue(id, input)
	if err != nil {
		return domain.GrainType{}, err
	}
	return s.store.UpdateGrainType(ctx, value)
}
func (s *Service) DeactivateGrainType(ctx context.Context, id string) (domain.GrainType, error) {
	return s.store.DeactivateGrainType(ctx, id)
}

func (s *Service) CreateTransportTransaction(ctx context.Context, input TransportInput) (domain.TransportTransaction, error) {
	if strings.TrimSpace(input.BranchID) == "" || strings.TrimSpace(input.TruckID) == "" || strings.TrimSpace(input.GrainTypeID) == "" {
		return domain.TransportTransaction{}, validation("branch_id, truck_id, and grain_type_id are required")
	}
	if _, err := s.store.GetActiveBranch(ctx, input.BranchID); err != nil {
		return domain.TransportTransaction{}, referenceError("branch", err)
	}
	if _, err := s.store.GetActiveTruck(ctx, input.TruckID); err != nil {
		return domain.TransportTransaction{}, referenceError("truck", err)
	}
	grain, err := s.store.GetActiveGrainType(ctx, input.GrainTypeID)
	if err != nil {
		return domain.TransportTransaction{}, referenceError("grain type", err)
	}
	return s.store.CreateTransportTransaction(ctx, domain.TransportTransaction{ID: uuid.NewString(), BranchID: input.BranchID, TruckID: input.TruckID, GrainTypeID: input.GrainTypeID, PurchasePriceMinorSnapshot: grain.PurchasePriceMinor, MarginPolicyBPSSnapshot: grain.MarginPolicyBPS})
}
func (s *Service) GetTransportTransaction(ctx context.Context, id string) (domain.TransportTransaction, error) {
	return s.store.GetTransportTransaction(ctx, id)
}
func (s *Service) ListTransportTransactions(ctx context.Context) ([]domain.TransportTransaction, error) {
	return s.store.ListTransportTransactions(ctx)
}
func (s *Service) TransitionTransportTransaction(ctx context.Context, id, status string) (domain.TransportTransaction, error) {
	if domain.NormalizeCode(status) != "CANCELLED" {
		return domain.TransportTransaction{}, validation("only transition to CANCELLED is allowed before final weighing")
	}
	transaction, err := s.store.GetTransportTransaction(ctx, id)
	if err != nil {
		return domain.TransportTransaction{}, err
	}
	if transaction.Status != "OPEN" {
		return domain.TransportTransaction{}, fmt.Errorf("%w: only OPEN transport transactions can be cancelled", domain.ErrConflict)
	}
	updated, err := s.store.CancelTransportTransaction(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.TransportTransaction{}, fmt.Errorf("%w: transport transaction changed concurrently", domain.ErrConflict)
	}
	return updated, err
}

func truckValue(id string, input TruckInput) (domain.Truck, error) {
	plate := domain.NormalizePlate(input.Plate)
	if err := domain.ValidatePlate(plate); err != nil {
		return domain.Truck{}, validation(err.Error())
	}
	if input.TareWeightGrams <= 0 {
		return domain.Truck{}, validation("tare_weight_grams must be greater than zero")
	}
	return domain.Truck{ID: id, Plate: plate, TareWeightGrams: input.TareWeightGrams}, nil
}
func branchValue(id string, input BranchInput) (domain.Branch, error) {
	code, name := domain.NormalizeCode(input.Code), strings.TrimSpace(input.Name)
	if code == "" || name == "" {
		return domain.Branch{}, validation("code and name are required")
	}
	return domain.Branch{ID: id, Code: code, Name: name}, nil
}
func validateScaleInput(input ScaleInput) error {
	if strings.TrimSpace(input.BranchID) == "" || domain.NormalizeCode(input.ScaleID) == "" || strings.TrimSpace(input.Name) == "" {
		return validation("branch_id, scale_id, and name are required")
	}
	return nil
}
func grainValue(id string, input GrainTypeInput) (domain.GrainType, error) {
	if domain.NormalizeCode(input.Code) == "" || strings.TrimSpace(input.Name) == "" {
		return domain.GrainType{}, validation("code and name are required")
	}
	if input.PurchasePriceMinor < 0 || input.InventoryTargetGrams <= 0 || input.MarginPolicyBPS < 0 || input.MarginPolicyBPS > 10000 {
		return domain.GrainType{}, validation("invalid grain price, inventory target, or margin policy")
	}
	return domain.GrainType{ID: id, Code: domain.NormalizeCode(input.Code), Name: strings.TrimSpace(input.Name), PurchasePriceMinor: input.PurchasePriceMinor, InventoryTargetGrams: input.InventoryTargetGrams, MarginPolicyBPS: input.MarginPolicyBPS}, nil
}
func hashKey(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", validation("api_key is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
func validation(message string) error { return fmt.Errorf("%w: %s", domain.ErrValidation, message) }
func referenceError(reference string, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return validation("active " + reference + " is required")
	}
	return err
}
