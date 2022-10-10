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

// Values is a key:value map populated by implementations of Experiment to be
// printed on the terminal at the end of the run. It is typically used to print
// various values of interest not suitable for graphical plots.
type Values = map[string]string

// UseValues injects Values into the context, to be used by AddValue.
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
func AddValue(ctx context.Context, key, value string) error {
	v := GetValues(ctx)
	if v == nil {
		return errors.Reason("no values map in context")
	}
	v[key] = value
	return nil
}

// Experiment is a generic interface for a single experiment.
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

// PlotDistribution dh, specifically its p.d.f. as approximated by
// dh.Histogram(), and related plots according to the config c.
func PlotDistribution(ctx context.Context, dh stats.DistributionWithHistogram, c *config.DistributionPlot, legend string) error {
	if c == nil {
		return nil
	}
	var xs0 []float64
	var ys []float64
	yLabel := "p.d.f."

	h := dh.Histogram()

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
	if err := plotErrors(ctx, h, xs0, c, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s errors'", legend)
	}
	if c.PlotMean {
		if err := plotMean(ctx, dh, c.Graph, min, max, legend); err != nil {
			return errors.Annotate(err, "failed to plot '%s mean'", legend)
		}
	}
	if err := plotPercentiles(ctx, dh, c, min, max, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s percentiles'", legend)
	}
	if err := plotAnalytical(ctx, dh, c, legend); err != nil {
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

func plotErrors(ctx context.Context, h *stats.Histogram, xs []float64, c *config.DistributionPlot, legend string) error {
	if c.ErrorsGraph == "" {
		return nil
	}
	n := h.Buckets().N
	es := make([]float64, n)
	for i, y := range h.StdErrors() {
		es[i] = y
	}
	xs, es = filterXY(xs, es, c)
	plt, err := plot.NewXYPlot(xs, es)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s errors'", legend)
	}
	plt.SetLegend(legend + " errors").SetYLabel("errors")
	if c.LogY {
		plt.SetYLabel("log10(errors)")
	}
	plt.SetLeftAxis(c.ErrorsLeftAxis)
	if c.ChartType == "bars" {
		plt.SetChartType(plot.ChartBars)
	}
	if err := plot.Add(ctx, plt, c.ErrorsGraph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s errors'", legend)
	}
	return nil
}

func plotMean(ctx context.Context, dh stats.DistributionWithHistogram, graph string, min, max float64, legend string) error {
	x := dh.Mean()
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

func plotPercentiles(ctx context.Context, dh stats.DistributionWithHistogram, c *config.DistributionPlot, min, max float64, legend string) error {
	for _, p := range c.Percentiles {
		x := dh.Quantile(p / 100.0)
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

// DistributionDistance computes a measure between the sample distribution given
// by h and an analytical distribution d in xs points corresponding to h's
// buckets, ignoring the buckets with less than ignoreCounts samples. The
// leftmost and rightmost buckets are always ignored, as they are catch-all
// buckets and may not accurately represent the p.d.f. value.
func DistributionDistance(h *stats.Histogram, d stats.Distribution, ignoreCounts int) float64 {
	var res float64
	if ignoreCounts < 0 {
		ignoreCounts = 0
	}
	n := h.Buckets().N
	for i := 1; i < n-1; i++ {
		if h.Count(i) <= uint(ignoreCounts) {
			continue
		}
		m := math.Abs(math.Log(h.PDF(i)) - math.Log(d.Prob(h.X(i))))
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
// For correct functionality assumes min < max, epsilon > 0, maxIter >= 1, and f
// to be continuous and monotone around a single minimum in [min..max].
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

func directCompound(ctx context.Context, d stats.Distribution, n int, c *stats.ParallelSamplingConfig) stats.Distribution {
	fn := func(d stats.Distribution, s interface{}) (float64, interface{}) {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += d.Rand()
		}
		return acc, nil
	}
	xform := &stats.Transform{
		InitState: func() interface{} { return nil },
		Fn:        fn,
	}
	return stats.NewRandDistribution(ctx, d, xform, c)
}

func fastCompound(ctx context.Context, d stats.Distribution, n int, c *stats.ParallelSamplingConfig) stats.Distribution {
	fn := func(d stats.Distribution, state interface{}) (float64, interface{}) {
		sums := state.([]float64)
		if len(sums) > 0 {
			sums = sums[1:]
		}
		for len(sums) <= n {
			var last float64
			if len(sums) > 0 {
				last = sums[len(sums)-1]
			}
			sums = append(sums, last+d.Rand())
		}
		x := sums[n] - sums[0]
		return x, sums
	}
	xform := &stats.Transform{
		InitState: func() interface{} { return []float64{} },
		Fn:        fn,
	}
	return stats.NewRandDistribution(ctx, d, xform, c)
}

// AnalyticalDistribution instantiates a distribution from the corresponding
// config.
func AnalyticalDistribution(ctx context.Context, c *config.AnalyticalDistribution) (dist stats.Distribution, distName string, err error) {
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
	if c.SourceSamples > 0 {
		if c.Seed > 0 {
			dist.Seed(uint64(c.Seed))
		}
		dist = stats.NewSampleDistributionFromRand(
			dist, c.SourceSamples, &c.DistConfig.Buckets)
		distName += fmt.Sprintf("[samples=%d]", c.SourceSamples)
	}
	if c.Compound == 1 {
		return
	}
	switch c.CompoundType {
	case "direct":
		dist = directCompound(ctx, dist, c.Compound, &c.DistConfig)
	case "fast":
		dist = fastCompound(ctx, dist, c.Compound, &c.DistConfig)
	case "biased":
		h := stats.CompoundHistogram(ctx, dist, c.Compound, &c.DistConfig)
		dist = stats.NewHistogramDistribution(h)
	default:
		err = errors.Reason("unsupported compound type: %s", c.CompoundType)
		return
	}
	distName += fmt.Sprintf(" x %d", c.Compound)
	return
}

// DeriveAlpha estimates the degrees of freedom parameter for a Student's T
// distribution with the given mean and MAD that most closely corresponds to the
// sample distribution given as a histogram h.
func DeriveAlpha(h *stats.Histogram, mean, MAD float64, c *config.DeriveAlpha) float64 {
	f := func(alpha float64) float64 {
		d := stats.NewStudentsTDistribution(alpha, mean, MAD)
		return DistributionDistance(h, d, c.IgnoreCounts)
	}
	return FindMin(f, c.MinX, c.MaxX, c.Epsilon, c.MaxIterations)
}

func plotAnalytical(ctx context.Context, dh stats.DistributionWithHistogram, c *config.DistributionPlot, legend string) error {
	if c.RefDist == nil {
		return nil
	}
	dc := *c.RefDist // shallow copy, to modify locally
	if c.AdjustRef {
		dc.Mean = dh.Mean()
		dc.MAD = dh.MAD()
	}

	h := dh.Histogram()
	var xs []float64
	if c.UseMeans {
		xs = h.Xs()
	} else {
		xs = h.Buckets().Xs(0.5)
	}
	if c.DeriveAlpha != nil {
		dc.Alpha = DeriveAlpha(h, dc.Mean, dc.MAD, c.DeriveAlpha)
	}

	if err := AddValue(ctx, legend+" mean", fmt.Sprintf("%.4g", dc.Mean)); err != nil {
		return errors.Annotate(err, "failed to add value for '%s mean'", legend)
	}
	if err := AddValue(ctx, legend+" MAD", fmt.Sprintf("%.4g", dc.MAD)); err != nil {
		return errors.Annotate(err, "failed to add value for '%s MAD'", legend)
	}
	if dc.Name == "t" {
		if err := AddValue(ctx, legend+" alpha", fmt.Sprintf("%.4g", dc.Alpha)); err != nil {
			return errors.Annotate(err, "failed to add value for '%s alpha'", legend)
		}
	}
	dist, distName, err := AnalyticalDistribution(ctx, &dc)
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

// CumulativeStatistic tracks the value of a statistic as more samples
// arrive. It is intended to be plotted as a graph of the statistic as a
// function of the number of samples.
//
// The idea is to evaluate visually the noisiness of the statistic as the number
// of samples increase.
type CumulativeStatistic struct {
	config      *config.CumulativeStatistic
	h           *stats.Histogram
	i           int
	numPoints   int
	sum         float64
	Xs          []float64
	Ys          []float64
	Percentiles [][]float64
	Expected    float64 // expected value of the statistic
	nextPoint   int
}

// NewCumulativeStatistic initializes an empty CumulativeStatistic object.
func NewCumulativeStatistic(cfg *config.CumulativeStatistic) *CumulativeStatistic {
	return &CumulativeStatistic{
		config:      cfg,
		Percentiles: make([][]float64, len(cfg.Percentiles)),
		h:           stats.NewHistogram(&cfg.Buckets),
	}
}

func (c *CumulativeStatistic) point(i int) int {
	logSamples := math.Log(float64(c.config.Samples))
	x := logSamples * float64(i+1) / float64(c.config.Points)
	return int(math.Floor(math.Exp(x)))
}

// AddDirect adds y as the direct value of the statistic at the next sample. The
// caller is responsible for computing the statistic from the current and all of
// the preceding samples.
func (c *CumulativeStatistic) AddDirect(y float64) {
	if c == nil {
		return
	}
	if c.i < c.config.Skip {
		c.Skip()
		return
	}
	c.i++
	c.h.Add(y)
	if c.i >= c.nextPoint {
		c.Xs = append(c.Xs, float64(c.i))
		c.Ys = append(c.Ys, y)
		c.numPoints++
		c.nextPoint = c.point(c.numPoints)
		for i, p := range c.config.Percentiles {
			c.Percentiles[i] = append(c.Percentiles[i], c.h.Quantile(p/100.0))
		}
	}
}

// AddToAverage updates a statistic computed as the average of y(x) values. This
// is useful e.g. for tracking a mean.
func (c *CumulativeStatistic) AddToAverage(y float64) {
	if c == nil {
		return
	}
	c.sum += y
	avg := c.sum / float64(c.i+1)
	c.AddDirect(avg)
}

// Skip the next sample from the statistic, but advance the sample and point
// counters.
func (c *CumulativeStatistic) Skip() {
	c.i++
	if c.i >= c.nextPoint {
		c.numPoints++
		c.nextPoint = c.point(c.numPoints)
	}
}

// SetExpected value of the statistic, for visual reference on the graph.
func (c *CumulativeStatistic) SetExpected(y float64) {
	if c == nil {
		return
	}
	c.Expected = y
}

// Map applies f to all the resulting point values (the statistic and its
// percentiles).
//
// This is useful e.g. for the standard deviation: accumulate variance as the
// average of (y - mean)^2, and compute the square root using Map.
func (c *CumulativeStatistic) Map(f func(float64) float64) {
	if c == nil {
		return
	}
	for i, v := range c.Ys {
		c.Ys[i] = f(v)
		for p := range c.Percentiles {
			c.Percentiles[p][i] = f(c.Percentiles[p][i])
		}
	}
}

// Plot the accumulated statistic values, percentiles and the expected value, as
// configured.
func (c *CumulativeStatistic) Plot(ctx context.Context, yLabel, legend string) error {
	if c == nil {
		return nil
	}
	plt, err := plot.NewXYPlot(c.Xs, c.Ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetLegend(legend).SetYLabel(yLabel)
	if err := plot.Add(ctx, plt, c.config.Graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	for i, p := range c.config.Percentiles {
		pLegend := fmt.Sprintf("%s %.3g-th %%-ile", legend, p)
		plt, err = plot.NewXYPlot(c.Xs, c.Percentiles[i])
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s'", pLegend)
		}
		plt.SetLegend(pLegend).SetYLabel(yLabel).SetChartType(plot.ChartDashed)
		if err := plot.Add(ctx, plt, c.config.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s'", pLegend)
		}
	}
	if c.config.PlotExpected {
		xs := []float64{c.Xs[0], c.Xs[len(c.Xs)-1]}
		ys := []float64{c.Expected, c.Expected}
		plt, err := plot.NewXYPlot(xs, ys)
		if err != nil {
			return errors.Annotate(err, "failed to add plot '%s expected'", legend)
		}
		eLegend := fmt.Sprintf("%s expected=%.4g", legend, c.Expected)
		plt.SetLegend(eLegend).SetYLabel(yLabel)
		plt.SetChartType(plot.ChartDashed)
		if err := plot.Add(ctx, plt, c.config.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s expected'", legend)
		}
	}
	return nil
}

// TestExperiment is a fake experiment used in tests. Define actual experiments
// in their own subpackages.
type TestExperiment struct {
	cfg *config.TestExperimentConfig
}

var _ Experiment = &TestExperiment{}

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
