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
	dist     *stats.RandDistribution
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
func randDistribution(c *config.AnalyticalDistribution) (dist *stats.RandDistribution, name string, err error) {
	var d stats.Distribution
	d, name, err = experiments.AnalyticalDistribution(c)
	if err != nil {
		err = errors.Annotate(err, "failed to create analytical distribution")
		return
	}
	var ok bool
	if dist, ok = d.(*stats.RandDistribution); !ok {
		xform := func(d stats.Distribution) float64 {
			return d.Rand()
		}
		dist = stats.NewRandDistribution(d, xform, c.Samples, &c.Buckets)
	}
	return
}

type cumulativeStatistic struct {
	Graph         string
	samples       int
	points        int
	Percentiles   []float64 // in [0..100]
	h             *stats.Histogram
	i             int
	sum           float64
	Xs            []float64
	Ys            []float64
	PercentilesYs [][]float64
	nextPoint     int
}

func newCumulativeStatistic(graph string, samples, points int, percentiles []float64, buckets *stats.Buckets) *cumulativeStatistic {
	return &cumulativeStatistic{
		Graph:         graph,
		samples:       samples,
		points:        points,
		Percentiles:   percentiles,
		PercentilesYs: make([][]float64, len(percentiles)),
		h:             stats.NewHistogram(buckets),
	}
}

func (c *cumulativeStatistic) point(i int) int {
	logSamples := math.Log(float64(c.samples))
	x := logSamples * float64(i+1) / float64(c.points)
	return int(math.Floor(math.Exp(x)))
}

func (c *cumulativeStatistic) Add(y float64) {
	if c == nil {
		return
	}
	c.i++
	c.sum += y
	avg := c.sum / float64(c.i)
	c.h.Add(avg)
	if c.i >= c.nextPoint {
		c.Xs = append(c.Xs, float64(c.i))
		c.Ys = append(c.Ys, avg)
		c.nextPoint = c.point(len(c.Xs))
		for i, p := range c.Percentiles {
			c.PercentilesYs[i] = append(c.PercentilesYs[i], c.h.Quantile(p/100.0))
		}
	}
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

func (c *cumulativeStatistic) plotCumulativeStatistic(ctx context.Context, yLabel, legend string) error {
	if c == nil {
		return nil
	}
	plt, err := plot.NewXYPlot(c.Xs, c.Ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetLegend(legend).SetYLabel(yLabel)
	if err := plot.Add(ctx, plt, c.Graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	for i, p := range c.Percentiles {
		pLegend := fmt.Sprintf("%s %.3g-th %%-ile", legend, p)
		plt, err = plot.NewXYPlot(c.Xs, c.PercentilesYs[i])
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s'", pLegend)
		}
		plt.SetLegend(pLegend).SetYLabel(yLabel).SetChartType(plot.ChartDashed)
		if err := plot.Add(ctx, plt, c.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s'", pLegend)
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
	d.dist, d.distName, err = randDistribution(&d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create RandDistribution")
	}
	if d.config.SamplePlot != nil {
		h := d.dist.Histogram()
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

	var cumulMean, cumulMAD, cumulSigma *cumulativeStatistic
	if d.config.CumulMean != nil {
		c := d.config.CumulMean
		cumulMean = newCumulativeStatistic(
			c.Graph, c.Samples, c.Points, c.Percentiles, &c.Buckets)
	}
	if d.config.CumulMAD != nil {
		c := d.config.CumulMAD
		cumulMAD = newCumulativeStatistic(
			c.Graph, c.Samples, c.Points, c.Percentiles, &c.Buckets)
	}
	if d.config.CumulSigma != nil {
		c := d.config.CumulSigma
		cumulSigma = newCumulativeStatistic(
			c.Graph, c.Samples, c.Points, c.Percentiles, &c.Buckets)
	}

	for i := 0; i < d.config.CumulSamples; i++ {
		y := d.dist.Rand()
		cumulMean.Add(y)
		diff := d.config.Dist.Mean - y
		cumulMAD.Add(math.Abs(diff))
		cumulSigma.Add(diff * diff)
	}
	cumulSigma.Map(func(y float64) float64 {
		if y < 0.0 {
			y = 0
		}
		return math.Sqrt(y)
	})

	if err := cumulMean.plotCumulativeStatistic(ctx, "mean", d.prefix("mean")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative mean")
	}
	if err := cumulMAD.plotCumulativeStatistic(ctx, "MAD", d.prefix("MAD")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative MAD")
	}
	if err := cumulSigma.plotCumulativeStatistic(ctx, "sigma", d.prefix("sigma")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative sigma")
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
	dist, distName, err := randDistribution(&d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create source distribution")
	}
	statDist := stats.NewRandDistribution(dist, xform, d.config.StatSamples, &c.Buckets)
	h := statDist.Histogram()
	fullName := d.prefix(distName + " " + name)
	if err = experiments.PlotDistribution(ctx, h, c, fullName); err != nil {
		return errors.Annotate(err, "failed to plot %s", fullName)
	}
	return nil
}
