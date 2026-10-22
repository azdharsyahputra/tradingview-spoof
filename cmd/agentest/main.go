// Command agentest is a forward test of an AI agent's setup calls.
//
// It replays ~17 days of M5 history, and at N sampled moments gives the agent
// (via the `agy` CLI) exactly what the advisor knew at that moment: the
// scenario map plus recent M15/M5 candles, with timestamps made relative so the
// date cannot be looked up. The agent must pick one scenario or WAIT. Each
// pick is then forward-tested on the candles that followed with the same
// trigger/SL/TP rules as cmd/shadow, and compared with the mechanical baseline
// (taking every non-FAR scenario at the same moment).
//
//	go run ./cmd/agentest xauusd                        # 20 decisions, gemini flash
//	go run ./cmd/agentest -n 40 -model claude-sonnet-4-6 -workers 4 xauusd
//	go run ./cmd/agentest -cache /tmp/xau_m5.json xauusd  # reuse downloaded history
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

type decision struct {
	Prompt     string  `json:"prompt_version"`
	Index      int     `json:"index"`
	Time       string  `json:"time_utc"`
	Price      float64 `json:"price"`
	Choice     string  `json:"choice"`
	Confidence int     `json:"confidence"`
	Reason     string  `json:"reason"`
	Error      string  `json:"error,omitempty"`
	Turns      int     `json:"agent_turns,omitempty"`

	PickStatus string  `json:"pick_status,omitempty"`
	PickR      float64 `json:"pick_r"`
	PickPips   float64 `json:"pick_pips"`
	PickUSD    float64 `json:"pick_usd_0_01"`
	PickWhy    string  `json:"pick_why,omitempty"`
	Verdict    string  `json:"verdict"` // BENAR, SALAH, NETRAL, NO_TRIGGER

	Baseline []tvspoof.ShadowTrade `json:"baseline"`
	BaseMean float64               `json:"baseline_mean_r"`
	BaseUSD  float64               `json:"baseline_mean_usd_0_01"`
	BaseTrig int                   `json:"baseline_triggered"`
}

func main() {
	n := flag.Int("n", 20, "number of agent decisions")
	model := flag.String("model", "gemini-3.8-flash-medium", "agy model")
	workers := flag.Int("workers", 3, "parallel agy calls")
	cache := flag.String("cache", "", "JSON file to load/save downloaded M5 history")
	out := flag.String("out", "agentest_results.json", "results file")
	pv := flag.String("prompt", "v3", "prompt version(s): v1, v2, v3, a comma list like v2,v3, or ab (= v1,v2)")
	show := flag.Int("show", -1, "print the prompt of decision #k (0-based) and exit, no agent call")
	flag.Parse()
	symbol := "xauusd"
	if flag.NArg() > 0 {
		symbol = flag.Arg(0)
	}
	if _, err := exec.LookPath("agy"); err != nil {
		fail(fmt.Errorf("agy CLI not found in PATH"))
	}

	h, err := loadHistory(symbol, *cache)
	if err != nil {
		fail(err)
	}
	m5 := h.M5
	end := h.CloseTime(len(m5) - 1)
	spread := tvspoof.ContractSpecFor(h.Symbol, m5[len(m5)-1].Close).TypicalSpread

	// Candidate moments: hourly, with >= 1 non-FAR scenario and >= 36h of future.
	var cands []int
	for i := tvspoof.ScenarioWarmup; i < len(m5)-36*12; i += 12 {
		for _, sc := range h.ReportAt(i).Scenarios {
			if sc.Status != "FAR" {
				cands = append(cands, i)
				break
			}
		}
	}
	if len(cands) == 0 {
		fail(fmt.Errorf("no candidate moments in history"))
	}
	picks := evenly(cands, *n)
	fmt.Printf("AGENT FORWARD TEST %s  model %s  %d decisions from %d candidates (%s → %s)\n",
		h.Symbol, *model, len(picks), len(cands), h.CloseTime(picks[0]).Format("2006-01-02"), h.CloseTime(picks[len(picks)-1]).Format("2006-01-02"))

	if *show >= 0 {
		if *show >= len(picks) {
			fail(fmt.Errorf("-show %d out of range (%d decisions)", *show, len(picks)))
		}
		v := strings.Split(*pv, ",")[0]
		if v == "ab" {
			v = "v2"
		}
		i := picks[*show]
		fmt.Println(prompt(h, i, h.ReportAt(i), v))
		return
	}

	jail, err := os.MkdirTemp("", "agentest-jail-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(jail)

	versions := strings.Split(*pv, ",")
	if *pv == "ab" {
		versions = []string{"v1", "v2"}
	}
	type task struct {
		i int
		v string
	}
	var tasks []task
	for _, i := range picks {
		for _, v := range versions {
			tasks = append(tasks, task{i, v})
		}
	}
	results := make([]decision, len(tasks))
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for k, tk := range tasks {
		wg.Add(1)
		go func(k, i int, v string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rep := h.ReportAt(i)
			d := decision{Prompt: v, Index: i, Time: h.CloseTime(i).Format("2006-01-02 15:04"), Price: rep.Price}
			choice, conf, reason, turns, err := askAgent(jail, *model, prompt(h, i, rep, v))
			d.Choice, d.Confidence, d.Reason, d.Turns = choice, conf, reason, turns
			if err != nil {
				d.Error = err.Error()
			}
			evaluate(&d, rep, m5, end, spread)
			results[k] = d
			mu.Lock()
			done++
			fmt.Printf("  [%3d/%d] %s %s  %-16s conf %3d  → %-10s %+7.1f pips $%+6.2f  (baseline %d trig, avg $%+.2f)\n",
				done, len(tasks), d.Prompt, d.Time, d.Choice, d.Confidence, d.Verdict, d.PickPips, d.PickUSD, d.BaseTrig, d.BaseUSD)
			mu.Unlock()
		}(k, tk.i, tk.v)
	}
	wg.Wait()

	for _, v := range versions {
		var rs []decision
		for _, d := range results {
			if d.Prompt == v {
				rs = append(rs, d)
			}
		}
		fmt.Printf("\n=== PROMPT %s ===", v)
		summarize(rs)
	}
	if raw, err := json.MarshalIndent(results, "", "  "); err == nil {
		_ = os.WriteFile(*out, raw, 0644)
		fmt.Printf("\nDetail per keputusan (alasan agent, trade baseline): %s\n", *out)
	}
}

func loadHistory(symbol, cache string) (*tvspoof.ScenarioHistory, error) {
	if cache != "" {
		if raw, err := os.ReadFile(cache); err == nil {
			var h tvspoof.ScenarioHistory
			if json.Unmarshal(raw, &h) == nil && len(h.M5) > 0 {
				return &h, nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	h, err := tvspoof.NewClient().FetchScenarioHistory(ctx, symbol, 5000)
	if err != nil {
		return nil, err
	}
	if cache != "" {
		if raw, err := json.Marshal(h); err == nil {
			_ = os.WriteFile(cache, raw, 0644)
		}
	}
	return h, nil
}

// evenly picks n evenly spaced elements.
func evenly(xs []int, n int) []int {
	if n >= len(xs) {
		return xs
	}
	out := make([]int, n)
	for k := range out {
		out[k] = xs[k*len(xs)/n]
	}
	return out
}

var dateRe = regexp.MustCompile(`\s*\([A-Z][a-z]{2} \d{2} \d{2}:\d{2} UTC\)`)

// prompt renders the point-in-time snapshot with relative timestamps only.
func prompt(h *tvspoof.ScenarioHistory, i int, r *tvspoof.ScenarioReport, version string) string {
	v2 := version == "v2" || version == "v3" // v2+ get the market-context data
	v3 := version == "v3"
	now := h.CloseTime(i)
	p := r.Precision
	f := func(v float64) string { return fmt.Sprintf("%.*f", p, v) }
	rel := func(ts int64) string { return fmt.Sprintf("T-%dm", int(now.Sub(time.Unix(ts, 0)).Minutes())) }
	var b strings.Builder

	b.WriteString(`Kamu analis pendukung untuk trader CFD bergaya MARKET REACTION + LIQUIDITY (atap/lantai, sweep, konfirmasi FVG, break & retest channel, struktur HH/HL/LH/LL).
Di bawah ini snapshot pasar pada waktu T (sekarang). Kamu TIDAK tahu apa yang terjadi setelah T.
JANGAN pakai tool apa pun, JANGAN cari data di internet. Analisa hanya dari data ini.

Tugas: pilih SATU skenario yang paling layak dipasang sebagai order kondisional sekarang, atau WAIT kalau tidak ada yang layak.
Aturan eksekusi skenario (otomatis, tidak bisa diubah):
- SWEEP_REJECT: tunggu harga menyentuh level, lalu candle M5 close balik + muncul FVG berlawanan → limit di tepi FVG (berlaku 6 candle), SL di luar wick sweep. Batal kalau M5 close tembus level > 0.5 ATR.
- BREAK_RETEST: tunggu M5 close tembus level, lalu retest yang hold → entry di close retest, SL di balik level. Batal kalau close balik (fake break).
- Exit: 50% di TP1, SL pindah ke BEP, sisa ke TP2. TP1 minimal 1.5x jarak SL — TP/SL yang tertulis di skenario sudah mengikuti aturan ini (SL final dihitung ulang dari candle trigger). Skenario kadaluarsa 12 jam.
Kamu dinilai dari hasil pips di candle berikutnya. WAIT dinilai benar kalau skenario-skenario yang ada rata-rata rugi.

`)
	if v3 {
		b.WriteString(v3Rules)
	} else if v2 {
		b.WriteString(`ATURAN DISIPLIN (wajib):
1. Jangan pilih BREAK_RETEST kalau harga sudah bergerak > 1.5 ATR dalam 1 jam terakhir menuju level itu (impuls sudah habis, rawan fake break). Breakout yang bagus datang dari kompresi dekat level, bukan dari lari kencang.
2. Kalau struktur M15 dan H1 sama-sama RANGE/TRANSITION, utamakan SWEEP_REJECT di tepi range; breakout di tengah range dilarang.
3. ADR terpakai >= 85%: jangan pilih breakout searah gerak hari ini; reject atau WAIT.
4. Level FRESH (belum disentuh hari ini) lebih kuat untuk reject; level yang sudah dites >= 3x cenderung jebol (lebih cocok break).
5. Jangan jadikan R:R besar sebagai alasan. Yang dinilai peluang trigger dan arah, bukan jauhnya target.
6. Butuh tanda exhaustion/absorption di level (body mengecil, wick panjang, volume relatif tinggi tanpa progress) untuk reject. Tidak ada tanda + tidak ada konteks jelas → WAIT. WAIT itu keputusan yang sah dan sering paling benar.
7. Searah D1/H4 trend dan DXY lebih diutamakan bila semua hal lain seimbang.
8. confidence: 50 = koin, 70 = jelas lebih baik dari koin, 85+ hanya jika semua aturan di atas mendukung.

`)
	}
	fmt.Fprintf(&b, "SYMBOL %s  harga %s  sesi %s  ATR M15 %s / M5 %s\n", r.Symbol, f(r.Price), r.Session, f(r.ATR15), f(r.ATR5))
	if v3 {
		fmt.Fprintf(&b, "JAM SEKARANG %s UTC (tanggal disembunyikan)\n", now.Format("15:04"))
	}
	fmt.Fprintf(&b, "RINGKASAN: %s\n\nSTRUKTUR (candle close):\n", r.Summary)
	for _, s := range r.Structure {
		fmt.Fprintf(&b, "  %-4s %-16s %s\n", s.TF, s.Trend, s.Sequence)
		if s.LastEvent != "" {
			fmt.Fprintf(&b, "       last: %s\n", dateRe.ReplaceAllString(s.LastEvent, ""))
		}
		if s.FlipNote != "" {
			fmt.Fprintf(&b, "       flip: %s\n", s.FlipNote)
		}
	}
	if c := r.Channel; c != nil {
		fmt.Fprintf(&b, "CHANNEL M15: %s %s upper %s lower %s posisi %.0f%%\n", c.Kind, c.State, f(c.Upper), f(c.Lower), c.Position)
	}
	if v2 {
		x := r.Context
		fmt.Fprintf(&b, "KONTEKS: gerak 1 jam %+.1f ATR, 4 jam %+.1f ATR | streak M5 %+d candle | body 3 candle terakhir vs sebelumnya %.2fx | volume candle terakhir %.1fx rata-rata\n",
			x.Move1hATR, x.Move4hATR, x.StreakM5, x.BodyRatio, x.RelVolLast)
		fmt.Fprintf(&b, "         ADR %s, range hari ini %s (%.0f%% ADR terpakai) | D1 %s (%+.1f ATR D1 dari EMA20) | H4 %s (EMA50) | %s\n",
			f(x.ADR), f(x.TodayRange), x.ADRUsedPct, x.D1Trend, x.D1DistATR, x.H4Trend, x.Killzone)
		if x.DXYNote != "" {
			fmt.Fprintf(&b, "         %s\n", x.DXYNote)
		}
	}
	lvl := func(sign string, l tvspoof.LiquidityLevel) {
		q := ""
		if v2 {
			if l.Touches == 0 {
				q = " [FRESH]"
			} else {
				q = fmt.Sprintf(" [dites %dx hari ini, terakhir %dm lalu]", l.Touches, l.LastTouchMin)
			}
		}
		fmt.Fprintf(&b, "  %s %s%.1f ATR x%d %s%s\n", f(l.Price), sign, l.DistATR, l.Strength, strings.Join(l.Names, " + "), q)
	}
	b.WriteString("\nLIKUIDITAS ATAS:\n")
	for _, l := range r.Above {
		lvl("+", l)
	}
	b.WriteString("LIKUIDITAS BAWAH:\n")
	for _, l := range r.Below {
		lvl("-", l)
	}
	if len(r.FVGs) > 0 {
		b.WriteString("\nFVG AKTIF:\n")
		for _, z := range r.FVGs {
			kind := "BEAR"
			if z.Bull {
				kind = "BULL"
			}
			fmt.Fprintf(&b, "  %s %s %s-%s (%s)\n", z.TF, kind, f(z.Bottom), f(z.Top), rel(z.Time))
		}
	}
	if len(r.Reactions) > 0 {
		b.WriteString("\nREAKSI TERAKHIR (M5):\n")
		for _, e := range r.Reactions {
			fmt.Fprintf(&b, "  %s %s %s %s\n", rel(e.Time), e.Kind, f(e.Price), e.Level)
		}
	}
	candles := func(title string, bars []tvspoof.Bar) {
		fmt.Fprintf(&b, "\n%s (waktu, O H L C V):\n", title)
		for _, c := range bars {
			fmt.Fprintf(&b, "  %s %s %s %s %s %.0f\n", rel(c.Time), f(c.Open), f(c.High), f(c.Low), f(c.Close), c.Volume)
		}
	}
	candles("CANDLE M15 TERAKHIR", h.BarsAsOf("M15", i, 24))
	candles("CANDLE M5 TERAKHIR", h.BarsAsOf("M5", i, 36))
	b.WriteString("\nSKENARIO YANG BISA DIPILIH:\n")
	for _, s := range r.Scenarios {
		spec := tvspoof.ContractSpecFor(r.Symbol, r.Price)
		pp := func(a, b float64) float64 { return spec.Pips(math.Abs(a - b)) }
		fmt.Fprintf(&b, "  [%s] %s %s %s @ %s (%s) %s %.1f ATR | SL est %s (%.0f pips) | TP1 %s (%.0f pips) | TP2 %s (%.0f pips)\n      trigger: %s\n      batal: %s\n",
			s.ID, s.Priority, s.Setup, s.Side, f(s.Level), s.LevelName, s.Status, s.DistATR, f(s.EstSL), pp(s.EstSL, s.Level), f(s.Target1), pp(s.Target1, s.Level), f(s.Target2), pp(s.Target2, s.Level), s.Trigger, s.Invalid)
	}
	b.WriteString("\nJawab JSON: choice (ID skenario atau WAIT), confidence 0-100, reason (1-3 kalimat, bahasa santai).\n")
	return b.String()
}

func askAgent(dir, model, prompt string) (choice string, conf int, reason string, turns int, err error) {
	schema := `{"type":"object","properties":{"choice":{"type":"string","enum":["R1-REJECT-SELL","R1-BREAK-BUY","S1-REJECT-BUY","S1-BREAK-SELL","WAIT"]},"confidence":{"type":"integer"},"reason":{"type":"string"}},"required":["choice","confidence","reason"]}`
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		cmd := exec.CommandContext(ctx, "agy", "-p", prompt, "--output-format", "json", "--json-schema", schema,
			"--model", model, "--sandbox", "--mode", "plan", "--disable-slash-commands", "--print-timeout", "240s")
		cmd.Dir = dir
		raw, runErr := cmd.Output()
		cancel()
		if runErr != nil {
			err = fmt.Errorf("agy: %v", runErr)
			continue
		}
		var resp struct {
			Status     string `json:"status"`
			NumTurns   int    `json:"num_turns"`
			Structured struct {
				Choice     string `json:"choice"`
				Confidence int    `json:"confidence"`
				Reason     string `json:"reason"`
			} `json:"structured_output"`
		}
		if jerr := json.Unmarshal(raw, &resp); jerr != nil || resp.Structured.Choice == "" {
			err = fmt.Errorf("agy output unparsable (status %q)", resp.Status)
			continue
		}
		return resp.Structured.Choice, resp.Structured.Confidence, resp.Structured.Reason, resp.NumTurns, nil
	}
	return "WAIT", 0, "", 0, err
}

func evaluate(d *decision, rep *tvspoof.ScenarioReport, m5 []tvspoof.Bar, end time.Time, spread float64) {
	all := tvspoof.NewShadowTrades(rep, nil, 12*time.Hour, true)
	sum := 0.0
	for k := range all {
		t := &all[k]
		tvspoof.EvaluateShadow(t, m5, end, spread)
		var sc tvspoof.Scenario
		for _, s := range rep.Scenarios {
			if s.ID == t.ScenarioID {
				sc = s
			}
		}
		if sc.Status != "FAR" {
			d.Baseline = append(d.Baseline, *t)
			if t.Triggered() {
				d.BaseTrig++
				sum += t.R
				d.BaseUSD += t.USD
			}
		}
		if t.ScenarioID == d.Choice {
			d.PickStatus, d.PickR, d.PickWhy = t.Status, t.R, t.Reason
			d.PickPips, d.PickUSD = t.Pips, t.USD
		}
	}
	if d.BaseTrig > 0 {
		d.BaseMean = sum / float64(d.BaseTrig)
		d.BaseUSD /= float64(d.BaseTrig)
	}
	switch {
	case d.Error != "":
		d.Verdict = "ERROR"
	case d.Choice == "WAIT":
		if d.BaseTrig == 0 || d.BaseMean <= 0 {
			d.Verdict = "BENAR"
		} else {
			d.Verdict = "SALAH"
			d.PickWhy = fmt.Sprintf("WAIT melewatkan skenario yang rata-rata $%+.2f @0.01 lot", d.BaseUSD)
		}
	case d.PickStatus == tvspoof.ShadowWin:
		d.Verdict = "BENAR"
	case d.PickStatus == tvspoof.ShadowLoss:
		d.Verdict = "SALAH"
	case d.PickStatus == tvspoof.ShadowBreakeven:
		d.Verdict = "NETRAL"
	default:
		d.Verdict = "NO_TRIGGER"
	}
}

func summarize(rs []decision) {
	count := map[string]int{}
	var pickPips, pickUSD, basePips, baseUSD []float64
	waits, waitRight := 0, 0
	for _, d := range rs {
		count[d.Verdict]++
		if d.Choice == "WAIT" {
			waits++
			if d.Verdict == "BENAR" {
				waitRight++
			}
		} else if d.PickStatus == tvspoof.ShadowWin || d.PickStatus == tvspoof.ShadowLoss || d.PickStatus == tvspoof.ShadowBreakeven {
			pickPips = append(pickPips, d.PickPips)
			pickUSD = append(pickUSD, d.PickUSD)
		}
		for _, t := range d.Baseline {
			if t.Triggered() {
				basePips = append(basePips, t.Pips)
				baseUSD = append(baseUSD, t.USD)
			}
		}
	}
	sum := func(xs []float64) float64 {
		s := 0.0
		for _, x := range xs {
			s += x
		}
		return s
	}
	avg := func(xs []float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		return sum(xs) / float64(len(xs))
	}
	wins := func(xs []float64) int {
		w := 0
		for _, x := range xs {
			if x > 0 {
				w++
			}
		}
		return w
	}
	fmt.Printf("\nHASIL: BENAR %d | SALAH %d | NETRAL %d | NO_TRIGGER %d | ERROR %d\n",
		count["BENAR"], count["SALAH"], count["NETRAL"], count["NO_TRIGGER"], count["ERROR"])
	fmt.Printf("Pilihan agent yang masuk       : %d trade, win %d / lose %d | total %+.1f pips, $%+.2f @0.01 lot (rata-rata %+.1f pips)\n",
		len(pickPips), wins(pickPips), len(pickPips)-wins(pickPips), sum(pickPips), sum(pickUSD), avg(pickPips))
	fmt.Printf("Baseline semua skenario mesin  : %d trade, win %d / lose %d | rata-rata %+.1f pips, $%+.2f @0.01 lot per trade\n",
		len(basePips), wins(basePips), len(basePips)-wins(basePips), avg(basePips), avg(baseUSD))
	fmt.Printf("WAIT: %d kali, benar %d\n", waits, waitRight)
	if len(pickPips) < 20 {
		fmt.Println("Catatan: sampel kecil (<20 trade masuk) — belum bisa disimpulkan, naikin -n.")
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// v3Rules is the market-reaction reading method from AGENTS.md (sections A–F),
// written without any directional preference.
const v3Rules = `CARA BACA (wajib, urut):
A. SESI. ASIA 00-07 UTC membangun range (tengah range = WAIT). LONDON 07-12 UTC biasanya menyapu SATU sisi range Asia dulu baru gerak asli; jangan entry 07:15-08:00 UTC. NY 13:30-17:00 UTC melanjutkan/mengoreksi London; 15-60 menit pertama rawan stop run.
B. LOKASI. Hanya tepi: atap/lantai likuiditas, FVG di ujung gerakan, zona flip. Dekat POC/tengah range = WAIT. Level yang sudah dites >=3-4x melemah; high/low yang makin menekan ke dalam zona = break makin mungkin.
C. FVG. FVG lahir di tengah range sering jebol. FVG dari displacement di ujung gerakan lebih andal. Reaksi paling presisi di tepi FVG. Kalau FVG terdekat langsung ditembus close, momentum berbalik dan zona reject yang valid adalah FVG berikutnya.
D. BREAK. Valid kalau ada absorption dulu (>=2 wick panjang berturut-turut yang diserap) atau kompresi rapat dekat level, lalu displacement close tembus. Lari kencang ke level tanpa absorption (>1.5 ATR/jam) = jangan kejar breakout. Stop run = tembus EQH/EQL lalu close balik + marubozu arah sebaliknya + MSS M5 → fade.
E. FLIP. Level dijebol lalu di-retest dari sisi sebaliknya dan hold = flip. Breakout gagal = level lama jadi atap/lantai lagi.
F. COUNTER-TREND (melawan H1/H4) hanya dengan sweep tuntas + MSS M5 + FVG searah reversal; kalau tidak, skip.
Tidak ada yang memenuhi A-F dengan jelas → WAIT. WAIT itu jawaban sah.

`
