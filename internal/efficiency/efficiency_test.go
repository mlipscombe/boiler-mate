package efficiency

import (
	"strings"
	"testing"
	"time"
)

type trace struct {
	analyzer *Analyzer
	at       time.Time
	sample   Sample
}

func newTrace() *trace {
	return &trace{
		analyzer: NewAnalyzer(), at: time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC),
		sample: Sample{
			Session: 1, Suitable: true, Zones: "Office",
			Load: Load{Demand: "office", Target: 80, Minimum: 10, Maximum: 90,
				Proportional: 3.5, Integral: 0.3, ConfiguredGains: true},
		},
	}
}

func (tr *trace) add(state int, water, output float64) {
	tr.at = tr.at.Add(5 * time.Second)
	tr.sample.At, tr.sample.State = tr.at, state
	tr.sample.Water, tr.sample.Output = water, output
	tr.analyzer.Observe(tr.sample)
}

func (tr *trace) hold(duration time.Duration, water, output float64) {
	for elapsed := time.Duration(0); elapsed < duration; elapsed += 5 * time.Second {
		tr.add(StateFiring, water, output)
	}
}

func (tr *trace) shortRun(output float64, during func(int)) {
	tr.add(2, 70, 0)
	for tick := 0; tick < 96; tick++ {
		if during != nil {
			during(tick)
		}
		tr.add(StateFiring, 70+float64(tick)*0.19, output)
	}
	tr.add(stateTemperatureReached, 89, 0)
	tr.add(stateTemperatureReached, 89, 0)
}

func TestTemperatureStopAdviceSeparatesCappedAndMinimumOutput(t *testing.T) {
	for _, check := range []struct {
		rule   string
		output float64
	}{
		{"zone-output", 28},
		{"minimum-output", 10},
	} {
		t.Run(check.rule, func(t *testing.T) {
			tr := newTrace()
			tr.shortRun(check.output, nil)
			tr.shortRun(check.output, nil)
			if got := tr.analyzer.Report().Suggestions; len(got) != 0 {
				t.Fatalf("issued advice before three burns: %+v", got)
			}
			tr.shortRun(check.output, nil)
			report := tr.analyzer.Report()
			if got := report.Suggestions; len(got) != 1 || got[0].Rule != check.rule || got[0].Matches != 3 || got[0].Zones != "Office" {
				t.Fatalf("wrong suggestion: %+v", got)
			}
			if report.ObservedRuns != 3 {
				t.Errorf("counted %d firing runs, want 3", report.ObservedRuns)
			}
		})
	}
}

func TestTemperatureStopAdviceRequiresComparableContinuousRuns(t *testing.T) {
	tr := newTrace()
	tr.shortRun(35, nil)
	tr.shortRun(35, nil)
	tr.shortRun(35, func(tick int) {
		tr.sample.Suitable = tick != 30
	})
	tr.shortRun(35, func(tick int) {
		if tick == 50 {
			tr.sample.Connection++
		}
	})
	tr.shortRun(35, func(tick int) {
		if tick == 50 {
			tr.at = tr.at.Add(time.Minute)
		}
	})
	tr.shortRun(35, func(tick int) {
		if tick == 50 {
			tr.sample.Load.Demand = "office+upstairs"
		}
	})
	if got := tr.analyzer.Report().Suggestions; len(got) != 0 {
		t.Fatalf("combined interrupted or different loads: %+v", got)
	}
}

func TestPIAdviceRequiresRepeatedUnsaturatedWaves(t *testing.T) {
	for _, mode := range []string{"steady", "without diagnostics", "clipped"} {
		t.Run(mode, func(t *testing.T) {
			tr := newTrace()
			tr.add(2, 60, 0)
			tr.hold(15*time.Minute, 76, 45)
			for turn := 0; turn < 7; turn++ {
				water, output, contribution := 77.0, 50.0, 15.0
				if turn%2 == 1 {
					water, output, contribution = 83, 33, -15
				}
				if mode != "without diagnostics" {
					tr.sample.PI = &PIReading{Proportional: contribution, Integral: 18}
				}
				if mode == "clipped" && turn%2 == 1 {
					output = 10
				}
				tr.hold(2*time.Minute, water, output)
			}
			got := tr.analyzer.Report().Suggestions
			if mode != "steady" {
				if len(got) != 0 {
					t.Fatalf("incorrect PI advice: %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Rule != "pi-oscillation" ||
				!strings.Contains(got[0].Summary, "P 3.5 and I 0.3") || !strings.Contains(got[0].Review, "integral carryover") {
				t.Fatalf("expected measured PI advice, got %+v", got)
			}
		})
	}
}

func TestCeilingAdviceRequiresSeparateStalledRuns(t *testing.T) {
	tr := newTrace()
	tr.add(2, 55, 0)
	tr.hold(2*time.Hour, 70, 90)
	tr.add(14, 70, 0)
	tr.add(2, 55, 0)
	for minute := 0; minute < 50; minute++ {
		tr.hold(time.Minute, 62+float64(minute)/5, 90)
	}
	tr.add(14, 72, 0)
	if got := tr.analyzer.Report().Suggestions; len(got) != 0 {
		t.Fatalf("one stalled run and one improving run produced advice: %+v", got)
	}
	tr.add(2, 60, 0)
	tr.hold(46*time.Minute, 72, 90)
	got := tr.analyzer.Report().Suggestions
	if len(got) != 1 || got[0].Rule != "ceiling-shortfall" || got[0].Matches != 2 {
		t.Fatalf("expected ceiling advice for two stalled runs: %+v", got)
	}
}
