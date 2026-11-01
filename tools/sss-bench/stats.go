package main

import (
	"math"
	"slices"
)

// minSignificanceSamples is the smallest per-side sample that can reach p < 0.05
// in a two-sided Mann-Whitney test (4 vs 4 gives 2/70). benchstat uses the same
// floor. minIntervalSamples is the smallest that gives a 95% interval for a
// median from order statistics (6 gives 96.9%).
const (
	minSignificanceSamples = 4
	minIntervalSamples     = 6
	// noisyCV flags a metric whose repetitions spread more than this. It marks a
	// measurement for attention; it does not change a comparison's verdict.
	noisyCV = 0.15
)

// percentile is the nearest-rank percentile of sorted values. It always returns
// a value that was actually observed.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(values))
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// sampleStdDev is the sample standard deviation; zero for fewer than two values.
func sampleStdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m, sum := mean(values), 0.0
	for _, v := range values {
		sum += (v - m) * (v - m)
	}
	return math.Sqrt(sum / float64(len(values)-1))
}

// medianInterval is a distribution-free confidence interval for the median from
// order statistics, at 95% or better. ok is false below minIntervalSamples.
func medianInterval(values []float64) (low, high float64, ok bool) {
	n := len(values)
	if n < minIntervalSamples {
		return 0, 0, false
	}
	sorted := slices.Sorted(slices.Values(values))
	// P(median outside [x(k), x(n-k+1)]) is 2*P(Binomial(n, 1/2) < k). Take the
	// tightest k that keeps it at 5% or less.
	k, tail, term := 1, 0.0, math.Pow(0.5, float64(n))
	for i := 0; i < n/2; i++ {
		tail += term
		if 2*tail > 0.05 {
			break
		}
		k = i + 2
		term *= float64(n-i) / float64(i+1)
	}
	k = max(k-1, 1)
	return sorted[k-1], sorted[n-k], true
}

// minAchievableP is the smallest two-sided Mann-Whitney p-value these sample sizes
// can produce: complete separation, two arrangements out of C(n+m, n).
func minAchievableP(n, m int) float64 {
	arrangements := 1.0
	for i := 1; i <= n; i++ {
		arrangements *= float64(m+i) / float64(i)
	}
	return min(2/arrangements, 1)
}

// holmAdjust applies the Holm-Bonferroni correction, returning adjusted p-values in
// the input's order. Testing many metrics at 0.05 finds differences that are not
// there; Holm holds the chance of even one false finding in the family to alpha.
func holmAdjust(values []float64) []float64 {
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		switch {
		case values[a] < values[b]:
			return -1
		case values[a] > values[b]:
			return 1
		}
		return 0
	})
	adjusted := make([]float64, len(values))
	running := 0.0
	for rank, i := range order {
		running = max(running, min(1, float64(len(values)-rank)*values[i]))
		adjusted[i] = running
	}
	return adjusted
}

// mannWhitneyP is the two-sided exact p-value of the Mann-Whitney U test, the
// test benchstat uses. It makes no assumption about the shape of the
// distributions, which latency samples rarely satisfy, only that repetitions are
// independent: each one here is a fresh process.
//
// The null distribution is enumerated rather than approximated, by counting the
// subsets of the pooled ranks with each possible sum. Tied values take their
// mean rank, which keeps the test exact for repeated values such as memory
// counters. Ranks are doubled so that mean ranks stay integers.
func mannWhitneyP(a, b []float64) float64 {
	n, total := len(a), len(a)+len(b)
	if n == 0 || len(b) == 0 {
		return 1
	}
	type item struct {
		value float64
		first bool
	}
	pooled := make([]item, 0, total)
	for _, v := range a {
		pooled = append(pooled, item{v, true})
	}
	for _, v := range b {
		pooled = append(pooled, item{v, false})
	}
	slices.SortFunc(pooled, func(x, y item) int {
		switch {
		case x.value < y.value:
			return -1
		case x.value > y.value:
			return 1
		}
		return 0
	})
	ranks := make([]int, total)
	observed, sum := 0, 0
	for i := 0; i < total; {
		j := i
		for j+1 < total && pooled[j+1].value == pooled[i].value {
			j++
		}
		rank := i + j + 2 // doubled mean of the 1-based ranks i+1 to j+1
		for k := i; k <= j; k++ {
			ranks[k] = rank
			sum += rank
			if pooled[k].first {
				observed += rank
			}
		}
		i = j + 1
	}
	// ways[k][s]: how many k-element subsets of the ranks sum to s.
	ways := make([][]float64, n+1)
	for k := range ways {
		ways[k] = make([]float64, sum+1)
	}
	ways[0][0] = 1
	for _, rank := range ranks {
		for k := n; k >= 1; k-- {
			for s := sum; s >= rank; s-- {
				ways[k][s] += ways[k-1][s-rank]
			}
		}
	}
	expected := n * (total + 1) // doubled ranks
	distance := abs(observed - expected)
	extreme, all := 0.0, 0.0
	for s, count := range ways[n] {
		all += count
		if abs(s-expected) >= distance {
			extreme += count
		}
	}
	return min(extreme/all, 1)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
