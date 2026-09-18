package tvspoof

import (
	"math"
	"testing"
)

func TestNormalizeForexFactoryEventAddsFundamentalContext(t *testing.T) {
	event, ok := normalizeForexFactoryEvent(forexFactoryEvent{
		Title:    "CPI m/m",
		Country:  "USD",
		Date:     "2026-09-18T12:30:00Z",
		Impact:   "High",
		Actual:   "0.5%",
		Forecast: "0.3%",
		Previous: "0.2%",
	})
	if !ok {
		t.Fatal("expected valid event")
	}
	if event.IndicatorType != "inflation" || event.Direction != "higher_is_stronger" {
		t.Fatalf("unexpected metadata: %#v", event)
	}
	if event.CurrencyEffect != "USD" || event.Unit != "%" {
		t.Fatalf("unexpected currency or unit: %#v", event)
	}
	if event.Surprise == nil || math.Abs(*event.Surprise-0.2) > 1e-9 {
		t.Fatalf("unexpected surprise: %v", event.Surprise)
	}
	if event.FundamentalBias != "bullish" {
		t.Fatalf("unexpected bias: %q", event.FundamentalBias)
	}
}

func TestNormalizeForexFactoryEventReversesLowerIsStrongerIndicators(t *testing.T) {
	event, ok := normalizeForexFactoryEvent(forexFactoryEvent{
		Title:    "Unemployment Rate",
		Country:  "USD",
		Date:     "2026-09-18T12:30:00Z",
		Actual:   "5.0%",
		Forecast: "4.5%",
	})
	if !ok {
		t.Fatal("expected valid event")
	}
	if event.Direction != "lower_is_stronger" || event.FundamentalBias != "bearish" {
		t.Fatalf("unexpected unemployment interpretation: %#v", event)
	}
}

func TestNormalizeForexFactoryEventLeavesMissingActualWithoutSurprise(t *testing.T) {
	event, ok := normalizeForexFactoryEvent(forexFactoryEvent{
		Title:    "GDP q/q",
		Country:  "EUR",
		Date:     "2026-09-18T12:30:00Z",
		Forecast: "0.3%",
	})
	if !ok {
		t.Fatal("expected valid event")
	}
	if event.Unit != "%" {
		t.Fatalf("expected forecast unit, got %q", event.Unit)
	}
	if event.Surprise != nil || event.FundamentalBias != "" {
		t.Fatalf("expected no surprise before actual release: %#v", event)
	}
}
