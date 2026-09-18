package tvspoof

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ForexFactoryCalendarURL is the weekly JSON export linked by Forex Factory's
// calendar page. The feed is provided by Fair Economy, the calendar publisher.
const ForexFactoryCalendarURL = "https://nfs.faireconomy.media/ff_calendar_thisweek.json"

type CalendarEvent struct {
	Title           string   `json:"title"`
	Country         string   `json:"country"`
	Date            string   `json:"date"`
	Timestamp       int64    `json:"timestamp"`
	Impact          string   `json:"impact"`
	Actual          string   `json:"actual,omitempty"`
	Forecast        string   `json:"forecast,omitempty"`
	Previous        string   `json:"previous,omitempty"`
	Unit            string   `json:"unit,omitempty"`
	IndicatorType   string   `json:"indicator_type,omitempty"`
	Direction       string   `json:"direction,omitempty"`
	CurrencyEffect  string   `json:"currency_effect,omitempty"`
	Surprise        *float64 `json:"surprise,omitempty"`
	FundamentalBias string   `json:"fundamental_bias,omitempty"`
}

type forexFactoryEvent struct {
	Title    string `json:"title"`
	Country  string `json:"country"`
	Date     string `json:"date"`
	Impact   string `json:"impact"`
	Actual   string `json:"actual"`
	Forecast string `json:"forecast"`
	Previous string `json:"previous"`
}

// FetchForexFactoryCalendar fetches and normalizes the current weekly feed.
func FetchForexFactoryCalendar(ctx context.Context) ([]CalendarEvent, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, ForexFactoryCalendarURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "tvspoof-local/1.0")

	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("forex factory returned HTTP %d", response.StatusCode)
	}

	var raw []forexFactoryEvent
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode forex factory calendar: %w", err)
	}

	events := make([]CalendarEvent, 0, len(raw))
	for _, item := range raw {
		if event, ok := normalizeForexFactoryEvent(item); ok {
			events = append(events, event)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp < events[j].Timestamp })
	return events, nil
}

type calendarMetadata struct {
	Unit          string
	IndicatorType string
	Direction     string
}

func normalizeForexFactoryEvent(item forexFactoryEvent) (CalendarEvent, bool) {
	date, err := time.Parse(time.RFC3339, item.Date)
	if err != nil {
		return CalendarEvent{}, false
	}

	metadata := classifyCalendarEvent(item.Title)
	event := CalendarEvent{
		Title:          item.Title,
		Country:        item.Country,
		Date:           item.Date,
		Timestamp:      date.Unix(),
		Impact:         item.Impact,
		Actual:         item.Actual,
		Forecast:       item.Forecast,
		Previous:       item.Previous,
		Unit:           metadata.Unit,
		IndicatorType:  metadata.IndicatorType,
		Direction:      metadata.Direction,
		CurrencyEffect: item.Country,
	}

	if event.Unit == "" {
		event.Unit = firstEconomicUnit(item.Actual, item.Forecast, item.Previous)
	}
	if surprise, unit, ok := economicSurprise(item.Actual, item.Forecast); ok {
		event.Surprise = &surprise
		if event.Unit == "" {
			event.Unit = unit
		}
		event.FundamentalBias = fundamentalBias(surprise, event.Direction)
	}

	return event, true
}

func classifyCalendarEvent(title string) calendarMetadata {
	name := strings.ToLower(title)
	metadata := calendarMetadata{IndicatorType: "other", Direction: "context_dependent"}

	switch {
	case strings.Contains(name, "unemployment"), strings.Contains(name, "jobless claims"), strings.Contains(name, "initial claims"), strings.Contains(name, "continuing claims"):
		metadata.IndicatorType = "labor"
		metadata.Direction = "lower_is_stronger"
	case strings.Contains(name, "non-farm"), strings.Contains(name, "nonfarm"), strings.Contains(name, "payroll"), strings.Contains(name, "employment"), strings.Contains(name, "adp"):
		metadata.IndicatorType = "labor"
		metadata.Direction = "higher_is_stronger"
	case strings.Contains(name, "cpi"), strings.Contains(name, "pce"), strings.Contains(name, "inflation"):
		metadata.IndicatorType = "inflation"
		metadata.Direction = "higher_is_stronger"
	case strings.Contains(name, "interest rate"), strings.Contains(name, "rate decision"), strings.Contains(name, "fomc"), strings.Contains(name, "fed "), strings.Contains(name, "ecb"), strings.Contains(name, "boe"), strings.Contains(name, "boj"), strings.Contains(name, "rba"), strings.Contains(name, "rbnz"), strings.Contains(name, "snb"):
		metadata.IndicatorType = "rates"
		metadata.Direction = "higher_is_stronger"
	case strings.Contains(name, "gdp"), strings.Contains(name, "pmi"), strings.Contains(name, "ism"), strings.Contains(name, "retail sales"), strings.Contains(name, "industrial production"), strings.Contains(name, "durable goods"), strings.Contains(name, "consumer confidence"), strings.Contains(name, "consumer sentiment"):
		metadata.IndicatorType = "growth"
		metadata.Direction = "higher_is_stronger"
	case strings.Contains(name, "trade balance"):
		metadata.IndicatorType = "trade"
	}

	return metadata
}

func economicSurprise(actual, forecast string) (float64, string, bool) {
	actualValue, actualUnit, actualOK := parseEconomicValue(actual)
	forecastValue, forecastUnit, forecastOK := parseEconomicValue(forecast)
	if !actualOK || !forecastOK || (actualUnit != "" && forecastUnit != "" && actualUnit != forecastUnit) {
		return 0, "", false
	}
	unit := actualUnit
	if unit == "" {
		unit = forecastUnit
	}
	return actualValue - forecastValue, unit, true
}

func parseEconomicValue(raw string) (float64, string, bool) {
	value := strings.TrimSpace(strings.ToUpper(raw))
	if value == "" || value == "N/A" || value == "NA" || value == "-" || strings.ContainsAny(value, "<>") {
		return 0, "", false
	}

	unit := ""
	if strings.HasSuffix(value, "%") {
		unit = "%"
		value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	} else if suffix := value[len(value)-1:]; suffix == "K" || suffix == "M" || suffix == "B" {
		unit = suffix
		value = strings.TrimSpace(value[:len(value)-1])
	}
	value = strings.ReplaceAll(value, ",", "")
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, "", false
	}
	return number, unit, true
}

func firstEconomicUnit(values ...string) string {
	for _, value := range values {
		if _, unit, ok := parseEconomicValue(value); ok && unit != "" {
			return unit
		}
	}
	return ""
}

func fundamentalBias(surprise float64, direction string) string {
	if surprise == 0 || direction == "context_dependent" {
		return "neutral"
	}
	positive := surprise > 0
	if direction == "lower_is_stronger" {
		positive = !positive
	}
	if positive {
		return "bullish"
	}
	return "bearish"
}
