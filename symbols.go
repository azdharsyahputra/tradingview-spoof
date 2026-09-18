package tvspoof

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const symbolSearchURL = "https://symbol-search.tradingview.com/symbol_search/v3/"

// SymbolSearchResult is a normalized TradingView symbol-search item.
type SymbolSearchResult struct {
	Symbol       string
	FullName     string
	Description  string
	Exchange     string
	Type         string
	CurrencyCode string
	Country      string
}

// SearchSymbols queries TradingView's public symbol search using a
// Chrome-like uTLS connection. It returns metadata only and never uses an
// account session or credential.
func SearchSymbols(ctx context.Context, query string, exchange string, limit int) ([]SymbolSearchResult, error) {
	query = strings.TrimSpace(query)
	if len(query) < 1 || len(query) > 100 {
		return nil, fmt.Errorf("query must contain 1 through 100 characters")
	}
	if limit < 1 || limit > 50 {
		return nil, fmt.Errorf("limit must be between 1 and 50")
	}

	endpoint, err := url.Parse(symbolSearchURL)
	if err != nil {
		return nil, err
	}
	params := endpoint.Query()
	params.Set("text", query)
	params.Set("hl", "1")
	params.Set("lang", "en")
	params.Set("search_type", "undefined")
	params.Set("domain", "production")
	if strings.TrimSpace(exchange) != "" {
		params.Set("exchange", strings.TrimSpace(exchange))
	}
	endpoint.RawQuery = params.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json, text/plain, */*")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("Origin", defaultOrigin)
	request.Header.Set("Referer", defaultOrigin+"/")
	request.Header.Set("User-Agent", defaultUserAgent)

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialTLSContext:    customUTLSDialerHTTP,
			ForceAttemptHTTP2: false,
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("TradingView symbol search failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, fmt.Errorf("TradingView symbol search returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read TradingView symbol search: %w", err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		var envelope map[string]json.RawMessage
		if objectErr := json.Unmarshal(body, &envelope); objectErr != nil {
			return nil, fmt.Errorf("decode TradingView symbol search: %w", err)
		}
		symbols, found := envelope["symbols"]
		if !found {
			return nil, fmt.Errorf("decode TradingView symbol search: response has no symbols field")
		}
		if err := json.Unmarshal(symbols, &raw); err != nil {
			return nil, fmt.Errorf("decode TradingView symbol search symbols: %w", err)
		}
	}
	results := make([]SymbolSearchResult, 0, min(limit, len(raw)))
	for _, item := range raw {
		if len(results) >= limit {
			break
		}
		symbol := searchString(item, "symbol")
		exchangeName := searchString(item, "exchange")
		fullName := searchString(item, "full_name")
		if fullName == "" && exchangeName != "" && symbol != "" {
			fullName = exchangeName + ":" + symbol
		}
		results = append(results, SymbolSearchResult{
			Symbol:       cleanSearchText(symbol),
			FullName:     cleanSearchText(fullName),
			Description:  cleanSearchText(searchString(item, "description")),
			Exchange:     cleanSearchText(exchangeName),
			Type:         cleanSearchText(searchString(item, "type")),
			CurrencyCode: cleanSearchText(searchString(item, "currency_code")),
			Country:      cleanSearchText(searchString(item, "country")),
		})
	}
	return results, nil
}

func searchString(item map[string]interface{}, key string) string {
	value, _ := item[key].(string)
	return value
}

func cleanSearchText(value string) string {
	value = strings.ReplaceAll(value, "<em>", "")
	value = strings.ReplaceAll(value, "</em>", "")
	return strings.TrimSpace(value)
}

// ParseSearchLimit returns a safe symbol-search response size.
func ParseSearchLimit(value string) (int, error) {
	if value == "" {
		return 12, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	return limit, nil
}
