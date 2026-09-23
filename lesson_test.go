package tvspoof

import (
	"path/filepath"
	"testing"
)

func TestLessonLoadSave(t *testing.T) {
	tempDir := t.TempDir()
	lessonPath := filepath.Join(tempDir, "lessons.json")

	pips := 80.0
	rrr := "1:1.23"
	tpHit := 4333.50

	lessons := []Lesson{
		{
			ID:          "LESSON-001",
			Date:        "2026-09-23",
			Symbol:      "OANDA:XAUUSD",
			Direction:   "SELL",
			Outcome:     "WIN",
			Entry:       4341.50,
			SL:          4348.00,
			TPHit:       &tpHit,
			PnLPips:     &pips,
			RRRActual:   &rrr,
			PlanID:      "PLAN-XAUUSD-007A",
			SetupType:   "SMC_OB_REJECTION",
			Timeframe:   "15m",
			Session:     "LONDON",
			Tags:        []string{"order_block", "premium_zone"},
			WhatWorked:  "Entry tepat di OB Premium Zone.",
			Lesson:      "OB + FVG = high probability rejection.",
			EmotionNote: "Disiplin.",
			Grade:       "A",
			CreatedAt:   "2026-09-23 04:30:00",
		},
		{
			ID:        "LESSON-002",
			Date:      "2026-09-23",
			Symbol:    "OANDA:XAUUSD",
			Direction: "SELL",
			Outcome:   "LOSS",
			Entry:     4350.00,
			SL:        4355.00,
			Lesson:    "Jangan entry tanpa konfirmasi MSS di LTF.",
			Grade:     "C",
			CreatedAt: "2026-09-23 05:00:00",
		},
	}

	// Save
	if err := SaveLessons(lessonPath, lessons); err != nil {
		t.Fatalf("SaveLessons failed: %v", err)
	}

	// Load
	loaded, err := LoadLessons(lessonPath)
	if err != nil {
		t.Fatalf("LoadLessons failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 lessons, got %d", len(loaded))
	}
	if loaded[0].Symbol != "OANDA:XAUUSD" || loaded[0].Outcome != "WIN" {
		t.Errorf("unexpected lesson 0: %+v", loaded[0])
	}
	if loaded[1].Grade != "C" {
		t.Errorf("unexpected lesson 1 grade: %s", loaded[1].Grade)
	}
}

func TestAppendLesson(t *testing.T) {
	tempDir := t.TempDir()
	lessonPath := filepath.Join(tempDir, "lessons.json")

	lesson := Lesson{
		Symbol:    "OANDA:XAUUSD",
		Direction: "BUY",
		Outcome:   "WIN",
		Entry:     4300.00,
		SL:        4295.00,
		Lesson:    "Discount zone + Asia sweep = high prob long.",
		Grade:     "A",
	}

	if err := AppendLesson(lessonPath, lesson); err != nil {
		t.Fatalf("AppendLesson failed: %v", err)
	}

	loaded, _ := LoadLessons(lessonPath)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(loaded))
	}
	if loaded[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
	if loaded[0].Date == "" {
		t.Error("expected auto-generated date")
	}
}

func TestUpsertLesson(t *testing.T) {
	tempDir := t.TempDir()
	lessonPath := filepath.Join(tempDir, "lessons.json")

	// First insert
	lesson1 := Lesson{
		ID:      "LESSON-UPD-001",
		Symbol:  "OANDA:XAUUSD",
		Outcome: "RUNNING",
		Entry:   4341.50,
		Lesson:  "Runner still active.",
		Grade:   "B+",
	}
	if err := UpsertLesson(lessonPath, lesson1); err != nil {
		t.Fatalf("UpsertLesson insert failed: %v", err)
	}

	// Update
	lesson1.Outcome = "WIN"
	lesson1.Lesson = "Runner hit TP2. Patience paid off."
	lesson1.Grade = "A+"
	if err := UpsertLesson(lessonPath, lesson1); err != nil {
		t.Fatalf("UpsertLesson update failed: %v", err)
	}

	loaded, _ := LoadLessons(lessonPath)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 lesson after upsert, got %d", len(loaded))
	}
	if loaded[0].Outcome != "WIN" || loaded[0].Grade != "A+" {
		t.Errorf("upsert did not update: %+v", loaded[0])
	}
}

func TestFilterLessons(t *testing.T) {
	lessons := []Lesson{
		{ID: "L1", Symbol: "OANDA:XAUUSD", Outcome: "WIN", Tags: []string{"fvg", "ob"}},
		{ID: "L2", Symbol: "OANDA:XAUUSD", Outcome: "LOSS", Tags: []string{"fvg"}},
		{ID: "L3", Symbol: "TVC:SPX", Outcome: "WIN", Tags: []string{"gap_fill"}},
		{ID: "L4", Symbol: "OANDA:XAUUSD", Outcome: "BREAKEVEN", Tags: []string{"ob"}},
	}

	// By outcome
	wins := FilterLessonsByOutcome(lessons, "WIN")
	if len(wins) != 2 {
		t.Errorf("expected 2 wins, got %d", len(wins))
	}

	losses := FilterLessonsByOutcome(lessons, "LOSS")
	if len(losses) != 1 {
		t.Errorf("expected 1 loss, got %d", len(losses))
	}

	// By symbol
	gold := FilterLessonsBySymbol(lessons, "XAUUSD")
	if len(gold) != 3 {
		t.Errorf("expected 3 XAUUSD lessons, got %d", len(gold))
	}

	spx := FilterLessonsBySymbol(lessons, "SPX")
	if len(spx) != 1 {
		t.Errorf("expected 1 SPX lesson, got %d", len(spx))
	}

	// By tag
	fvg := FilterLessonsByTag(lessons, "fvg")
	if len(fvg) != 2 {
		t.Errorf("expected 2 fvg-tagged lessons, got %d", len(fvg))
	}

	ob := FilterLessonsByTag(lessons, "ob")
	if len(ob) != 2 {
		t.Errorf("expected 2 ob-tagged lessons, got %d", len(ob))
	}
}

func TestComputeLessonStats(t *testing.T) {
	lessons := []Lesson{
		{Outcome: "WIN", Grade: "A+"},
		{Outcome: "WIN", Grade: "A"},
		{Outcome: "LOSS", Grade: "C"},
		{Outcome: "BREAKEVEN", Grade: "B"},
		{Outcome: "RUNNING", Grade: "A"},
	}

	stats := ComputeLessonStats(lessons)

	if stats.Total != 5 {
		t.Errorf("expected total 5, got %d", stats.Total)
	}
	if stats.Wins != 2 {
		t.Errorf("expected 2 wins, got %d", stats.Wins)
	}
	if stats.Losses != 1 {
		t.Errorf("expected 1 loss, got %d", stats.Losses)
	}
	if stats.Breakeven != 1 {
		t.Errorf("expected 1 breakeven, got %d", stats.Breakeven)
	}
	if stats.Running != 1 {
		t.Errorf("expected 1 running, got %d", stats.Running)
	}

	// Win rate: 2 / (2+1) = 66.67%
	expectedWR := 66.66
	if stats.WinRate < expectedWR || stats.WinRate > 66.68 {
		t.Errorf("expected win rate ~66.67%%, got %.2f%%", stats.WinRate)
	}

	// Avg grade: (10+9+5+7+9)/5 = 8.0 → B+
	if stats.AvgGrade != "B+" {
		t.Errorf("expected avg grade B+, got %s", stats.AvgGrade)
	}
}

func TestLoadNonExistentLessons(t *testing.T) {
	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "nope.json")

	lessons, err := LoadLessons(nonExistent)
	if err != nil {
		t.Fatalf("expected nil err for non-existent file, got %v", err)
	}
	if len(lessons) != 0 {
		t.Fatalf("expected 0 lessons, got %d", len(lessons))
	}
}
