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

// Lesson represents a post-trade lesson learned entry in the trading journal.
type Lesson struct {
	ID           string   `json:"id"`
	Date         string   `json:"date"`
	Symbol       string   `json:"symbol"`
	Direction    string   `json:"direction"`              // BUY, SELL
	Outcome      string   `json:"outcome"`                // WIN, LOSS, BREAKEVEN, RUNNING
	Entry        float64  `json:"entry"`
	SL           float64  `json:"sl"`
	TPHit        *float64 `json:"tp_hit"`                 // nullable — nil if still running or SL hit
	PnLPips      *float64 `json:"pnl_pips"`               // nullable
	RRRActual    *string  `json:"rrr_actual"`             // nullable
	PlanID       string   `json:"plan_id,omitempty"`       // link back to tradeplans.json
	SetupType    string   `json:"setup_type,omitempty"`    // e.g. SMC_OB_REJECTION, FVG_FILL, LIQUIDITY_SWEEP
	Timeframe    string   `json:"timeframe,omitempty"`     // e.g. 15m, 1H, 4H
	Session      string   `json:"session,omitempty"`       // ASIA, LONDON, NEW_YORK
	Tags         []string `json:"tags,omitempty"`          // searchable tags
	WhatWorked   string   `json:"what_worked,omitempty"`   // what went right
	WhatFailed   string   `json:"what_failed,omitempty"`   // what went wrong / mistakes
	Lesson       string   `json:"lesson"`                  // the core takeaway
	EmotionNote  string   `json:"emotion_note,omitempty"`  // psychological state during the trade
	Grade        string   `json:"grade,omitempty"`         // A+, A, B+, B, C, D, F
	CreatedAt    string   `json:"created_at,omitempty"`
}

var (
	lessonMu sync.RWMutex
)

// DefaultLessonPath returns the default path to lessons.json.
func DefaultLessonPath() string {
	if _, err := os.Stat("lessons.json"); err == nil {
		return "lessons.json"
	}
	exePath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exePath)
		candidate := filepath.Join(dir, "lessons.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "lessons.json"
}

// LoadLessons loads all lessons from a JSON file.
func LoadLessons(filePath string) ([]Lesson, error) {
	lessonMu.RLock()
	defer lessonMu.RUnlock()

	if filePath == "" {
		filePath = DefaultLessonPath()
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Lesson{}, nil
		}
		return nil, fmt.Errorf("gagal membaca file lessons: %w", err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return []Lesson{}, nil
	}

	var lessons []Lesson
	if err := json.Unmarshal(data, &lessons); err != nil {
		return nil, fmt.Errorf("format JSON lessons invalid: %w", err)
	}

	return lessons, nil
}

// SaveLessons writes lessons to a JSON file.
func SaveLessons(filePath string, lessons []Lesson) error {
	lessonMu.Lock()
	defer lessonMu.Unlock()

	if filePath == "" {
		filePath = DefaultLessonPath()
	}

	data, err := json.MarshalIndent(lessons, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal serialize lessons: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("gagal menulis file lessons: %w", err)
	}

	return nil
}

// AppendLesson adds a new lesson entry. Auto-generates ID and timestamp if empty.
func AppendLesson(filePath string, lesson Lesson) error {
	if lesson.ID == "" {
		lesson.ID = fmt.Sprintf("LESSON-%s", time.Now().Format("20060102-150405"))
	}
	if lesson.Date == "" {
		lesson.Date = time.Now().Format("2006-01-02")
	}
	if lesson.CreatedAt == "" {
		lesson.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	lessons, _ := LoadLessons(filePath)
	lessons = append(lessons, lesson)

	return SaveLessons(filePath, lessons)
}

// UpsertLesson adds or updates a lesson by ID.
func UpsertLesson(filePath string, lesson Lesson) error {
	if lesson.ID == "" {
		return AppendLesson(filePath, lesson)
	}
	if lesson.CreatedAt == "" {
		lesson.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	if lesson.Date == "" {
		lesson.Date = time.Now().Format("2006-01-02")
	}

	lessons, _ := LoadLessons(filePath)
	found := false
	for i, l := range lessons {
		if l.ID == lesson.ID {
			lessons[i] = lesson
			found = true
			break
		}
	}
	if !found {
		lessons = append(lessons, lesson)
	}

	return SaveLessons(filePath, lessons)
}

// FilterLessonsByOutcome returns lessons filtered by outcome (WIN, LOSS, BREAKEVEN).
func FilterLessonsByOutcome(lessons []Lesson, outcome string) []Lesson {
	var filtered []Lesson
	outcome = strings.ToUpper(outcome)
	for _, l := range lessons {
		if strings.ToUpper(l.Outcome) == outcome {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

// FilterLessonsBySymbol returns lessons filtered by symbol.
func FilterLessonsBySymbol(lessons []Lesson, symbol string) []Lesson {
	var filtered []Lesson
	symbol = strings.ToUpper(symbol)
	for _, l := range lessons {
		if strings.Contains(strings.ToUpper(l.Symbol), symbol) {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

// FilterLessonsByTag returns lessons that contain a specific tag.
func FilterLessonsByTag(lessons []Lesson, tag string) []Lesson {
	var filtered []Lesson
	tag = strings.ToLower(tag)
	for _, l := range lessons {
		for _, t := range l.Tags {
			if strings.ToLower(t) == tag {
				filtered = append(filtered, l)
				break
			}
		}
	}
	return filtered
}

// LessonStats returns win/loss/breakeven counts and win rate.
type LessonStats struct {
	Total     int     `json:"total"`
	Wins      int     `json:"wins"`
	Losses    int     `json:"losses"`
	Breakeven int     `json:"breakeven"`
	Running   int     `json:"running"`
	WinRate   float64 `json:"win_rate"` // percentage
	AvgGrade  string  `json:"avg_grade"`
}

// ComputeLessonStats calculates aggregate statistics from a set of lessons.
func ComputeLessonStats(lessons []Lesson) LessonStats {
	stats := LessonStats{Total: len(lessons)}

	gradeScore := map[string]int{
		"A+": 10, "A": 9, "B+": 8, "B": 7, "C": 5, "D": 3, "F": 1,
	}
	totalGrade := 0
	gradeCount := 0

	for _, l := range lessons {
		switch strings.ToUpper(l.Outcome) {
		case "WIN":
			stats.Wins++
		case "LOSS":
			stats.Losses++
		case "BREAKEVEN":
			stats.Breakeven++
		case "RUNNING":
			stats.Running++
		}
		if score, ok := gradeScore[strings.ToUpper(l.Grade)]; ok {
			totalGrade += score
			gradeCount++
		}
	}

	decided := stats.Wins + stats.Losses
	if decided > 0 {
		stats.WinRate = float64(stats.Wins) / float64(decided) * 100.0
	}

	if gradeCount > 0 {
		avgScore := float64(totalGrade) / float64(gradeCount)
		switch {
		case avgScore >= 9.5:
			stats.AvgGrade = "A+"
		case avgScore >= 8.5:
			stats.AvgGrade = "A"
		case avgScore >= 7.5:
			stats.AvgGrade = "B+"
		case avgScore >= 6.5:
			stats.AvgGrade = "B"
		case avgScore >= 4.0:
			stats.AvgGrade = "C"
		case avgScore >= 2.0:
			stats.AvgGrade = "D"
		default:
			stats.AvgGrade = "F"
		}
	}

	return stats
}
