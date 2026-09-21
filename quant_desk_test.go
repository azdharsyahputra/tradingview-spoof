package tvspoof

import (
	"testing"
	"time"
)

func TestNormalizeDeskSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "OANDA:XAUUSD"},
		{"xauusd", "OANDA:XAUUSD"},
		{"gold", "OANDA:XAUUSD"},
		{"btc", "BINANCE:BTCUSDT"},
		{"eurusd", "FX:EURUSD"},
		{"dxy", "TVC:DXY"},
		{"NASDAQ:AAPL", "NASDAQ:AAPL"},
	}

	for _, tc := range tests {
		got := NormalizeDeskSymbol(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeDeskSymbol(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestDeterminePrecisionForSymbol(t *testing.T) {
	if p := DeterminePrecisionForSymbol("OANDA:XAUUSD"); p != 2 {
		t.Errorf("expected precision 2 for gold, got %d", p)
	}
	if p := DeterminePrecisionForSymbol("FX:EURUSD"); p != 5 {
		t.Errorf("expected precision 5 for forex, got %d", p)
	}
	if p := DeterminePrecisionForSymbol("FX:USDJPY"); p != 3 {
		t.Errorf("expected precision 3 for JPY, got %d", p)
	}
}

func TestCalculateVolumeProfile(t *testing.T) {
	bars := []Bar{
		{Time: 1000, Open: 100, High: 105, Low: 95, Close: 102, Volume: 500},
		{Time: 1060, Open: 102, High: 104, Low: 98, Close: 100, Volume: 1200},
		{Time: 1120, Open: 100, High: 108, Low: 100, Close: 107, Volume: 300},
	}

	vp := calculateVolumeProfile(bars, 2)
	if vp.TotalVolume != 2000 {
		t.Errorf("expected total volume 2000, got %f", vp.TotalVolume)
	}
	if vp.POC <= 0 || vp.VAH <= 0 || vp.VAL <= 0 {
		t.Errorf("invalid volume profile values: POC=%f, VAH=%f, VAL=%f", vp.POC, vp.VAH, vp.VAL)
	}
	if vp.VAL > vp.POC || vp.POC > vp.VAH {
		t.Errorf("inconsistent Value Area: VAL (%f) <= POC (%f) <= VAH (%f) violated", vp.VAL, vp.POC, vp.VAH)
	}
}

func TestCalculateATR(t *testing.T) {
	bars := []Bar{
		{Time: 1, Open: 10, High: 15, Low: 8, Close: 12},
		{Time: 2, Open: 12, High: 16, Low: 11, Close: 14},
		{Time: 3, Open: 14, High: 18, Low: 13, Close: 17},
	}
	atr := calculateATR(bars, 3)
	if atr <= 0 {
		t.Errorf("expected positive ATR, got %f", atr)
	}
}

func TestCalculateSessionLiquidity(t *testing.T) {
	now := time.Now().UTC()
	today0100 := time.Date(now.Year(), now.Month(), now.Day(), 1, 0, 0, 0, time.UTC).Unix()
	today0400 := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, time.UTC).Unix()

	bars15M := []Bar{
		{Time: today0100, Open: 4360, High: 4370, Low: 4355, Close: 4365},
		{Time: today0400, Open: 4365, High: 4368, Low: 4353, Close: 4360}, // sweeps 4355 low and closes 4360
	}
	barsD := []Bar{
		{Time: today0100 - 86400, Open: 4350, High: 4380, Low: 4340, Close: 4375},
		{Time: today0100, Open: 4375, High: 4378, Low: 4353, Close: 4360},
	}

	liq := calculateSessionLiquidity(bars15M, barsD, 4360)
	if liq.AsiaHigh != 4370 || liq.AsiaLow != 4353 {
		t.Errorf("unexpected Asia range: High=%f, Low=%f", liq.AsiaHigh, liq.AsiaLow)
	}
	if liq.PDH != 4380 || liq.PDL != 4340 {
		t.Errorf("unexpected PDH/PDL: PDH=%f, PDL=%f", liq.PDH, liq.PDL)
	}
}
