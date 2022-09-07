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

// Package powerdist is an experiment to study analytical power distributions.
package powerdist

import (
	"context"
	"fmt"
	"math"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// PowerDist is an Experiment implementation for studying properties of
// analytical power and normal distributions.
type PowerDist struct {
	config   *config.PowerDist
	distName string
	source   stats.Distribution // true source distribution
	rand     *stats.RandDistribution
}

var _ experiments.Experiment = &PowerDist{}

// prefix the experiment's ID to s, if there is one.
func (d *PowerDist) prefix(s string) string {
	if d.config.ID == "" {
		return s
	}
	return d.config.ID + " " + s
}

// randDistribution wraps analytical distribution into RandDistribution, as
// necessary.
func randDistribution(ctx context.Context, c *config.AnalyticalDistribution) (source stats.Distribution, rand *stats.RandDistribution, name string, err error) {
	source, name, err = experiments.AnalyticalDistribution(ctx, c)
	if err != nil {
		err = errors.Annotate(err, "failed to create analytical distribution")
		return
	}
	var ok bool
	if rand, ok = source.(*stats.RandDistribution); !ok {
		xform := func(d stats.Distribution) float64 {
			return d.Rand()
		}
		rand = stats.NewRandDistribution(ctx, source, xform, c.Samples, &c.Buckets)
	}
	return
}

type cumulativeStatistic struct {
	config        *config.CumulativeStatistic
	h             *stats.Histogram
	i             int
	numPoints     int
	sum           float64
	Xs            []float64
	Ys            []float64
	PercentilesYs [][]float64
	Expected      float64 // expected value of the statistic
	nextPoint     int
}

func newCumulativeStatistic(cfg *config.CumulativeStatistic) *cumulativeStatistic {
	return &cumulativeStatistic{
		config:        cfg,
		PercentilesYs: make([][]float64, len(cfg.Percentiles)),
		h:             stats.NewHistogram(&cfg.Buckets),
	}
}

func (c *cumulativeStatistic) point(i int) int {
	logSamples := math.Log(float64(c.config.Samples))
	x := logSamples * float64(i+1) / float64(c.config.Points)
	return int(math.Floor(math.Exp(x)))
}

func (c *cumulativeStatistic) AddDirect(y float64) {
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
			c.PercentilesYs[i] = append(c.PercentilesYs[i], c.h.Quantile(p/100.0))
		}
	}
}

func (c *cumulativeStatistic) AddToAverage(y float64) {
	if c == nil {
		return
	}
	c.sum += y
	avg := c.sum / float64(c.i+1)
	c.AddDirect(avg)
}

// Skip the next sample from the statistic, but advance the sample and point
// counters.
func (c *cumulativeStatistic) Skip() {
	c.i++
	if c.i >= c.nextPoint {
		c.numPoints++
		c.nextPoint = c.point(c.numPoints)
	}
}

func (c *cumulativeStatistic) SetExpected(y float64) {
	if c == nil {
		return
	}
	c.Expected = y
}

// Map applies f to all the resulting point values (averages and percentiles).
func (c *cumulativeStatistic) Map(f func(float64) float64) {
	if c == nil {
		return
	}
	for i, v := range c.Ys {
		c.Ys[i] = f(v)
		for p := range c.PercentilesYs {
			c.PercentilesYs[p][i] = f(c.PercentilesYs[p][i])
		}
	}
}

func (c *cumulativeStatistic) Plot(ctx context.Context, yLabel, legend string) error {
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
		plt, err = plot.NewXYPlot(c.Xs, c.PercentilesYs[i])
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

func (d *PowerDist) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	var err error
	if d.config, ok = cfg.(*config.PowerDist); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	d.source, d.rand, d.distName, err = randDistribution(ctx, &d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create RandDistribution")
	}
	if d.config.SamplePlot != nil {
		h := d.rand.Histogram()
		c := d.config.SamplePlot
		name := d.prefix(d.distName)
		if err := experiments.PlotDistribution(ctx, h, c, name); err != nil {
			return errors.Annotate(err, "failed to plot %s", d.distName)
		}
	}
	meanFn := func(d *stats.Histogram) float64 { return d.Mean() }
	if err := d.plotStatistic(ctx, d.config.MeanDist, meanFn, "means"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("means"))
	}
	madFn := func(d *stats.Histogram) float64 { return d.MAD() }
	if err := d.plotStatistic(ctx, d.config.MADDist, madFn, "MADs"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("MADs"))
	}
	sigmaFn := func(d *stats.Histogram) float64 { return d.Sigma() }
	if err := d.plotStatistic(ctx, d.config.SigmaDist, sigmaFn, "Sigmas"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("Sigmas"))
	}
	alphaFn := func() func(*stats.Histogram) float64 {
		// Add an extra function closure to cache and hide these vars.
		mean := d.source.Mean()
		mad := d.source.MAD()
		k := d.config.AlphaIgnoreCounts
		return func(h *stats.Histogram) float64 {
			return experiments.DeriveAlpha(h, mean, mad, d.config.AlphaParams, k)
		}
	}
	if err := d.plotStatistic(ctx, d.config.AlphaDist, alphaFn(), "Alphas"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("Alphas"))
	}

	var cumulMean, cumulMAD, cumulSigma, cumulAlpha *cumulativeStatistic
	if d.config.CumulMean != nil {
		cumulMean = newCumulativeStatistic(d.config.CumulMean)
		cumulMean.SetExpected(d.source.Mean())
	}
	if d.config.CumulMAD != nil {
		cumulMAD = newCumulativeStatistic(d.config.CumulMAD)
		cumulMAD.SetExpected(d.source.MAD())
	}
	if d.config.CumulSigma != nil {
		cumulSigma = newCumulativeStatistic(d.config.CumulSigma)
		cumulSigma.SetExpected(math.Sqrt(d.source.Variance()))
	}
	if d.config.CumulAlpha != nil {
		cumulAlpha = newCumulativeStatistic(d.config.CumulAlpha)
		cumulAlpha.SetExpected(d.config.Dist.Alpha)
	}

	cumulHist := stats.NewHistogram(&d.config.Dist.Buckets)
	for i := 0; i < d.config.CumulSamples; i++ {
		y := d.rand.Rand()
		cumulMean.AddToAverage(y)
		diff := d.config.Dist.Mean - y
		cumulMAD.AddToAverage(math.Abs(diff))
		cumulSigma.AddToAverage(diff * diff)
		cumulHist.Add(y)
		// Deriving alpha is expensive, skip if not needed.
		if cumulAlpha != nil {
			cumulAlpha.AddDirect(experiments.DeriveAlpha(
				cumulHist,
				d.config.Dist.Mean,
				d.config.Dist.MAD,
				d.config.AlphaParams,
				d.config.AlphaIgnoreCounts,
			))
		}
	}
	cumulSigma.Map(func(y float64) float64 {
		if y < 0.0 {
			y = 0
		}
		return math.Sqrt(y)
	})

	if err := cumulMean.Plot(ctx, "mean", d.prefix("mean")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative mean")
	}
	if err := cumulMAD.Plot(ctx, "MAD", d.prefix("MAD")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative MAD")
	}
	if err := cumulSigma.Plot(ctx, "sigma", d.prefix("sigma")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative sigma")
	}
	if err := cumulAlpha.Plot(ctx, "alpha", d.prefix("alpha")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative alpha")
	}
	return nil
}

func (d *PowerDist) plotStatistic(
	ctx context.Context,
	c *config.DistributionPlot,
	f func(*stats.Histogram) float64, // compute the statistic
	name string,
) (err error) {
	if c == nil {
		return nil
	}
	xform := func(d stats.Distribution) float64 {
		rd := d.Copy().(*stats.RandDistribution) // use a fresh copy to recompute the histogram
		return f(rd.Histogram())
	}
	// Do NOT directly compute dist.Histogram() or statistics that require it, so
	// that copies would have to compute it every time.
	_, dist, distName, err := randDistribution(ctx, &d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create source distribution")
	}
	statDist := stats.NewRandDistribution(ctx, dist, xform, d.config.StatSamples, &c.Buckets)
	h := statDist.Histogram()
	fullName := d.prefix(distName + " " + name)
	if err = experiments.PlotDistribution(ctx, h, c, fullName); err != nil {
		return errors.Annotate(err, "failed to plot %s", fullName)
	}
	return nil
}
