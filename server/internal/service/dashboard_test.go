package service

import "testing"

func TestNormalizeDatasetChartType(t *testing.T) {
	cases := map[string]string{
		"":            "bar",
		"line":        "line",
		"area":        "area",
		"bar_h":       "bar_h",
		"bar_stacked": "bar_stacked",
		"combo":       "combo",
		"scatter":     "scatter",
		"unknown":     "bar",
	}
	for input, want := range cases {
		if got := normalizeDatasetChartType(input); got != want {
			t.Fatalf("normalizeDatasetChartType(%q) = %q, want %q", input, got, want)
		}
	}
}
