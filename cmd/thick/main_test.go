package main

import (
	"strings"
	"testing"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func TestRenderDashboardAlignmentScenarios(t *testing.T) {
	symbols := []string{"OANDA:XAUUSD", "BINANCE:BTCUSDT", "FX:EURUSD", "TVC:SPX", "FOREXCOM:NAS100"}
	expectedWidth := 96

	for _, sym := range symbols {
		t.Run(sym, func(t *testing.T) {
			price := 7650.50
			change := 12.34
			chgPct := 0.16
			state := &MarketState{
				symbol:        sym,
				interval:      "15",
				lastPrice:     &price,
				change:        &change,
				changePercent: &chgPct,
				tickCount:     5432,
				statusMsg:     "OK",
				precision:     2,
				hasActiveBar:  true,
				activeBar: tvspoof.Bar{
					Time:  time.Now().Unix(),
					Open:  7645.0,
					High:  7655.0,
					Low:   7640.0,
					Close: 7650.5,
				},
				historyBars: []tvspoof.Bar{
					{Time: 1726900000, Open: 7640.0, High: 7650.0, Low: 7635.0, Close: 7645.0},
					{Time: 1726900900, Open: 7645.0, High: 7655.0, Low: 7640.0, Close: 7650.5},
				},
			}

			output := renderDashboardString(state, 10, nil)
			lines := strings.Split(output, "\n")

			for lineIdx, line := range lines {
				clean := stripANSI(line)
				if strings.HasPrefix(clean, "  ┌") ||
					strings.HasPrefix(clean, "  │") ||
					strings.HasPrefix(clean, "  ├") ||
					strings.HasPrefix(clean, "  └") {
					visLen := len([]rune(clean))
					if visLen != expectedWidth {
						t.Fatalf("[%s] Line %d has visible length %d, expected %d.\nLine: %q\nClean: %q",
							sym, lineIdx+1, visLen, expectedWidth, line, clean)
					}
				}
			}
		})
	}
}

func TestRenderDashboardEmptyHistoryAlignment(t *testing.T) {
	expectedWidth := 96
	state := &MarketState{
		symbol:    "OANDA:XAUUSD",
		interval:  "15",
		statusMsg: "Menunggu data...",
		precision: 2,
	}

	output := renderDashboardString(state, 10, nil)
	lines := strings.Split(output, "\n")

	for lineIdx, line := range lines {
		clean := stripANSI(line)
		if strings.HasPrefix(clean, "  ┌") ||
			strings.HasPrefix(clean, "  │") ||
			strings.HasPrefix(clean, "  ├") ||
			strings.HasPrefix(clean, "  └") {
			visLen := len([]rune(clean))
			if visLen != expectedWidth {
				t.Fatalf("Line %d has visible length %d, expected %d.\nLine: %q\nClean: %q",
					lineIdx+1, visLen, expectedWidth, line, clean)
			}
		}
	}
}
