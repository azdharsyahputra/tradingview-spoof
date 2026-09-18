package tvspoof

import "testing"

func TestExtractHistoryBars(t *testing.T) {
	result := map[int64]Bar{}
	extractHistoryBars([]interface{}{"cs_1", map[string]interface{}{
		"s": []interface{}{
			map[string]interface{}{"i": float64(0), "v": []interface{}{float64(1700000060), 2.0, 3.0, 1.0, 2.5, 10.0}},
			map[string]interface{}{"i": float64(1), "v": []interface{}{float64(1700000000), 1.0, 2.0, 0.5, 2.0, 8.0}},
		},
	}}, result)

	bars := sortedBars(result)
	if len(bars) != 2 || bars[0].Time != 1700000000 || bars[1].Close != 2.5 {
		t.Fatalf("unexpected bars: %#v", bars)
	}
}

func TestDecodeTVMessage(t *testing.T) {
	method, payload, err := decodeTVMessage(`{"m":"series_completed","p":["cs_1","s1",""]}`)
	if err != nil || method != "series_completed" || len(payload) != 3 {
		t.Fatalf("unexpected message: %s %#v %v", method, payload, err)
	}
}
