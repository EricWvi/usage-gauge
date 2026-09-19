package parser

import (
	"encoding/json"
	"testing"

	"usage-gauge/internal/types"
)

func TestCodexQuotaWindows(t *testing.T) {
	t.Setenv("CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, body    string
		status, count int
		want          types.UsageStatus
	}{
		{"legacy", `{"rateLimits":{"planType":"plus","primary":{"usedPercent":0,"windowDurationMins":300,"resetsAt":1800000000},"secondary":{"usedPercent":42,"windowDurationMins":10080}}}`, 200, 2, types.StatusOK},
		{"multi-bucket", `{"rateLimits":{"primary":{"usedPercent":1}},"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":2}},"review":{"secondary":{"usedPercent":3,"windowDurationMins":10080}}}}`, 200, 2, types.StatusOK},
		{"null windows", `{"rateLimits":{"primary":null,"secondary":null}}`, 200, 0, types.StatusOK},
		{"malformed", `{}`, 200, 0, types.StatusError},
		{"bridge error", `{"error":"login required"}`, 502, 0, types.StatusError},
		{"auth", `{}`, 401, 0, types.StatusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			if err := json.Unmarshal([]byte(tc.body), &body); err != nil {
				t.Fatal(err)
			}
			got, err := New().Parse("codex", body, types.ParseContext{HTTPStatus: tc.status})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want || len(got.Tiers) != tc.count {
				t.Fatalf("result: %+v", got)
			}
			if tc.name == "legacy" && (got.Tiers[0].Utilization != 0 || got.Tiers[0].ResetsAt != "2027-01-15T08:00:00.000Z" || got.Tiers[1].Label != "weekly") {
				t.Fatalf("windows: %+v", got.Tiers)
			}
		})
	}
}
