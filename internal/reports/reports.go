// Package reports exposes indexed, aggregate-only reads over finalized
// weighings. It deliberately has no mutation methods.
package reports

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"scale-challenge/internal/domain"
)

type Filter struct {
	BranchID    string
	GrainTypeID string
	Start       *time.Time
	End         *time.Time
}

type Row struct {
	BranchID                string `json:"branch_id"`
	BranchCode              string `json:"branch_code"`
	GrainTypeID             string `json:"grain_type_id"`
	GrainTypeCode           string `json:"grain_type_code"`
	NetWeightGrams          int64  `json:"net_weight_grams"`
	TotalCostMinor          int64  `json:"total_cost_minor"`
	AveragePurchasePrice    int64  `json:"average_purchase_price_minor"`
	AverageAppliedMarginBPS int32  `json:"average_applied_margin_bps"`
	CompletedTransportCount int64  `json:"completed_transport_count"`
	purchasePriceSum        int64
	appliedMarginSum        int64
}

type Result struct {
	Totals Row   `json:"totals"`
	Items  []Row `json:"items"`
}

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Query is one grouped SQL aggregate, not a per-row lookup. The transport
// status predicate keeps the report bounded strictly to completed business
// state even if a malformed historical weighing were present.
func (s *Service) Query(ctx context.Context, filter Filter) (Result, error) {
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return Result{}, fmt.Errorf("%w: start must be before or equal to end", domain.ErrValidation)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT w.branch_id, b.code, w.grain_type_id, g.code,
		       COALESCE(SUM(w.net_weight_grams), 0)::BIGINT,
		       COALESCE(SUM(w.load_cost_minor), 0)::BIGINT,
		       COALESCE(ROUND(AVG(w.purchase_price_minor_snapshot)), 0)::BIGINT,
		       COALESCE(ROUND(AVG(w.applied_margin_bps)), 0)::INTEGER,
		       COALESCE(SUM(w.purchase_price_minor_snapshot), 0)::BIGINT,
		       COALESCE(SUM(w.applied_margin_bps), 0)::BIGINT,
		       COUNT(*)::BIGINT
		FROM weighings w
		JOIN transport_transactions t ON t.id = w.transport_transaction_id AND t.status = 'WEIGHED'
		JOIN branches b ON b.id = w.branch_id
		JOIN grain_types g ON g.id = w.grain_type_id
		WHERE w.stage = 'FINAL'
		  AND (NULLIF($1, '') IS NULL OR w.branch_id = $1)
		  AND (NULLIF($2, '') IS NULL OR w.grain_type_id = $2)
		  AND ($3::TIMESTAMPTZ IS NULL OR w.weighed_at >= $3)
		  AND ($4::TIMESTAMPTZ IS NULL OR w.weighed_at <= $4)
		GROUP BY w.branch_id, b.code, w.grain_type_id, g.code
		ORDER BY b.code, g.code`, strings.TrimSpace(filter.BranchID), strings.TrimSpace(filter.GrainTypeID), filter.Start, filter.End)
	if err != nil {
		return Result{}, fmt.Errorf("query finalized weighing report: %w", err)
	}
	defer rows.Close()
	result := Result{Items: make([]Row, 0)}
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.BranchID, &row.BranchCode, &row.GrainTypeID, &row.GrainTypeCode, &row.NetWeightGrams, &row.TotalCostMinor, &row.AveragePurchasePrice, &row.AverageAppliedMarginBPS, &row.purchasePriceSum, &row.appliedMarginSum, &row.CompletedTransportCount); err != nil {
			return Result{}, fmt.Errorf("scan finalized weighing report: %w", err)
		}
		result.Items = append(result.Items, row)
		result.Totals.NetWeightGrams += row.NetWeightGrams
		result.Totals.TotalCostMinor += row.TotalCostMinor
		result.Totals.CompletedTransportCount += row.CompletedTransportCount
	}
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("iterate finalized weighing report: %w", err)
	}
	if result.Totals.CompletedTransportCount > 0 {
		var priceSum, marginSum int64
		for _, row := range result.Items {
			priceSum += row.purchasePriceSum
			marginSum += row.appliedMarginSum
		}
		result.Totals.AveragePurchasePrice = (priceSum + result.Totals.CompletedTransportCount/2) / result.Totals.CompletedTransportCount
		result.Totals.AverageAppliedMarginBPS = int32((marginSum + result.Totals.CompletedTransportCount/2) / result.Totals.CompletedTransportCount)
	}
	return result, nil
}
