package tvspoof

import (
	"encoding/json"
	"os"
	"strings"
)

// DefaultCalibrationPath is where cmd/backtest writes the empirical
// win-rate table that AnalyzeDesk loads.
const DefaultCalibrationPath = "calibration.json"

// minCalibrationSamples is the smallest bucket trusted over the baseline.
const minCalibrationSamples = 30

// Calibration maps conviction-score edge buckets to out-of-sample outcome
// frequencies measured by the walk-forward backtester.
type Calibration struct {
	GeneratedAt string              `json:"generated_at"`
	Symbols     []SymbolCalibration `json:"symbols"`
}

// SymbolCalibration holds the buckets for one symbol.
type SymbolCalibration struct {
	Symbol   string              `json:"symbol"`
	From     string              `json:"from"`
	To       string              `json:"to"`
	Buckets  []CalibrationBucket `json:"buckets"`
	Settings map[string]float64  `json:"settings,omitempty"`
}

// CalibrationBucket is the outcome frequency for |score-50| in [MinEdge, MaxEdge].
type CalibrationBucket struct {
	Direction   string  `json:"direction"` // "LONG" or "SHORT"
	MinEdge     int     `json:"min_edge"`
	MaxEdge     int     `json:"max_edge"`
	Samples     int     `json:"samples"`
	TP1Rate     float64 `json:"tp1_rate"`    // P(TP1 before SL)
	RunnerRate  float64 `json:"runner_rate"` // P(TP2 before BEP | TP1)
	ExpectancyR float64 `json:"expectancy_r"`
}

// Lookup returns the matching bucket with enough samples, or nil.
func (c *Calibration) Lookup(symbol, direction string, edge int) *CalibrationBucket {
	if c == nil {
		return nil
	}
	for i := range c.Symbols {
		sc := &c.Symbols[i]
		if !strings.EqualFold(sc.Symbol, symbol) {
			continue
		}
		for j := range sc.Buckets {
			b := &sc.Buckets[j]
			if b.Direction == direction && edge >= b.MinEdge && edge <= b.MaxEdge && b.Samples >= minCalibrationSamples {
				return b
			}
		}
	}
	return nil
}

// Upsert replaces (or adds) the calibration for sc.Symbol.
func (c *Calibration) Upsert(sc SymbolCalibration) {
	for i := range c.Symbols {
		if strings.EqualFold(c.Symbols[i].Symbol, sc.Symbol) {
			c.Symbols[i] = sc
			return
		}
	}
	c.Symbols = append(c.Symbols, sc)
}

// LoadCalibration reads a calibration file; a missing or invalid file yields nil.
func LoadCalibration(path string) *Calibration {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c Calibration
	if json.Unmarshal(data, &c) != nil {
		return nil
	}
	return &c
}

// SaveCalibration writes the calibration file.
func SaveCalibration(path string, c *Calibration) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
