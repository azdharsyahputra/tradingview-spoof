package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

// ANSI Colors
const (
	colorReset    = "\033[0m"
	colorBold     = "\033[1m"
	colorGreen    = "\033[38;5;48m"
	colorRed      = "\033[38;5;203m"
	colorYellow   = "\033[38;5;220m"
	colorCyan     = "\033[38;5;75m"
	colorDarkGray = "\033[38;5;240m"
	colorBgAlert  = "\033[48;5;196m\033[38;5;231m"
	colorBgGreen  = "\033[48;5;28m\033[38;5;231m"
)

type TriggerEngine struct {
	mu            sync.RWMutex
	symbol        string
	lastPrice     float64
	todayHigh     float64
	todayLow      float64
	lastAlert     map[string]time.Time
	alertCoolDown time.Duration

	// Key Levels
	retestSellLow  float64
	retestSellHigh float64
	reclaimTrapLvl float64
	breakdownLvl   float64
	s2TargetLvl    float64
	resistenLvl    float64
}

func NewTriggerEngine(symbol string) *TriggerEngine {
	return &TriggerEngine{
		symbol:         symbol,
		lastAlert:      make(map[string]time.Time),
		alertCoolDown:  15 * time.Second, // Cooldown per category agar tidak spamming
		retestSellLow:  4298.50,
		retestSellHigh: 4302.50,
		reclaimTrapLvl: 4304.50,
		breakdownLvl:   4291.50,
		s2TargetLvl:    4289.34,
		resistenLvl:    4316.44,
	}
}

func (te *TriggerEngine) CanAlert(key string) bool {
	te.mu.Lock()
	defer te.mu.Unlock()
	last, ok := te.lastAlert[key]
	if !ok || time.Since(last) >= te.alertCoolDown {
		te.lastAlert[key] = time.Now()
		return true
	}
	return false
}

func (te *TriggerEngine) EvaluateTick(price float64) {
	te.mu.Lock()
	te.lastPrice = price
	if te.todayLow == 0 || price < te.todayLow {
		te.todayLow = price
	}
	if price > te.todayHigh {
		te.todayHigh = price
	}
	te.mu.Unlock()

	nowStr := time.Now().Format("15:04:05.000")

	// 1. Trigger SELL RETEST ZONE [4298.50 - 4302.50]
	if price >= te.retestSellLow && price <= te.retestSellHigh {
		if te.CanAlert("RETEST_SELL") {
			fmt.Printf("\a\n%s 🎯 [TRIGGER VALID: SELL RETEST ZONE!] %s\n", colorBgAlert, colorReset)
			fmt.Printf("   %sTime:%s %s | %sPrice:%s %.2f\n", colorDarkGray, colorReset, nowStr, colorBold, colorReset, price)
			fmt.Printf("   %sSetup:%s RETEST BREAKDOWN LEVEL (Asian Low Re-test)\n", colorCyan, colorReset)
			fmt.Printf("   %sAction:%s SELL @ %.2f | SL: %.2f | TP1: %.2f | TP2: %.2f\n", colorRed, colorReset, price, te.reclaimTrapLvl, 4292.00, te.s2TargetLvl)
			fmt.Printf("   --------------------------------------------------------\n\n")
		}
	}

	// 2. Trigger BEAR TRAP / BULLISH RECLAIM (> 4304.50)
	if price > te.reclaimTrapLvl {
		if te.CanAlert("BEAR_TRAP") {
			fmt.Printf("\a\n%s ⚠️ [TRIGGER: BULLISH RECLAIM / BEAR TRAP DETECTED!] %s\n", colorBgGreen, colorReset)
			fmt.Printf("   %sTime:%s %s | %sPrice:%s %.2f\n", colorDarkGray, colorReset, nowStr, colorBold, colorReset, price)
			fmt.Printf("   %sAnalysis:%s Harga menembus ke atas %.2f. Breakdown tadi adalah Liquidity Sweep / Fakeout!\n", colorYellow, colorReset, te.reclaimTrapLvl)
			fmt.Printf("   %sNext Target:%s %.2f (S1 Pivot / Retest Structure)\n", colorGreen, colorReset, te.resistenLvl)
			fmt.Printf("   --------------------------------------------------------\n\n")
		}
	}

	// 3. Trigger NEW BREAKDOWN (< 4291.50)
	if price < te.breakdownLvl && price > te.s2TargetLvl {
		if te.CanAlert("BREAKDOWN_EXPANSION") {
			fmt.Printf("\a\n%s 🚨 [TRIGGER: BREAKDOWN CONTINUATION!] %s\n", colorBgAlert, colorReset)
			fmt.Printf("   %sTime:%s %s | %sPrice:%s %.2f\n", colorDarkGray, colorReset, nowStr, colorBold, colorReset, price)
			fmt.Printf("   %sAnalysis:%s New Low tembus di bawah %.2f, menuju langsung ke Daily Pivot S2 (%.2f)\n", colorRed, colorReset, te.breakdownLvl, te.s2TargetLvl)
			fmt.Printf("   --------------------------------------------------------\n\n")
		}
	}

	// 4. Trigger S2 TARGET HIT (<= 4289.34)
	if price <= te.s2TargetLvl {
		if te.CanAlert("S2_TARGET_HIT") {
			fmt.Printf("\a\n%s 🏁 [TRIGGER: S2 DAILY EXPANSION TARGET HIT!] %s\n", colorBgGreen, colorReset)
			fmt.Printf("   %sTime:%s %s | %sPrice:%s %.2f\n", colorDarkGray, colorReset, nowStr, colorBold, colorReset, price)
			fmt.Printf("   %sAction:%s Amankan TP posisi SELL. Waspadai technical bounce di lantai S2.\n", colorGreen, colorReset)
			fmt.Printf("   --------------------------------------------------------\n\n")
		}
	}

	// 5. Trigger APPROACHING HTF RESISTANCE (>= 4315.00)
	if price >= 4315.00 && price <= 4323.00 {
		if te.CanAlert("HTF_RESISTANCE") {
			fmt.Printf("\a\n%s 🏰 [TRIGGER: HTF RESISTANCE RETEST!] %s\n", colorBgAlert, colorReset)
			fmt.Printf("   %sTime:%s %s | %sPrice:%s %.2f\n", colorDarkGray, colorReset, nowStr, colorBold, colorReset, price)
			fmt.Printf("   %sSetup:%s Retest Previous Day Low / S1 Pivot. Pantau Rejection SELL!\n", colorYellow, colorReset)
			fmt.Printf("   --------------------------------------------------------\n\n")
		}
	}
}

func main() {
	symbolFlag := flag.String("symbol", "xauusd", "Trading symbol (e.g., xauusd, eurusd, btcusd)")
	flag.Parse()

	sym := strings.TrimSpace(*symbolFlag)
	if sym == "" {
		sym = "xauusd"
	}

	tvSym := tvspoof.NormalizeDeskSymbol(sym)
	engine := NewTriggerEngine(tvSym)

	fmt.Println(colorBold + colorCyan + "╔════════════════════════════════════════════════════════════════════════╗" + colorReset)
	fmt.Printf(colorBold+colorCyan+"║  ⚡ ZTERM REAL-TIME TICK TRIGGER DAEMON  •  %s\n"+colorReset, tvSym)
	fmt.Println(colorBold + colorCyan + "╚════════════════════════════════════════════════════════════════════════╝" + colorReset)
	fmt.Printf("📡 Mengkoneksikan WebSocket Real-Time Stream untuk %s%s%s...\n", colorYellow, tvSym, colorReset)
	fmt.Printf("🎯 Radar Rules Aktif:\n")
	fmt.Printf("   • [SELL RETEST]    : %.2f – %.2f (SL: %.2f, TP: %.2f)\n", engine.retestSellLow, engine.retestSellHigh, engine.reclaimTrapLvl, engine.s2TargetLvl)
	fmt.Printf("   • [BEAR TRAP]      : Break di atas %.2f (Target S1: %.2f)\n", engine.reclaimTrapLvl, engine.resistenLvl)
	fmt.Printf("   • [BREAKDOWN S2]   : Break di bawah %.2f ➔ S2: %.2f\n\n", engine.breakdownLvl, engine.s2TargetLvl)

	client := tvspoof.NewClient()
	client.OnQuote = func(q tvspoof.QuoteUpdate) {
		if q.Price != nil {
			engine.EvaluateTick(*q.Price)
		}
	}

	client.OnError = func(err error) {
		fmt.Printf("%s[WS ERROR]%s %v\n", colorRed, colorReset, err)
	}

	if err := client.Connect(); err != nil {
		fmt.Printf("%s[FATAL]%s Gagal koneksi: %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}
	defer client.Close()

	client.AddSymbol(tvSym)
	fmt.Printf("%s✅ Real-time tick stream aktif! Menunggu validasi trigger...%s\n\n", colorGreen, colorReset)

	// Keep running until interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Heartbeat ticker to show stream is alive
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Printf("\n%s🛑 Trigger engine dihentikan.%s\n", colorYellow, colorReset)
			return
		case <-ticker.C:
			engine.mu.RLock()
			lp := engine.lastPrice
			engine.mu.RUnlock()
			if lp > 0 {
				fmt.Printf("%s[%s]%s Tick Stream Active | %s: %s%.2f%s\n", colorDarkGray, time.Now().Format("15:04:05"), colorReset, tvSym, colorBold, lp, colorReset)
			}
		}
	}
}
