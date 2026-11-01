package main

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPercentileIsNearestRank(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, test := range []struct{ p, want float64 }{{50, 5}, {90, 9}, {95, 10}, {99, 10}, {10, 1}, {0, 1}, {100, 10}} {
		if got := percentile(values, test.p); got != test.want {
			t.Errorf("p%g = %g, want %g", test.p, got, test.want)
		}
	}
	if percentile(nil, 50) != 0 {
		t.Error("empty input should give zero")
	}
}

func TestMedianMeanDeviation(t *testing.T) {
	if median([]float64{3, 1, 2}) != 2 || median([]float64{4, 1, 3, 2}) != 2.5 || median(nil) != 0 {
		t.Error("median")
	}
	if mean([]float64{1, 2, 3}) != 2 {
		t.Error("mean")
	}
	if !near(sampleStdDev([]float64{2, 4, 4, 4, 5, 5, 7, 9}), math.Sqrt(32.0/7)) || sampleStdDev([]float64{5}) != 0 {
		t.Error("standard deviation")
	}
}

func TestMedianIntervalNeedsSixSamples(t *testing.T) {
	if _, _, ok := medianInterval([]float64{1, 2, 3, 4, 5}); ok {
		t.Error("five samples cannot give a 95% interval")
	}
	low, high, ok := medianInterval([]float64{6, 1, 5, 2, 4, 3})
	if !ok || low != 1 || high != 6 {
		t.Errorf("six samples: [%g, %g] %v", low, high, ok)
	}
	// Ten samples: [x2, x9] covers 97.9%; [x3, x8] would cover only 89%.
	low, high, ok = medianInterval([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	if !ok || low != 2 || high != 9 {
		t.Errorf("ten samples: [%g, %g] %v", low, high, ok)
	}
}

func TestMannWhitneyExactValues(t *testing.T) {
	sequence := func(start, n float64) []float64 {
		var out []float64
		for i := 0.0; i < n; i++ {
			out = append(out, start+i)
		}
		return out
	}
	// Complete separation: two extreme arrangements out of C(n+m, n).
	for _, test := range []struct {
		n    float64
		want float64
	}{{3, 2.0 / 20}, {4, 2.0 / 70}, {5, 2.0 / 252}, {6, 2.0 / 924}} {
		if got := mannWhitneyP(sequence(0, test.n), sequence(100, test.n)); !near(got, test.want) {
			t.Errorf("n=%g: p = %g, want %g", test.n, got, test.want)
		}
	}
	if p := mannWhitneyP([]float64{1, 1, 1, 1}, []float64{1, 1, 1, 1}); p != 1 {
		t.Errorf("all tied: p = %g", p)
	}
	if p := mannWhitneyP(nil, []float64{1}); p != 1 {
		t.Errorf("empty side: p = %g", p)
	}
}

// bruteForceP enumerates every way of assigning the pooled values to the two
// sides, which is the definition the dynamic programme must reproduce.
func bruteForceP(a, b []float64) float64 {
	pool := append(append([]float64(nil), a...), b...)
	total := len(pool)
	ranks := make([]float64, total)
	for i, v := range pool {
		below, equal := 0.0, 0.0
		for _, other := range pool {
			if other < v {
				below++
			} else if other == v {
				equal++
			}
		}
		ranks[i] = below + (equal+1)/2
	}
	statistic := func(mask int) float64 {
		sum := 0.0
		for i := 0; i < total; i++ {
			if mask>>i&1 == 1 {
				sum += ranks[i]
			}
		}
		return sum
	}
	expected := float64(len(a)) * float64(total+1) / 2
	observed := statistic(1<<len(a) - 1)
	extreme, count := 0.0, 0.0
	for mask := 0; mask < 1<<total; mask++ {
		bits := 0
		for i := 0; i < total; i++ {
			bits += mask >> i & 1
		}
		if bits != len(a) {
			continue
		}
		count++
		if math.Abs(statistic(mask)-expected) >= math.Abs(observed-expected)-1e-9 {
			extreme++
		}
	}
	return extreme / count
}

func TestMannWhitneyMatchesEnumerationWithTies(t *testing.T) {
	cases := [][2][]float64{
		{{1, 2, 3, 4}, {3, 4, 5, 6}},
		{{1, 1, 2, 2, 3}, {2, 3, 3, 4, 4}},
		{{10, 10, 10, 11}, {10, 11, 12, 12, 12}},
		{{5, 5, 5, 5, 6}, {5, 5, 6, 6, 6}},
		{{1.5, 2.5, 2.5}, {2.5, 3.5, 4.5, 4.5}},
	}
	for _, test := range cases {
		want := bruteForceP(test[0], test[1])
		if got := mannWhitneyP(test[0], test[1]); !near(got, want) {
			t.Errorf("%v vs %v: p = %g, enumeration gives %g", test[0], test[1], got, want)
		}
	}
}
