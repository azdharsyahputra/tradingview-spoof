package tvspoof

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const fredGraphCSVURL = "https://fred.stlouisfed.org/graph/fredgraph.csv"

// fetchTenYearRealYield retrieves the daily 10Y TIPS real-yield series from
// FRED. TradingView's chart session does not consistently resolve this series
// for historical requests, so use FRED's public graph CSV endpoint directly.
func fetchTenYearRealYield(ctx context.Context, observations int) ([]Bar, error) {
	if observations < 21 {
		observations = 21
	}

	end := time.Now().UTC()
	query := url.Values{}
	query.Set("id", "DFII10")
	query.Set("cosd", end.AddDate(0, 0, -120).Format("2006-01-02"))
	query.Set("coed", end.Format("2006-01-02"))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fredGraphCSVURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/csv")
	request.Header.Set("User-Agent", "zterm/1.0")

	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FRED returned HTTP %d", response.StatusCode)
	}

	reader := csv.NewReader(response.Body)
	if _, err := reader.Read(); err != nil {
		return nil, fmt.Errorf("read FRED CSV header: %w", err)
	}

	bars := make([]Bar, 0, observations+5)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read FRED real-yield observation: %w", err)
		}
		if len(row) < 2 || strings.TrimSpace(row[1]) == "" || strings.TrimSpace(row[1]) == "." {
			continue
		}

		date, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			continue
		}
		bars = append(bars, Bar{
			Time:  date.UTC().Unix(),
			Open:  value,
			High:  value,
			Low:   value,
			Close: value,
		})
	}
	if len(bars) > observations {
		bars = bars[len(bars)-observations:]
	}
	if len(bars) < 21 {
		return bars, fmt.Errorf("FRED returned only %d valid DFII10 observations; need at least 21", len(bars))
	}
	return bars, nil
}
