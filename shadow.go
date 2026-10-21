package tvspoof

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// Shadow trading: every scenario the advisor proposes is paper-traded against
// real M5 candles so its calls can be graded and turned into lessons.

// DefaultShadowPath is the shadow-trade journal.
const DefaultShadowPath = "shadow_trades.json"

// Shadow statuses.
const (
	ShadowPending     = "PENDING"     // waiting for trigger
	ShadowActive      = "ACTIVE"      // triggered, managing exits
	ShadowWin         = "WIN"         // R > 0.05
	ShadowLoss        = "LOSS"        // R < -0.05
	ShadowBreakeven   = "BREAKEVEN"   // |R| <= 0.05
	ShadowInvalidated = "INVALIDATED" // invalidation hit before trigger
	ShadowExpired     = "EXPIRED"     // no trigger before expiry
)

// ShadowTrade is one paper-traded scenario.
type ShadowTrade struct {
	ID         string  `json:"id"`
	Symbol     string  `json:"symbol"`
	CreatedAt  int64   `json:"created_at"`
	ExpiresAt  int64   `json:"expires_at"`
	ScenarioID string  `json:"scenario_id"`
	Setup      string  `json:"setup"`
	Side       string  `json:"side"`
	Level      float64 `json:"level"`
	LevelName  string  `json:"level_name"`
	ATR        float64 `json:"atr_m15"`
	Bias       string  `json:"bias"`
	Priority   string  `json:"priority"`
	Session    string  `json:"session"`
	PlannedTP1 float64 `json:"planned_tp1"`
	PlannedTP2 float64 `json:"planned_tp2"`
	Context    string  `json:"context,omitempty"` // advisor summary at creation
	// Market context at creation, for rule research (cmd/shadow research).
	Ctx *ShadowContext `json:"ctx,omitempty"`

	Status     string  `json:"status"`
	TriggerAt  int64   `json:"trigger_at,omitempty"`
	Entry      float64 `json:"entry,omitempty"`
	SL         float64 `json:"sl,omitempty"`
	TP1        float64 `json:"tp1,omitempty"`
	TP2        float64 `json:"tp2,omitempty"`
	TP1Hit     bool    `json:"tp1_hit,omitempty"`
	RunnerHit  bool    `json:"runner_hit,omitempty"`
	R          float64 `json:"r,omitempty"`
	Pips       float64 `json:"pips,omitempty"`     // realised, whole position (50% TP1 + runner), after spread
	USD        float64 `json:"usd_0_01,omitempty"` // realised USD at 0.01 lot
	SLPips     float64 `json:"sl_pips,omitempty"`  // entry → SL distance
	TP1Pips    float64 `json:"tp1_pips,omitempty"`
	TP2Pips    float64 `json:"tp2_pips,omitempty"`
	ResolvedAt int64   `json:"resolved_at,omitempty"`
	Reason     string  `json:"reason,omitempty"`
	LessonID   string  `json:"lesson_id,omitempty"`
}

// ShadowContext snapshots how price was trading when the scenario was logged.
type ShadowContext struct {
	Hour        int     `json:"hour_utc"`
	DistATR     float64 `json:"dist_atr"`
	Move1hATR   float64 `json:"move_1h_atr"`   // signed toward the trade side (+ = price already running the trade's way)
	MoveToLevel float64 `json:"move_to_level"` // signed toward the level (+ = price running INTO the level)
	ADRUsedPct  float64 `json:"adr_used_pct"`
	Touches     int     `json:"level_touches"`
	Strength    int     `json:"level_strength"`
	Range30mATR float64 `json:"range_30m_atr"`
	AbsorbFor   int     `json:"absorb_for"` // absorption wicks supporting the trade side
	H1Trend     string  `json:"h1_trend"`
	D1Aligned   bool    `json:"d1_aligned"`
}

// ShadowRules are the trigger/exit mechanics shared by live shadow trading,
// backfill and the agent test. Change them only through research runs.
type ShadowRules struct {
	SweepSLBufATR   float64 // SL beyond the sweep extreme
	BreakSLBufATR   float64 // SL beyond the retest low / level
	MinRiskATR      float64 // minimum stop distance
	TP1MinR         float64 // TP1 at least this multiple of the stop distance
	TP2AddR         float64 // TP2 = TP1 + this multiple (when no level target)
	UseLevelTargets bool    // use liquidity-level targets when they fit
	MaxHoldBars     int     // M5 bars after the trigger before a market time-out
}

// ShadowParams is the active rule set.
var ShadowParams = ShadowRules{SweepSLBufATR: 0.15, BreakSLBufATR: 0.3, MinRiskATR: 0.3, TP1MinR: 1.5, TP2AddR: 1.5, UseLevelTargets: true, MaxHoldBars: 288}

// Open reports whether the trade still needs evaluation.
func (t ShadowTrade) Open() bool { return t.Status == ShadowPending || t.Status == ShadowActive }

// Triggered reports whether the trade had an entry.
func (t ShadowTrade) Triggered() bool { return t.TriggerAt > 0 }

// LoadShadowTrades reads the journal; a missing file is empty.
func LoadShadowTrades(path string) ([]ShadowTrade, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ShadowTrade
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	return out, json.Unmarshal(data, &out)
}

// SaveShadowTrades writes the journal.
func SaveShadowTrades(path string, trades []ShadowTrade) error {
	data, err := json.MarshalIndent(trades, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// NewShadowTrades converts advisor scenarios into pending shadow trades,
// skipping FAR scenarios and duplicates of still-open trades.
func NewShadowTrades(rep *ScenarioReport, existing []ShadowTrade, expiry time.Duration, includeFar bool) []ShadowTrade {
	var out []ShadowTrade
	seq := len(existing)
	for _, sc := range rep.Scenarios {
		if sc.Status == "FAR" && !includeFar {
			continue
		}
		dup := false
		for _, e := range append(existing, out...) {
			if e.Open() && e.Symbol == rep.Symbol && e.Setup == sc.Setup && e.Side == sc.Side && math.Abs(e.Level-sc.Level) <= 0.25*rep.ATR15 {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		seq++
		ctx := shadowContext(rep, sc)
		out = append(out, ShadowTrade{
			Ctx:        ctx,
			ID:         fmt.Sprintf("SHD-%s-%04d", shortSymbol(rep.Symbol), seq),
			Symbol:     rep.Symbol,
			CreatedAt:  rep.Time.Unix(),
			ExpiresAt:  rep.Time.Add(expiry).Unix(),
			ScenarioID: sc.ID,
			Setup:      sc.Setup,
			Side:       sc.Side,
			Level:      sc.Level,
			LevelName:  sc.LevelName,
			ATR:        rep.ATR15,
			Bias:       rep.Bias,
			Priority:   sc.Priority,
			Session:    rep.Session,
			PlannedTP1: sc.Target1,
			PlannedTP2: sc.Target2,
			Context:    rep.Summary,
			Status:     ShadowPending,
		})
	}
	return out
}

func shortSymbol(sym string) string {
	if i := strings.LastIndex(sym, ":"); i >= 0 {
		sym = sym[i+1:]
	}
	return sym
}

// EvaluateShadow advances a shadow trade over M5 candles (any order, any
// overlap; only CLOSED candles after CreatedAt are used).
func EvaluateShadow(t *ShadowTrade, m5 []Bar, now time.Time, spread float64) {
	if !t.Open() {
		return
	}
	bars := closedOnly(m5, 5*time.Minute, now)
	start := sort.Search(len(bars), func(i int) bool { return bars[i].Time >= t.CreatedAt })
	bars = bars[start:]

	// Canonical orientation: SWEEP_REJECT = sell at resistance,
	// BREAK_RETEST = buy above resistance. Mirror prices for the other side.
	flip := (t.Setup == "SWEEP_REJECT" && t.Side == "BUY") || (t.Setup == "BREAK_RETEST" && t.Side == "SELL")
	sgn := 1.0
	if flip {
		sgn = -1
	}
	cb := make([]Bar, len(bars))
	for i, b := range bars {
		cb[i] = b
		if flip {
			cb[i] = Bar{Time: b.Time, Open: -b.Open, High: -b.Low, Low: -b.High, Close: -b.Close}
		}
	}
	L, atr := t.Level*sgn, t.ATR
	tp1c, tp2c := t.PlannedTP1*sgn, t.PlannedTP2*sgn

	trig := -1
	var entry, sl float64
	if !t.Triggered() {
		switch t.Setup {
		case "SWEEP_REJECT": // sell canonical
			touched, extreme := -1, math.Inf(-1)
			for i, b := range cb {
				if b.Time >= t.ExpiresAt {
					break
				}
				if b.Close > L+0.5*atr {
					t.resolve(ShadowInvalidated, b.Time, "level jebol (close jauh di atas level) sebelum ada konfirmasi reject")
					return
				}
				if b.High >= L-0.1*atr {
					if touched < 0 {
						touched = i
					}
					extreme = math.Max(extreme, b.High)
				}
				if touched >= 0 && i >= 2 && i-2 >= touched-1 && b.High < cb[i-2].Low && cb[i-1].Close < cb[i-1].Open && b.Close < L {
					// Confirmation FVG printed. Sniper entry: sell limit at the
					// FVG's proximal edge (this candle's high), valid 6 candles.
					limit := b.High
					sl = extreme + ShadowParams.SweepSLBufATR*atr
					ranAway := false
					for j := i + 1; j < len(cb) && j <= i+6; j++ {
						c := cb[j]
						if c.High >= limit {
							if c.High >= sl {
								// filled and stopped inside the same candle
								t.TriggerAt = c.Time
								t.Entry, t.SL = limit*sgn, sl*sgn
								t.TP1, t.TP2 = t.Entry, t.Entry
								t.R = -1 - spread/math.Max(sl-limit, 1e-9)
								t.setPips()
								t.resolve(ShadowLoss, c.Time, "limit FVG ke-fill lalu SL kena di candle yang sama")
								return
							}
							trig, entry = j, limit
							break
						}
						if c.Low <= limit-2*(sl-limit) {
							ranAway = true // ran 2R away without retesting the FVG
							break
						}
					}
					if trig < 0 && !ranAway && len(cb) <= i+6 {
						return // limit still working, wait for more candles
					}
					if trig < 0 {
						t.resolve(ShadowExpired, b.Time, "FVG konfirmasi muncul tapi tidak di-retest (limit tidak ke-fill) — jangan kejar")
						return
					}
					break
				}
			}
		case "BREAK_RETEST": // buy canonical
			broke := -1
			for i, b := range cb {
				if b.Time >= t.ExpiresAt {
					break
				}
				if broke < 0 {
					if b.Close > L+0.1*atr {
						broke = i
					}
					continue
				}
				if b.Close < L-0.3*atr {
					t.resolve(ShadowInvalidated, b.Time, "fake breakout: close balik ke bawah level sebelum retest")
					return
				}
				if b.High > L+2.5*atr && b.Low > L+0.25*atr {
					t.resolve(ShadowExpired, b.Time, "harga kabur tanpa retest (no entry, jangan kejar)")
					return
				}
				if i > broke && b.Low <= L+0.25*atr && b.Close > L {
					trig, entry = i, b.Close
					sl = math.Min(b.Low, L) - ShadowParams.BreakSLBufATR*atr
					break
				}
			}
		}
		if trig < 0 {
			if now.Unix() >= t.ExpiresAt {
				t.resolve(ShadowExpired, t.ExpiresAt, "tidak ada trigger sampai expiry")
			}
			return
		}
		// Canonical SL is above entry for SWEEP_REJECT (sell) and below for BREAK_RETEST (buy).
		risk := math.Abs(entry - sl)
		risk = math.Max(risk, ShadowParams.MinRiskATR*atr)
		if risk > 2.5*atr {
			t.resolve(ShadowExpired, cb[trig].Time, fmt.Sprintf("trigger muncul tapi SL terlalu lebar (%.1f ATR) — skip", risk/atr))
			return
		}
		dirC := 1.0 // canonical direction of the trade
		if t.Setup == "SWEEP_REJECT" {
			dirC = -1
		}
		sl = entry - dirC*risk
		m1 := ShadowParams.TP1MinR
		tp1 := entry + dirC*m1*risk
		if d := (tp1c - entry) * dirC; ShadowParams.UseLevelTargets && d > m1*risk && d < 6*risk {
			tp1 = tp1c
		}
		tp2 := tp1 + dirC*ShadowParams.TP2AddR*risk
		if d := (tp2c - tp1) * dirC; d > 0.5*risk && d < 8*risk {
			tp2 = tp2c
		}
		t.TriggerAt = cb[trig].Time
		t.Entry, t.SL, t.TP1, t.TP2 = entry*sgn, sl*sgn, tp1*sgn, tp2*sgn
		t.Status = ShadowActive
	}

	// Manage exits on real bars after the trigger candle.
	k := sort.Search(len(bars), func(i int) bool { return bars[i].Time > t.TriggerAt })
	bt := BacktestTrade{Entry: t.Entry, SL: t.SL, TP1: t.TP1, TP2: t.TP2}
	future := bars[k:]
	maxHold := ShadowParams.MaxHoldBars
	simulateManagedTrade(&bt, t.Side == "BUY", future, maxHold, spread)
	t.TP1Hit, t.RunnerHit, t.R = bt.TP1Hit, bt.RunnerHit, bt.R
	t.setPips()
	finished := bt.Exited || len(future) >= maxHold
	if !finished {
		return
	}
	end := t.TriggerAt
	if bt.ExitTime > 0 {
		end = bt.ExitTime
	}
	switch {
	case bt.R > 0.05:
		t.resolve(ShadowWin, end, exitReason(bt))
	case bt.R < -0.05:
		t.resolve(ShadowLoss, end, exitReason(bt))
	default:
		t.resolve(ShadowBreakeven, end, exitReason(bt))
	}
}

func exitReason(bt BacktestTrade) string {
	switch {
	case bt.RunnerHit:
		return "TP1 + TP2 kena (runner full)"
	case bt.TP1Hit && bt.R > 0:
		return "TP1 kena, runner balik ke BEP"
	case bt.StoppedOut:
		return "SL kena sebelum TP1"
	}
	return fmt.Sprintf("time-out %s, ditutup di harga pasar", time.Duration(ShadowParams.MaxHoldBars)*5*time.Minute)
}

// setPips fills the pip / USD view of the trade from R and the levels.
func (t *ShadowTrade) setPips() {
	spec := ContractSpecFor(t.Symbol, t.Entry)
	risk := math.Abs(t.Entry - t.SL)
	r1 := func(v float64) float64 { return math.Round(v*10) / 10 }
	t.Pips = r1(spec.Pips(t.R * risk))
	t.USD = math.Round(spec.USDMicro(t.R*risk)*100) / 100
	t.SLPips = r1(spec.Pips(risk))
	t.TP1Pips = r1(spec.Pips(math.Abs(t.TP1 - t.Entry)))
	t.TP2Pips = r1(spec.Pips(math.Abs(t.TP2 - t.Entry)))
}

func (t *ShadowTrade) resolve(status string, at int64, reason string) {
	t.Status, t.ResolvedAt, t.Reason = status, at, reason
}

// ShadowLesson turns a resolved, triggered shadow trade into a lesson entry.
func ShadowLesson(t ShadowTrade, id string, stat ShadowSetupStat) Lesson {
	outcome := t.Status
	grade := "C"
	switch {
	case t.R >= 2: // >= 2x the SL distance
		grade = "A+"
	case t.R >= 1:
		grade = "A"
	case t.R > 0.05:
		grade = "B"
	case t.R < -0.05:
		grade = "D"
	}
	pips := t.Pips
	rrr := fmt.Sprintf("%+.1f pips ($%+.2f @0.01 lot)", t.Pips, t.USD)
	align := "counter-trend"
	if t.Priority == "PRIMARY" {
		align = "searah struktur"
	} else if t.Priority == "SECONDARY" {
		align = "struktur mixed"
	}
	worked, failed := "", ""
	switch t.Status {
	case ShadowWin:
		worked = fmt.Sprintf("%s %s di %s (%s, bias %s) valid: %s.", t.Setup, t.Side, t.LevelName, align, t.Bias, t.Reason)
	case ShadowLoss:
		failed = fmt.Sprintf("%s %s di %s (%s, bias %s) gagal: %s.", t.Setup, t.Side, t.LevelName, align, t.Bias, t.Reason)
	default:
		worked = fmt.Sprintf("%s %s di %s: %s.", t.Setup, t.Side, t.LevelName, t.Reason)
	}
	lesson := fmt.Sprintf("[SHADOW] %s %s %s di %s → %s %s (SL %.0f pips, TP1 %.0f pips). Track record setup ini (%s): %d trigger, win %.0f%%, rata-rata %+.1f pips/trade.",
		t.Setup, t.Side, align, t.LevelName, outcome, rrr, t.SLPips, t.TP1Pips, stat.Key, stat.Triggered, stat.WinRate*100, stat.AvgPips)
	tp := t.TP1
	if t.RunnerHit {
		tp = t.TP2
	}
	var tpHit *float64
	if t.TP1Hit {
		tpHit = &tp
	}
	return Lesson{
		ID:         id,
		Date:       time.Unix(t.TriggerAt, 0).UTC().Format("2006-01-02"),
		Symbol:     t.Symbol,
		Direction:  t.Side,
		Outcome:    outcome,
		Entry:      t.Entry,
		SL:         t.SL,
		TPHit:      tpHit,
		PnLPips:    &pips,
		RRRActual:  &rrr,
		PlanID:     t.ID,
		SetupType:  t.Setup,
		Timeframe:  "5m",
		Session:    t.Session,
		Tags:       []string{"shadow", strings.ToLower(t.Setup), strings.ToLower(t.Side), strings.ToLower(t.Priority)},
		WhatWorked: worked,
		WhatFailed: failed,
		Lesson:     lesson,
		Grade:      grade,
		CreatedAt:  time.Now().UTC().Format("2006-01-02 15:04:05"),
	}
}

// ShadowSetupStat aggregates resolved shadow trades by setup/side/priority.
type ShadowSetupStat struct {
	Key         string  `json:"key"`
	Logged      int     `json:"logged"`
	Triggered   int     `json:"triggered"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	Invalidated int     `json:"invalidated"`
	Expired     int     `json:"expired"`
	WinRate     float64 `json:"win_rate"` // wins / resolved triggered
	ExpectancyR float64 `json:"expectancy_r"`
	TotalR      float64 `json:"total_r"`
	TotalPips   float64 `json:"total_pips"`
	AvgPips     float64 `json:"avg_pips"`
	TotalUSD    float64 `json:"total_usd_0_01"`
}

// ShadowStats groups trades by "SETUP SIDE PRIORITY" plus an "ALL" row.
func ShadowStats(trades []ShadowTrade) []ShadowSetupStat {
	m := map[string]*ShadowSetupStat{}
	add := func(key string, t ShadowTrade) {
		s, ok := m[key]
		if !ok {
			s = &ShadowSetupStat{Key: key}
			m[key] = s
		}
		s.Logged++
		switch t.Status {
		case ShadowInvalidated:
			s.Invalidated++
		case ShadowExpired:
			s.Expired++
		case ShadowWin, ShadowLoss, ShadowBreakeven:
			s.Triggered++
			s.TotalR += t.R
			s.TotalPips += t.Pips
			s.TotalUSD += t.USD
			if t.Status == ShadowWin {
				s.Wins++
			} else if t.Status == ShadowLoss {
				s.Losses++
			}
		}
	}
	for _, t := range trades {
		add(ShadowKey(t), t)
		add(t.Setup+" "+t.Side, t)
		add("ALL", t)
	}
	var out []ShadowSetupStat
	for _, s := range m {
		if s.Triggered > 0 {
			s.WinRate = float64(s.Wins) / float64(s.Triggered)
			s.ExpectancyR = s.TotalR / float64(s.Triggered)
			s.AvgPips = s.TotalPips / float64(s.Triggered)
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ShadowKey is the finest stats bucket of a trade.
func ShadowKey(t ShadowTrade) string { return t.Setup + " " + t.Side + " " + t.Priority }

// StatFor returns the stat row for key (zero value if missing).
func StatFor(stats []ShadowSetupStat, key string) ShadowSetupStat {
	for _, s := range stats {
		if s.Key == key {
			return s
		}
	}
	return ShadowSetupStat{Key: key}
}

func shadowContext(rep *ScenarioReport, sc Scenario) *ShadowContext {
	x := rep.Context
	dir := 1.0
	if sc.Side == "SELL" {
		dir = -1
	}
	toLevel := 1.0
	if sc.Level < rep.Price {
		toLevel = -1
	}
	c := &ShadowContext{
		Hour: rep.Time.UTC().Hour(), DistATR: sc.DistATR,
		Move1hATR: x.Move1hATR * dir, MoveToLevel: x.Move1hATR * toLevel,
		ADRUsedPct: x.ADRUsedPct, Range30mATR: x.Range30mATR,
		D1Aligned: (x.D1Trend == "UP") == (sc.Side == "BUY") && x.D1Trend != "",
	}
	if sc.Side == "BUY" {
		c.AbsorbFor = x.AbsorbLow
	} else {
		c.AbsorbFor = x.AbsorbHigh
	}
	for _, l := range append(append([]LiquidityLevel{}, rep.Above...), rep.Below...) {
		if l.Price == sc.Level {
			c.Touches, c.Strength = l.Touches, l.Strength
		}
	}
	for _, st := range rep.Structure {
		if st.TF == "H1" {
			c.H1Trend = st.Trend
		}
	}
	return c
}
