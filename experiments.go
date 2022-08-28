// Copyright 2022 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package experiments

import (
	"context"
	"fmt"
	"math"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

type contextKey int

const (
	valuesContextKey contextKey = iota
)

type Values = map[string]string

// UseValues injects the values map into the context. It is intended to be used
// by Experiments to add key:value pairs to be printed on the terminal at the
// end of the run.
func UseValues(ctx context.Context, v Values) context.Context {
	return context.WithValue(ctx, valuesContextKey, v)
}

// GetValues previously injected by UseValues, or nil.
func GetValues(ctx context.Context) Values {
	v, ok := ctx.Value(valuesContextKey).(Values)
	if !ok {
		return nil
	}
	return v
}

// AddValue adds (or overwrites) a key:value pair to the Values in the context.
// These pairs are intended to be printed on the terminal at the end of the run
// of all the experiments.
func AddValue(ctx context.Context, key, value string) error {
	v := GetValues(ctx)
	if v == nil {
		return errors.Reason("no values map in context")
	}
	v[key] = value
	return nil
}

// Experiment is a generic experiment interface. Each implementation is expected
// to add key:value pairs using AddValue, plots using plot.AddLeft/AddRight, or
// save data in files.
type Experiment interface {
	Run(ctx context.Context, cfg config.ExperimentConfig) error
}

// maybeSkipZeros removes (x, y) elements where y < 1e-300, if so configured.
// Strictly speaking, we're trying to avoid zeros, but in practice anything
// below this number may be printed or interpreted as 0 in plots.
func maybeSkipZeros(xs, ys []float64, c *config.DistributionPlot) ([]float64, []float64) {
	if len(xs) != len(ys) {
		panic(errors.Reason("len(xs) [%d] != len(ys) [%d]", len(xs), len(ys)))
	}
	if c.KeepZeros {
		return xs, ys
	}
	xs1 := []float64{}
	ys1 := []float64{}
	for i, x := range xs {
		if ys[i] >= 1.0e-300 {
			xs1 = append(xs1, x)
			ys1 = append(ys1, ys[i])
		}
	}
	return xs1, ys1
}

// maybeLog10 computes log10 for the slice of values if LogY is true.
func maybeLog10(ys []float64, c *config.DistributionPlot) []float64 {
	if !c.LogY {
		return ys
	}
	res := make([]float64, len(ys))
	for i, y := range ys {
		res[i] = math.Log10(y)
	}
	return res
}

// filterXY optionally skips zeros and computes log10 if configured.
func filterXY(xs, ys []float64, c *config.DistributionPlot) ([]float64, []float64) {
	xs, ys = maybeSkipZeros(xs, ys, c)
	ys = maybeLog10(ys, c)
	return xs, ys
}

// minMax returns the min and max values from ys.
func minMax(ys []float64) (float64, float64) {
	min := math.Inf(1)
	max := math.Inf(-1)
	for _, y := range ys {
		if y < min {
			min = y
		}
		if y > max {
			max = y
		}
	}
	return min, max
}

func PlotDistribution(ctx context.Context, h *stats.Histogram, c *config.DistributionPlot, legend string) error {
	if c == nil {
		return nil
	}
	var xs0 []float64
	var ys []float64
	yLabel := "p.d.f."

	if c.UseMeans {
		xs0 = h.Xs()
	} else {
		xs0 = h.Buckets().Xs(0.5)
	}

	ys = h.PDFs()
	xs, ys := filterXY(xs0, ys, c)
	min, max := minMax(ys)
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetLegend(legend + " " + yLabel)
	if c.LogY {
		yLabel = "log10(" + yLabel + ")"
	}
	plt.SetYLabel(yLabel)
	if c.ChartType == "bars" {
		plt.SetChartType(plot.ChartBars)
	}
	plt.SetLeftAxis(c.LeftAxis)
	if err := plot.Add(ctx, plt, c.Graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	if err := plotCounts(ctx, h, xs0, c, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s counts'", legend)
	}
	if c.PlotMean {
		if err := plotMean(ctx, h, c.Graph, min, max, legend); err != nil {
			return errors.Annotate(err, "failed to plot '%s mean'", legend)
		}
	}
	if err := plotPercentiles(ctx, h, c, min, max, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s percentiles'", legend)
	}
	if err := plotAnalytical(ctx, h, c, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s ref dist'", legend)
	}
	return nil
}

func plotCounts(ctx context.Context, h *stats.Histogram, xs []float64, c *config.DistributionPlot, legend string) error {
	if c.CountsGraph == "" {
		return nil
	}
	cs := make([]float64, len(h.Counts()))
	for i, y := range h.Counts() {
		cs[i] = float64(y)
	}
	xs, cs = maybeSkipZeros(xs, cs, c)
	plt, err := plot.NewXYPlot(xs, cs)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s counts'", legend)
	}
	plt.SetLegend(legend + " counts").SetYLabel("counts")
	plt.SetLeftAxis(c.CountsLeftAxis)
	if c.ChartType == "bars" {
		plt.SetChartType(plot.ChartBars)
	}
	if err := plot.Add(ctx, plt, c.CountsGraph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s counts'", legend)
	}
	return nil
}

func plotMean(ctx context.Context, h *stats.Histogram, graph string, min, max float64, legend string) error {
	x := h.Mean()
	plt, err := plot.NewXYPlot([]float64{x, x}, []float64{min, max})
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s mean'", legend)
	}
	plt.SetLegend(fmt.Sprintf("%s mean=%.4g", legend, x))
	plt.SetYLabel("").SetChartType(plot.ChartDashed)
	if err := plot.Add(ctx, plt, graph); err != nil {
		return errors.Annotate(err, "failed to add '%s mean' plot", legend)
	}
	return nil
}

func plotPercentiles(ctx context.Context, h *stats.Histogram, c *config.DistributionPlot, min, max float64, legend string) error {
	for _, p := range c.Percentiles {
		x := h.Quantile(p / 100.0)
		plt, err := plot.NewXYPlot([]float64{x, x}, []float64{min, max})
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s %gth %%-ile'",
				legend, p)
		}
		plt.SetLegend(fmt.Sprintf("%s %gth %%-ile=%.3g", legend, p, x))
		plt.SetYLabel("").SetChartType(plot.ChartDashed)
		if err := plot.Add(ctx, plt, c.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s %gth %%-ile'", legend, p)
		}
	}
	return nil
}

// distributionDistance computes a measure between the sample distribution given
// by h and an analytical distribution d in xs points corresponding to h's
// buckets, ignoring the points with less than ignoreCounts counts in h.
func distributionDistance(xs []float64, h *stats.Histogram, d stats.Distribution, ignoreCounts int) float64 {
	var res float64
	if ignoreCounts < 0 {
		ignoreCounts = 0
	}
	for i, x := range xs {
		if h.Count(i) <= uint(ignoreCounts) {
			continue
		}
		m := math.Abs(math.Log(h.PDF(i)) - math.Log(d.Prob(x)))
		if m > res {
			res = m
		}
	}
	return res
}

// FindMin is a generic search for a function minimum within [min..max]
// interval. Stop when the search interval is less than epsilon, or the number
// of iterations exceeds maxIter.
//
// For correction functionality assumes min < max, epsilon > 0, maxIter >= 1.
func FindMin(f func(float64) float64, min, max, epsilon float64, maxIter int) float64 {
	for i := 0; i < maxIter && (max-min) > epsilon; i++ {
		d := (max - min) / 2.1
		m1 := min + d
		m2 := max - d
		if f(m1) < f(m2) {
			max = m2
		} else {
			min = m1
		}
	}
	return (max + min) / 2.0
}

// AnalyticalDistribution instantiated from the corresponding config.
func AnalyticalDistribution(c *config.AnalyticalDistribution) (dist stats.Distribution, distName string, err error) {
	switch c.Name {
	case "t":
		dist = stats.NewStudentsTDistribution(c.Alpha, c.Mean, c.MAD)
		distName = fmt.Sprintf("T(a=%.2f)", c.Alpha)
	case "normal":
		dist = stats.NewNormalDistribution(c.Mean, c.MAD)
		distName = "Gauss"
	default:
		err = errors.Reason("unsuppoted distribution type: '%s'", c.Name)
		return
	}
	if c.Compound > 1 {
		xform := func(d stats.Distribution) float64 {
			acc := 0.0
			for i := 0; i < c.Compound; i++ {
				acc += d.Rand()
			}
			if c.Normalize {
				acc /= float64(c.Compound)
			}
			return acc
		}
		dist = stats.NewRandDistribution(dist, xform, c.Samples, &c.Buckets)
		distName += fmt.Sprintf(" x %d", c.Compound)
	}
	return
}

func plotAnalytical(ctx context.Context, h *stats.Histogram, c *config.DistributionPlot, legend string) error {
	if c.RefDist == nil {
		return nil
	}
	dc := *c.RefDist // shallow copy, to modify locally
	if c.AdjustRef {
		dc.Mean = h.Mean()
		dc.MAD = h.MAD()
	}
	var xs []float64
	if c.UseMeans {
		xs = h.Xs()
	} else {
		xs = h.Buckets().Xs(0.5)
	}
	if c.DeriveAlpha != nil {
		f := func(alpha float64) float64 {
			d := stats.NewStudentsTDistribution(alpha, dc.Mean, dc.MAD)
			return distributionDistance(xs, h, d, c.IgnoreCounts)
		}
		m := c.DeriveAlpha
		dc.Alpha = FindMin(f, m.MinX, m.MaxX, m.Epsilon, m.MaxIterations)
		if err := AddValue(ctx, legend+" alpha", fmt.Sprintf("%.4g", dc.Alpha)); err != nil {
			return errors.Annotate(err, "failed to add value for '%s alpha'", legend)
		}
	}
	dist, distName, err := AnalyticalDistribution(&dc)
	if err != nil {
		return errors.Annotate(err, "failed to instantiate reference distribution")
	}
	ys := make([]float64, len(xs))
	for i, x := range xs {
		ys[i] = dist.Prob(x)
	}
	xs, ys = filterXY(xs, ys, c)
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create '%s' analytical plot", legend)
	}
	plt.SetLegend(legend + " ref:" + distName).SetChartType(plot.ChartDashed)
	if c.LogY {
		plt.SetYLabel("log10(p.d.f.)")
	} else {
		plt.SetYLabel("p.d.f.")
	}
	if err := plot.Add(ctx, plt, c.Graph); err != nil {
		return errors.Annotate(err, "failed to add '%s' analytical plot", legend)
	}
	return nil
}

// TestExperiment is a fake experiment used in tests. Define actual experiments
// in their own subpackages.
type TestExperiment struct {
	cfg *config.TestExperimentConfig
}

var _ Experiment = &TestExperiment{}

// Run implements Experiment.
func (t *TestExperiment) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	t.cfg, ok = cfg.(*config.TestExperimentConfig)
	if !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	if err := AddValue(ctx, "grade", fmt.Sprintf("%g", t.cfg.Grade)); err != nil {
		return errors.Annotate(err, "cannot add grade value")
	}
	passed := "failed"
	if t.cfg.Passed {
		passed = "passed"
	}
	if err := AddValue(ctx, "test", passed); err != nil {
		return errors.Annotate(err, "cannot add pass/fail value")
	}
	p, err := plot.NewXYPlot([]float64{1.0, 2.0}, []float64{21.5, 42.0})
	if err != nil {
		return errors.Annotate(err, "failed to create XY plot")
	}
	if err := plot.Add(ctx, p, t.cfg.Graph); err != nil {
		return errors.Annotate(err, "cannot add plot")
	}
	return nil
}
