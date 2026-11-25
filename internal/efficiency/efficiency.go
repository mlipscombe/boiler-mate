// Package efficiency turns recent observations into read-only heating advice.
package efficiency

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

const Lookback = 7 * 24 * time.Hour

const (
	StateFiring             = 5
	stateTemperatureReached = 9
	maxSampleGap            = 15 * time.Second
	shortBurnLimit          = 20 * time.Minute
	tailLength              = 3 * time.Minute
	startupHold             = 15 * time.Minute
	powerMargin             = 2
)

type Sample struct {
	At         time.Time `json:"-"`
	Session    int64     `json:"session"`
	Connection uint64    `json:"connection,omitempty"`
	State      int       `json:"state"`
	// Suitable marks an ordinary space-heating interval with complete telemetry.
	Suitable bool       `json:"suitable,omitempty"`
	Load     Load       `json:"load"`
	Zones    string     `json:"zones,omitempty"`
	Water    float64    `json:"water,omitempty"`
	Output   float64    `json:"output,omitempty"`
	PI       *PIReading `json:"pi,omitempty"`
}

// Load combines the stable conditions used to compare time intervals.
type Load struct {
	Demand           string  `json:"demand,omitempty"`
	Target           float64 `json:"target,omitempty"`
	Minimum          float64 `json:"minimum,omitempty"`
	Maximum          float64 `json:"maximum,omitempty"`
	Ramp             float64 `json:"ramp,omitempty"`
	StopDifferential float64 `json:"stopDifferential,omitempty"`
	Proportional     float64 `json:"proportional,omitempty"`
	Integral         float64 `json:"integral,omitempty"`
	ConfiguredGains  bool    `json:"configuredGains,omitempty"`
}

type PIReading struct {
	Proportional float64 `json:"proportional"`
	Integral     float64 `json:"integral"`
}

type Report struct {
	FirstSample    *time.Time   `json:"firstSample"`
	LastSample     *time.Time   `json:"lastSample"`
	HeatDemandMins int          `json:"heatDemandMinutes"`
	ObservedRuns   int          `json:"observedRuns"`
	Suggestions    []Suggestion `json:"suggestions"`
}

type Suggestion struct {
	Rule    string    `json:"rule"`
	Title   string    `json:"title"`
	Zones   string    `json:"zones"`
	Latest  time.Time `json:"latest"`
	Matches int       `json:"matches"`
	Summary string    `json:"summary"`
	Review  string    `json:"review"`
}

type Analyzer struct {
	previous    Sample
	burn        *burn
	knownDemand time.Duration
	report      Report
	findings    map[findingKey]finding
}

type findingKey struct {
	rule string
	load Load
}

type finding struct {
	suggestion Suggestion
	required   int
	detail     string
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{findings: make(map[findingKey]finding)}
}

func (a *Analyzer) Observe(sample Sample) {
	if a.report.FirstSample == nil {
		a.report.FirstSample = &sample.At
	}
	a.report.LastSample = &sample.At
	previous := a.previous
	a.previous = sample
	elapsed := sample.At.Sub(previous.At)
	continuous := elapsed > 0 && elapsed <= maxSampleGap &&
		sample.Session == previous.Session && sample.Connection == previous.Connection &&
		sample.Suitable && previous.Suitable && sample.Load == previous.Load
	if continuous {
		a.knownDemand += elapsed
	}

	if sample.State != StateFiring || !sample.Suitable {
		if sample.State == stateTemperatureReached && continuous && a.burn != nil {
			a.checkTemperatureStop(sample)
		}
		a.burn = nil
		return
	}
	if !continuous || a.burn == nil {
		knownStart := continuous && (previous.State == stateTemperatureReached || previous.State >= 1 && previous.State <= 4)
		a.burn = &burn{start: sample.At, knownStart: knownStart}
		if knownStart {
			a.report.ObservedRuns++
		}
	}
	a.burn.appendTail(sample)
	if sample.At.Sub(a.burn.start) >= startupHold {
		a.checkOscillation(sample)
		a.checkCeiling(sample)
	}
}

func (a *Analyzer) Report() Report {
	report := a.report
	report.HeatDemandMins = int(a.knownDemand / time.Minute)
	report.Suggestions = make([]Suggestion, 0)
	type publishedKey struct{ rule, demand string }
	latest := make(map[publishedKey]finding)
	for key, current := range a.findings {
		if current.suggestion.Matches >= current.required {
			group := publishedKey{key.rule, key.load.Demand}
			if current.suggestion.Latest.After(latest[group].suggestion.Latest) {
				latest[group] = current
			}
		}
	}
	for _, current := range latest {
		suggestion := current.suggestion
		suggestion.Summary = current.detail
		report.Suggestions = append(report.Suggestions, suggestion)
	}
	sort.Slice(report.Suggestions, func(i, j int) bool {
		left, right := report.Suggestions[i], report.Suggestions[j]
		if !left.Latest.Equal(right.Latest) {
			return left.Latest.After(right.Latest)
		}
		if left.Rule != right.Rule {
			return left.Rule < right.Rule
		}
		return left.Zones < right.Zones
	})
	return report
}

func (a *Analyzer) record(sample Sample, rule, title, review string, required, matches int, detail func(int) string) {
	key := findingKey{rule, sample.Load}
	previous := a.findings[key].suggestion.Matches
	count := previous + matches
	a.findings[key] = finding{
		suggestion: Suggestion{Rule: rule, Title: title, Zones: sample.Zones, Latest: sample.At, Matches: count, Review: review},
		required:   required, detail: detail(count),
	}
}

type burn struct {
	start               time.Time
	knownStart          bool
	tail                []Sample
	oscillation         oscillation
	ceiling             ceiling
	oscillationReported bool
	ceilingReported     bool
}

func (b *burn) appendTail(sample Sample) {
	for len(b.tail) > 0 && sample.At.Sub(b.tail[0].At) > tailLength {
		b.tail = b.tail[1:]
	}
	b.tail = append(b.tail, sample)
}

func (a *Analyzer) checkTemperatureStop(stop Sample) {
	burn := a.burn
	length := stop.At.Sub(burn.start)
	if !burn.knownStart || length > shortBurnLimit || len(burn.tail) == 0 {
		return
	}
	first, last := burn.tail[0], burn.tail[len(burn.tail)-1]
	tail := last.At.Sub(first.At)
	if tail < 2*time.Minute {
		return
	}
	minimum := stop.Load.Minimum
	nearMinimum, aboveMinimum := 0, 0
	var output []float64
	for _, sample := range burn.tail {
		output = append(output, sample.Output)
		if math.Abs(sample.Output-minimum) <= powerMargin {
			nearMinimum++
		}
		if sample.Output >= minimum+5 {
			aboveMinimum++
		}
	}
	switch {
	case tail >= tailLength-maxSampleGap && nearMinimum == len(output) && last.Water-first.Water >= 1:
		a.record(stop, "minimum-output", "Check heat transfer at minimum output",
			"Check that the calling loops are open and circulating. If water keeps warming at minimum output, review the loop load and available buffer capacity with your installer.",
			3, 1, func(count int) string {
				return fmt.Sprintf("%d short burns reached the temperature stop while these zones were still calling. In the latest burn, water rose %.1f °C over %.1f minutes near the %.0f%% minimum.",
					count, last.Water-first.Water, tail.Minutes(), minimum)
			})
	case aboveMinimum*4 >= len(output)*3 && last.Output >= minimum+5:
		slices.Sort(output)
		median := output[len(output)/2]
		a.record(stop, "zone-output", "Try a lower output ceiling for these zones",
			"Consider a lower space-heating maximum for this combination of loops. Compare burn length and room temperature recovery during similar calls.",
			3, 1, func(count int) string {
				return fmt.Sprintf("%d short burns reached the temperature stop while these zones were still calling. The latest ran for %.1f minutes, with a median output of %.0f%% in its final %.1f minutes and a configured minimum of %.0f%%.",
					count, length.Minutes(), median, tail.Minutes(), minimum)
			})
	}
}
