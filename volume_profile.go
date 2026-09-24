package tvspoof

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SessionType defines the trading session.
type SessionType string

const (
	SessionAsia    SessionType = "ASIAN"
	SessionLondon  SessionType = "LONDON"
	SessionNewYork SessionType = "NEW_YORK"
	SessionCustom  SessionType = "SESSION"
)

// SessionVolumeProfile contains volume profile metrics for a specific session.
type SessionVolumeProfile struct {
	Session     SessionType `json:"session"`
	Date        string      `json:"date"`
	StartTime   time.Time   `json:"start_time"`
	EndTime     time.Time   `json:"end_time"`
	POC         float64     `json:"poc"`
	VAH         float64     `json:"vah"`
	VAL         float64     `json:"val"`
	TotalVolume float64     `json:"total_volume"`
	BuyVolume   float64     `json:"buy_volume"`
	SellVolume  float64     `json:"sell_volume"`
	Delta       float64     `json:"delta"`
	DeltaRatio  float64     `json:"delta_ratio"` // Buy Vol / Total Vol (0.0 to 1.0)
	HVNs        []float64   `json:"hvns,omitempty"`
	LVNs        []float64   `json:"lvns,omitempty"`
	IsTested    bool        `json:"is_tested"`
}

// NakedPOC represents a Point of Control from a prior session that has never been tested.
type NakedPOC struct {
	Price       float64     `json:"price"`
	Session     SessionType `json:"session"`
	Date        string      `json:"date"`
	TotalVolume float64     `json:"total_volume"`
	Distance    float64     `json:"distance"`     // Difference from current price
	DistancePct float64     `json:"distance_pct"` // % distance
	Status      string      `json:"status"`       // "VIRGIN / UNTESTED" or "TESTED"
	Significance string     `json:"significance"` // "HIGH", "MEDIUM", "LOW"
}

// VolumeAnalysisReport aggregates multi-session profiling, nPOCs, and node maps.
type VolumeAnalysisReport struct {
	Symbol           string                 `json:"symbol"`
	CurrentPrice     float64                `json:"current_price"`
	Precision        int                    `json:"precision"`
	Timestamp        time.Time              `json:"timestamp"`
	CurrentSession   SessionType            `json:"current_session"`
	SessionProfiles  []SessionVolumeProfile `json:"session_profiles"`
	NakedPOCs        []NakedPOC             `json:"naked_pocs"`
	ActiveHVNs       []float64              `json:"active_hvns"`
	ActiveLVNs       []float64              `json:"active_lvns"`
	CompositePOC     float64                `json:"composite_poc"`
	CompositeVAH     float64                `json:"composite_vah"`
	CompositeVAL     float64                `json:"composite_val"`
	TotalVolume      float64                `json:"total_volume"`
}

// AnalyzeVolumeProfile performs complete multi-session volume profiling and nPOC identification.
func AnalyzeVolumeProfile(bars15M []Bar, currentPrice float64, precision int, symbol string) VolumeAnalysisReport {
	report := VolumeAnalysisReport{
		Symbol:       symbol,
		CurrentPrice: currentPrice,
		Precision:    precision,
		Timestamp:    time.Now().UTC(),
	}

	if len(bars15M) == 0 {
		return report
	}

	// 1. Calculate overall composite profile
	compositeVP := calculateVolumeProfile(bars15M, precision)
	report.CompositePOC = compositeVP.POC
	report.CompositeVAH = compositeVP.VAH
	report.CompositeVAL = compositeVP.VAL
	report.TotalVolume = compositeVP.TotalVolume

	// Extract HVNs and LVNs from composite profile
	hvns, lvns := extractNodesFromBins(compositeVP.Bins, precision)
	report.ActiveHVNs = hvns
	report.ActiveLVNs = lvns

	// 2. Segment bars into trading sessions
	sessions := segmentBarsIntoSessions(bars15M, precision)
	report.SessionProfiles = sessions

	// Determine current session from last bar time
	lastBarTime := time.Unix(bars15M[len(bars15M)-1].Time, 0).UTC()
	hour := lastBarTime.Hour()
	if hour >= 0 && hour < 8 {
		report.CurrentSession = SessionAsia
	} else if hour >= 7 && hour < 16 {
		report.CurrentSession = SessionLondon
	} else {
		report.CurrentSession = SessionNewYork
	}

	// 3. Find Naked POCs (Prior session POCs that have not been breached)
	report.NakedPOCs = findNakedPOCs(sessions, currentPrice, precision)

	return report
}

// segmentBarsIntoSessions splits bars into Asian, London, and NY session blocks.
func segmentBarsIntoSessions(bars []Bar, precision int) []SessionVolumeProfile {
	var profiles []SessionVolumeProfile
	if len(bars) == 0 {
		return profiles
	}

	type sessionBucket struct {
		session SessionType
		date    string
		bars    []Bar
	}

	var buckets []sessionBucket
	var currentBucket *sessionBucket

	for _, b := range bars {
		t := time.Unix(b.Time, 0).UTC()
		hour := t.Hour()
		dateStr := t.Format("2006-01-02")

		var st SessionType
		if hour >= 0 && hour < 8 {
			st = SessionAsia
		} else if hour >= 8 && hour < 14 {
			st = SessionLondon
		} else {
			st = SessionNewYork
		}

		if currentBucket == nil || currentBucket.session != st || currentBucket.date != dateStr {
			buckets = append(buckets, sessionBucket{
				session: st,
				date:    dateStr,
				bars:    []Bar{b},
			})
			currentBucket = &buckets[len(buckets)-1]
		} else {
			currentBucket.bars = append(currentBucket.bars, b)
		}
	}

	// Compute profile for each session bucket
	for _, b := range buckets {
		if len(b.bars) == 0 {
			continue
		}
		vp := calculateVolumeProfile(b.bars, precision)
		totBuy, totSell := calculateSessionBuySellVolume(b.bars)
		delta := totBuy - totSell
		ratio := 0.5
		if (totBuy + totSell) > 0 {
			ratio = totBuy / (totBuy + totSell)
		}

		startTime := time.Unix(b.bars[0].Time, 0).UTC()
		endTime := time.Unix(b.bars[len(b.bars)-1].Time, 0).UTC()

		h, l := extractNodesFromBins(vp.Bins, precision)

		profiles = append(profiles, SessionVolumeProfile{
			Session:     b.session,
			Date:        b.date,
			StartTime:   startTime,
			EndTime:     endTime,
			POC:         vp.POC,
			VAH:         vp.VAH,
			VAL:         vp.VAL,
			TotalVolume: vp.TotalVolume,
			BuyVolume:   totBuy,
			SellVolume:  totSell,
			Delta:       delta,
			DeltaRatio:  ratio,
			HVNs:        h,
			LVNs:        l,
		})
	}

	return profiles
}

// calculateSessionBuySellVolume calculates buy vs sell volume based on intra-candle displacement.
func calculateSessionBuySellVolume(bars []Bar) (float64, float64) {
	totBuy := 0.0
	totSell := 0.0
	for _, b := range bars {
		cRange := b.High - b.Low
		if cRange <= 0 {
			totBuy += b.Volume * 0.5
			totSell += b.Volume * 0.5
			continue
		}
		// Weight buyer pressure by close location and body direction
		buyFrac := ((b.Close - b.Low) + (b.High - b.Open)) / (2.0 * cRange)
		if buyFrac < 0.05 {
			buyFrac = 0.05
		} else if buyFrac > 0.95 {
			buyFrac = 0.95
		}
		bVol := b.Volume * buyFrac
		sVol := b.Volume - bVol
		totBuy += bVol
		totSell += sVol
	}
	return totBuy, totSell
}

// extractNodesFromBins extracts High Volume Nodes (HVN) and Low Volume Nodes (LVN).
func extractNodesFromBins(bins []VolumeProfileBin, precision int) ([]float64, []float64) {
	var hvns []float64
	var lvns []float64

	if len(bins) < 5 {
		return hvns, lvns
	}

	// Calculate median volume
	volList := make([]float64, len(bins))
	for i, b := range bins {
		volList[i] = b.Volume
	}
	sort.Float64s(volList)
	medianVol := volList[len(volList)/2]
	if medianVol <= 0 {
		return hvns, lvns
	}

	for i := 1; i < len(bins)-1; i++ {
		// Local Peak (HVN)
		if bins[i].Volume > bins[i-1].Volume && bins[i].Volume > bins[i+1].Volume && bins[i].Volume >= medianVol*1.35 {
			hvns = append(hvns, math.Round(bins[i].Price*math.Pow10(precision))/math.Pow10(precision))
		}
		// Local Valley (LVN / Slippage corridor)
		if bins[i].Volume < bins[i-1].Volume && bins[i].Volume < bins[i+1].Volume && bins[i].Volume <= medianVol*0.45 {
			lvns = append(lvns, math.Round(bins[i].Price*math.Pow10(precision))/math.Pow10(precision))
		}
	}

	return hvns, lvns
}

// findNakedPOCs identifies prior session POCs that have not been breached by subsequent prices.
func findNakedPOCs(sessions []SessionVolumeProfile, currentPrice float64, precision int) []NakedPOC {
	var npocs []NakedPOC
	if len(sessions) <= 1 {
		return npocs
	}

	// Iterate through past sessions (excluding current running session)
	for i := 0; i < len(sessions)-1; i++ {
		sess := sessions[i]
		poc := sess.POC
		if poc <= 0 {
			continue
		}

		// Check if any subsequent session bar tested this POC
		tested := false
		for j := i + 1; j < len(sessions); j++ {
			laterSess := sessions[j]
			// If subsequent session traded across the POC level
			if laterSess.VAL <= poc && laterSess.VAH >= poc {
				tested = true
				break
			}
		}

		diff := poc - currentPrice
		distPct := (diff / currentPrice) * 100

		status := "VIRGIN / UNTESTED"
		sig := "HIGH"
		if tested {
			status = "TESTED (MITIGATED)"
			sig = "LOW"
		} else if math.Abs(distPct) > 2.0 {
			sig = "MEDIUM"
		}

		npocs = append(npocs, NakedPOC{
			Price:       math.Round(poc*math.Pow10(precision)) / math.Pow10(precision),
			Session:     sess.Session,
			Date:        sess.Date,
			TotalVolume: sess.TotalVolume,
			Distance:    diff,
			DistancePct: distPct,
			Status:      status,
			Significance: sig,
		})
	}

	return npocs
}

// SaveVolumeHistory persists session profiles and nPOCs to disk.
func SaveVolumeHistory(filePath string, report VolumeAnalysisReport) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dir for volume history: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal volume history: %w", err)
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadVolumeHistory loads volume analysis history from disk.
func LoadVolumeHistory(filePath string) (*VolumeAnalysisReport, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var report VolumeAnalysisReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse volume history: %w", err)
	}
	return &report, nil
}
