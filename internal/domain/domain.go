// Package domain contains T02 registration and transport invariants.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation failed")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrUnavailable  = errors.New("unavailable")
)

type Branch struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Scale struct {
	ID         string    `json:"id"`
	BranchID   string    `json:"branch_id"`
	ScaleID    string    `json:"scale_id"`
	Name       string    `json:"name"`
	APIKeyHash string    `json:"-"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Truck struct {
	ID              string    `json:"id"`
	Plate           string    `json:"plate"`
	TareWeightGrams int64     `json:"tare_weight_grams"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type GrainType struct {
	ID                   string    `json:"id"`
	Code                 string    `json:"code"`
	Name                 string    `json:"name"`
	PurchasePriceMinor   int64     `json:"purchase_price_minor"`
	InventoryTargetGrams int64     `json:"inventory_target_grams"`
	MarginPolicyBPS      int32     `json:"margin_policy_bps"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type TransportTransaction struct {
	ID                         string    `json:"id"`
	BranchID                   string    `json:"branch_id"`
	TruckID                    string    `json:"truck_id"`
	GrainTypeID                string    `json:"grain_type_id"`
	Status                     string    `json:"status"`
	PurchasePriceMinorSnapshot int64     `json:"purchase_price_minor_snapshot"`
	MarginPolicyBPSSnapshot    int32     `json:"margin_policy_bps_snapshot"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// ScaleReading is the normalized asynchronous ingestion event. Raw readings
// live only in Redis Streams until a later worker consumes them.
type ScaleReading struct {
	EventID     string
	ScaleID     string
	Plate       string
	WeightGrams int64
	MeasuredAt  time.Time
	ReceivedAt  time.Time
}

func NormalizeCode(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func NormalizePlate(value string) string {
	var normalized strings.Builder
	for _, character := range strings.ToUpper(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func ValidatePlate(plate string) error {
	if len(plate) != 7 {
		return errors.New("plate must contain seven letters or digits")
	}
	return nil
}
