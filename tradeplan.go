package tvspoof

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TradePlan represents an institutional quantitative trade plan.
type TradePlan struct {
	ID        string  `json:"id"`
	Symbol    string  `json:"symbol"`
	Asset     string  `json:"asset"`
	Direction string  `json:"direction"` // BUY, SELL, BUY_LIMIT, SELL_LIMIT
	Status    string  `json:"status"`    // RUNNING, PENDING, WATCHING, HIT_TP, HIT_SL, CLOSED
	Entry     float64 `json:"entry"`
	EntryZone string  `json:"entry_zone,omitempty"`
	SL        float64 `json:"sl"`
	TP1       float64 `json:"tp1"`
	TP2       float64 `json:"tp2,omitempty"`
	RRR       string  `json:"rrr,omitempty"`
	Notes     string  `json:"notes,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
}

var (
	tradePlanMu sync.RWMutex
)

// DefaultTradePlanPath returns the default path to tradeplans.json.
func DefaultTradePlanPath() string {
	// Check current directory
	if _, err := os.Stat("tradeplans.json"); err == nil {
		return "tradeplans.json"
	}
	// Check root of repo if running from subdir
	exePath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exePath)
		candidate := filepath.Join(dir, "tradeplans.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "tradeplans.json"
}

// LoadTradePlans loads all trade plans from a JSON file.
func LoadTradePlans(filePath string) ([]TradePlan, error) {
	tradePlanMu.RLock()
	defer tradePlanMu.RUnlock()

	if filePath == "" {
		filePath = DefaultTradePlanPath()
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TradePlan{}, nil
		}
		return nil, fmt.Errorf("gagal membaca file tradeplans: %w", err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return []TradePlan{}, nil
	}

	var plans []TradePlan
	if err := json.Unmarshal(data, &plans); err != nil {
		return nil, fmt.Errorf("format JSON tradeplans invalid: %w", err)
	}

	return plans, nil
}

// SaveTradePlans writes trade plans to a JSON file.
func SaveTradePlans(filePath string, plans []TradePlan) error {
	tradePlanMu.Lock()
	defer tradePlanMu.Unlock()

	if filePath == "" {
		filePath = DefaultTradePlanPath()
	}

	data, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal serialize tradeplans: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("gagal menulis file tradeplans: %w", err)
	}

	return nil
}

// UpsertTradePlan adds or updates a trade plan by ID.
func UpsertTradePlan(filePath string, plan TradePlan) error {
	if plan.ID == "" {
		plan.ID = fmt.Sprintf("PLAN-%s-%s", strings.ReplaceAll(NormalizeDeskSymbol(plan.Symbol), ":", "-"), time.Now().Format("01021504"))
	}
	if plan.CreatedAt == "" {
		plan.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	plans, _ := LoadTradePlans(filePath)
	found := false
	for i, p := range plans {
		if p.ID == plan.ID {
			plans[i] = plan
			found = true
			break
		}
	}
	if !found {
		plans = append(plans, plan)
	}

	return SaveTradePlans(filePath, plans)
}
