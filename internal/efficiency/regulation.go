package efficiency

import (
	"fmt"
	"time"
)

type oscillation struct {
	since                 time.Time
	candidateSince        time.Time
	aboveSince            time.Time
	candidate             int
	side                  int
	turns                 int
	lowest, highest       float64
	lowOutput, highOutput float64
	lowP, highP           float64
	lowI, highI           float64
	integralLag           bool
}

func (a *Analyzer) checkOscillation(sample Sample) {
	if a.burn.oscillationReported {
		return
	}
	wave := &a.burn.oscillation
	deviation := sample.Water - sample.Load.Target
	if sample.PI == nil || sample.Output <= sample.Load.Minimum+powerMargin ||
		sample.Output >= sample.Load.Maximum-powerMargin || deviation < -8 || deviation > 8 {
		*wave = oscillation{}
		return
	}
	if wave.since.IsZero() || sample.At.Sub(wave.since) > 2*time.Hour {
		*wave = oscillation{
			since: sample.At, lowest: deviation, highest: deviation,
			lowOutput: sample.Output, highOutput: sample.Output,
			lowP: sample.PI.Proportional, highP: sample.PI.Proportional,
			lowI: sample.PI.Integral, highI: sample.PI.Integral,
		}
	}
	wave.lowest, wave.highest = min(wave.lowest, deviation), max(wave.highest, deviation)
	wave.lowOutput, wave.highOutput = min(wave.lowOutput, sample.Output), max(wave.highOutput, sample.Output)
	wave.lowP, wave.highP = min(wave.lowP, sample.PI.Proportional), max(wave.highP, sample.PI.Proportional)
	wave.lowI, wave.highI = min(wave.lowI, sample.PI.Integral), max(wave.highI, sample.PI.Integral)

	side := 0
	if deviation >= 2 {
		side = 1
	} else if deviation <= -2 {
		side = -1
	}
	if side != wave.candidate {
		wave.candidate, wave.candidateSince = side, sample.At
	}
	if side != 0 && side != wave.side && sample.At.Sub(wave.candidateSince) >= 30*time.Second {
		if wave.side != 0 {
			wave.turns++
		}
		wave.side = side
	}
	if deviation >= 2 && sample.PI.Integral >= 10 && sample.Output >= sample.Load.Minimum+10 {
		if wave.aboveSince.IsZero() {
			wave.aboveSince = sample.At
		}
		wave.integralLag = wave.integralLag || sample.At.Sub(wave.aboveSince) >= 90*time.Second
	} else {
		wave.aboveSince = time.Time{}
	}
	if wave.turns < 6 || wave.highOutput-wave.lowOutput < 10 ||
		max(wave.highP-wave.lowP, wave.highI-wave.lowI) < 5 {
		return
	}

	review := "Check circulation and look at how the proportional and integral contributions respond around each turn. Use comparable runs to review regulation with your installer."
	if wave.integralLag {
		review = "Output and the integral contribution stayed elevated after water passed the target. Review integral carryover and circulation with your installer before adjusting regulation."
	}
	a.burn.oscillationReported = true
	a.record(sample, "pi-oscillation", "Review temperature regulation",
		review, 1, 1, func(_ int) string {
			return fmt.Sprintf("Water temperature swung %.1f °C below to %.1f °C above target through %d complete oscillations with steady calls. Output moved between %.0f%% and %.0f%%. The P contribution varied by %.0f points and I by %.0f points. Configured gains are P %g and I %g.",
				-wave.lowest, wave.highest, wave.turns/2, wave.lowOutput, wave.highOutput,
				wave.highP-wave.lowP, wave.highI-wave.lowI,
				sample.Load.Proportional, sample.Load.Integral)
		})
}

type ceiling struct {
	since time.Time
	ticks []temperatureTick
}

type temperatureTick struct {
	at    time.Time
	water float64
}

func (a *Analyzer) checkCeiling(sample Sample) {
	burn := a.burn
	if burn.ceilingReported || !burn.knownStart {
		return
	}
	limit := &burn.ceiling
	if sample.Output < sample.Load.Maximum-powerMargin || sample.Output > sample.Load.Maximum+powerMargin ||
		sample.Load.Target-sample.Water < 5 {
		*limit = ceiling{}
		return
	}
	if limit.since.IsZero() {
		limit.since = sample.At
	}
	if len(limit.ticks) == 0 || sample.At.Sub(limit.ticks[len(limit.ticks)-1].at) >= time.Minute {
		limit.ticks = append(limit.ticks, temperatureTick{sample.At, sample.Water})
		if len(limit.ticks) > 15 {
			limit.ticks = limit.ticks[1:]
		}
	}
	if sample.At.Sub(limit.since) < 30*time.Minute || len(limit.ticks) < 15 {
		return
	}
	from := limit.ticks[0]
	if sample.Water-from.water >= 1 {
		return
	}
	burn.ceilingReported = true
	a.record(sample, "ceiling-shortfall", "Check the active output limit",
		"Check the configured maximum and any outdoor power band. If the limit is intentional, review whether the boiler and distribution system can deliver enough heat for these zones.",
		2, 1, func(count int) string {
			return fmt.Sprintf("In %d burns, the boiler spent at least 30 minutes near its %.0f%% maximum while water remained at least 5 °C below target. In the latest burn it gained less than 1 °C over the final %.0f minutes.",
				count, sample.Load.Maximum, sample.At.Sub(from.at).Minutes())
		})
}
