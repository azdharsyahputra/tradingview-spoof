package tvspoof

import (
	"path/filepath"
	"testing"
)

func TestTradePlanLoadSave(t *testing.T) {
	tempDir := t.TempDir()
	planPath := filepath.Join(tempDir, "tradeplans.json")

	plans := []TradePlan{
		{
			ID:        "PLAN-XAUUSD-001",
			Symbol:    "OANDA:XAUUSD",
			Asset:     "GOLD (XAUUSD)",
			Direction: "BUY",
			Status:    "RUNNING",
			Entry:     4360.00,
			EntryZone: "4358.00 - 4362.00",
			SL:        4354.50,
			TP1:       4368.50,
			TP2:       4379.00,
			RRR:       "1:3.4",
			Notes:     "Sweep Asia Low @ 4356.22, re-entry buy discount zone.",
			CreatedAt: "2026-09-21 11:15:00",
		},
		{
			ID:        "PLAN-SPX-001",
			Symbol:    "TVC:SPX",
			Asset:     "S&P 500 (SPX)",
			Direction: "BUY_LIMIT",
			Status:    "PENDING",
			Entry:     7638.00,
			EntryZone: "7636.00 - 7640.00",
			SL:        7620.00,
			TP1:       7685.00,
			TP2:       7720.00,
			RRR:       "1:2.6",
			Notes:     "4H FVG + Retest EMA 21 Support.",
			CreatedAt: "2026-09-21 11:20:00",
		},
	}

	// Save
	if err := SaveTradePlans(planPath, plans); err != nil {
		t.Fatalf("SaveTradePlans failed: %v", err)
	}

	// Load
	loaded, err := LoadTradePlans(planPath)
	if err != nil {
		t.Fatalf("LoadTradePlans failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(loaded))
	}
	if loaded[0].Symbol != "OANDA:XAUUSD" || loaded[0].Entry != 4360.00 {
		t.Errorf("unexpected plan 0: %+v", loaded[0])
	}
	if loaded[1].Symbol != "TVC:SPX" || loaded[1].Status != "PENDING" {
		t.Errorf("unexpected plan 1: %+v", loaded[1])
	}

	// Test Upsert
	newPlan := TradePlan{
		ID:        "PLAN-XAUUSD-001",
		Symbol:    "OANDA:XAUUSD",
		Status:    "RUNNING",
		Entry:     4360.00,
		SL:        4356.00, // modified SL
		Notes:     "Trailing SL",
	}
	if err := UpsertTradePlan(planPath, newPlan); err != nil {
		t.Fatalf("UpsertTradePlan failed: %v", err)
	}

	loaded2, _ := LoadTradePlans(planPath)
	if len(loaded2) != 2 {
		t.Fatalf("expected 2 plans after update, got %d", len(loaded2))
	}
	if loaded2[0].SL != 4356.00 || loaded2[0].Notes != "Trailing SL" {
		t.Errorf("upsert did not update in place: %+v", loaded2[0])
	}
}

func TestLoadNonExistentTradePlans(t *testing.T) {
	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "non_existent.json")

	plans, err := LoadTradePlans(nonExistent)
	if err != nil {
		t.Fatalf("expected nil err for non-existent file, got %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected 0 plans, got %d", len(plans))
	}
}
