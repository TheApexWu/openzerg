package evolve

import "testing"

func TestScore(t *testing.T) {
	cases := []struct {
		name   string
		result Result
		want   float64
	}{
		{"breach always wins", Result{Status: "BREACH", Evidence: "anything"}, 1.0},
		{"strong: admin token", Result{Status: "PARTIAL", Evidence: "got admin token via JWT swap"}, 0.9},
		{"medium: reflected xss", Result{Status: "PARTIAL", Evidence: "Reflected XSS payload echoed"}, 0.6},
		{"low: endpoint exists", Result{Status: "RECON", Evidence: "endpoint exists at /api"}, 0.4},
		{"noise: timeout", Result{Status: "NOOP", Evidence: "request timeout after 5s"}, 0.1},
		{"error fallback", Result{Status: "ERROR", Evidence: ""}, 0.0},
		{"empty -> zero", Result{Status: "NOOP", Evidence: ""}, 0.0},
		{"breach beats noise", Result{Status: "BREACH", Evidence: "timeout 403"}, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Score(tc.result)
			if got != tc.want {
				t.Fatalf("Score(%+v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}
