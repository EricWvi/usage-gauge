package parser

import (
	"encoding/json"
	"testing"

	"usage-gauge/internal/types"
)

func TestClaudeQuotaWindows(t *testing.T) {
	t.Setenv("CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, body        string
		httpStatus, count int
		status            types.UsageStatus
	}{
		{"windows", `{"five_hour":{"utilization":0,"resets_at":"2026-09-23T20:00:00+08:00"},"seven_day":{"utilization":42.5},"seven_day_sonnet":{"utilization":10},"seven_day_opus":null}`, 200, 3, types.StatusOK},
		{"absent", `{"five_hour":null,"seven_day":null}`, 200, 0, types.StatusOK},
		{"invalid values", `{"five_hour":{"utilization":"42"},"seven_day":{"utilization":null}}`, 200, 0, types.StatusOK},
		{"extra", `{"extra_usage":{"is_enabled":true,"utilization":120}}`, 200, 1, types.StatusOK},
		{"array", `{"limits":[{"kind":"weekly_scoped","group":"weekly","percent":12,"scope":{"model":{"id":"fable","display_name":"Fable"}}},{"kind":"weekly","percent":5,"is_active":false}]}`, 200, 1, types.StatusOK},
		{"malformed", `{}`, 200, 0, types.StatusError},
		{"dual format", `{"five_hour":{"utilization":12},"seven_day":{"utilization":42},"seven_day_sonnet":{"utilization":5},"limits":[{"kind":"session","percent":12},{"kind":"weekly_all","percent":42},{"kind":"weekly_scoped","percent":5,"scope":{"model":{"id":"sonnet","display_name":"Sonnet"}}}]}`, 200, 3, types.StatusOK},
		{"array primary windows", `{"limits":[{"kind":"session","percent":12},{"kind":"weekly_all","percent":42}]}`, 200, 2, types.StatusOK},
		{"auth", `{"error":"login required"}`, 401, 0, types.StatusExpired},
		{"forbidden", `{}`, 403, 0, types.StatusExpired},
		{"limited", `{"error":"rate limited"}`, 429, 0, types.StatusError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			if err := json.Unmarshal([]byte(tc.body), &body); err != nil {
				t.Fatal(err)
			}
			got, err := New().Parse("claude", body, types.ParseContext{HTTPStatus: tc.httpStatus})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.status || len(got.Tiers) != tc.count {
				t.Fatalf("result: %+v", got)
			}
			if tc.name == "windows" && (got.Tiers[0].Utilization != 0 || got.Tiers[0].ResetsAt != "2026-09-23T12:00:00.000Z" || got.Tiers[1].Utilization != 42.5 || got.Tiers[1].Label != "weekly") {
				t.Fatalf("windows: %+v", got.Tiers)
			}
		})
	}
}
