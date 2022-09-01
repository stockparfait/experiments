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
	samples       int
	points        int
	percentiles   []float64 // in [0..100]
	h             *stats.Histogram
	i             int
	sum           float64
	xs            []float64
	ys            []float64
	percentilesYs [][]float64
	nextPoint     int
}

func newCumulativeStatistic(samples, points int, percentiles []float64, buckets *stats.Buckets) *cumulativeStatistic {
	return &cumulativeStatistic{
		samples:       samples,
		points:        points,
		percentiles:   percentiles,
		percentilesYs: make([][]float64, len(percentiles)),
		h:             stats.NewHistogram(buckets),
	}
}

func (c *cumulativeStatistic) point(i int) int {
	max := math.Log(float64(c.samples))
	x := max * float64(i+1) / float64(c.points)
	return int(math.Floor(math.Exp(x)))
}

func (c *cumulativeStatistic) Add(y float64) {
	c.i++
	c.sum += y
	avg := c.sum / float64(c.i)
	c.h.Add(avg)
	if c.i >= c.nextPoint {
		c.xs = append(c.xs, float64(c.i))
		c.ys = append(c.ys, avg)
		c.nextPoint = c.point(len(c.xs))
		for i, p := range c.percentiles {
			c.percentilesYs[i] = append(c.percentilesYs[i], c.h.Quantile(p/100.0))
		}
	}
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

	meanBuckets, err := stats.NewBuckets(101, -10, 10, stats.LinearSpacing)
	if err != nil {
		return errors.Annotate(err, "failed to create mean buckets")
	}
	if d.config.MeanDist != nil {
		meanBuckets = &d.config.MeanDist.Buckets
	}
	cumulMean := newCumulativeStatistic(
		d.config.Samples, d.config.Points, d.config.Percentiles, meanBuckets)

	madBuckets, err := stats.NewBuckets(101, 0, 10, stats.LinearSpacing)
	if err != nil {
		return errors.Annotate(err, "failed to create MAD buckets")
	}
	if d.config.MADDist != nil {
		madBuckets = &d.config.MADDist.Buckets
	}
	cumulMAD := newCumulativeStatistic(
		d.config.Samples, d.config.Points, d.config.Percentiles, madBuckets)

	sigmaBuckets, err := stats.NewBuckets(101, 0, 10, stats.LinearSpacing)
	if err != nil {
		return errors.Annotate(err, "failed to create sigma buckets")
	}
	if d.config.SigmaDist != nil {
		sigmaBuckets = &d.config.SigmaDist.Buckets
	}
	cumulSigma := newCumulativeStatistic(
		d.config.Samples, d.config.Points, d.config.Percentiles, sigmaBuckets)

	for i := 0; i < d.config.Samples; i++ {
		y := d.dist.Rand()
		cumulMean.Add(y)
		diff := d.config.Dist.Mean - y
		cumulMAD.Add(math.Abs(diff))
		cumulSigma.Add(diff * diff)
	}
	for i, v := range cumulSigma.ys {
		cumulSigma.ys[i] = math.Sqrt(v)
		for p := range cumulSigma.percentilesYs {
			if cumulSigma.percentilesYs[p][i] < 0.0 {
				cumulSigma.percentilesYs[p][i] = 0.0
			}
			cumulSigma.percentilesYs[p][i] = math.Sqrt(cumulSigma.percentilesYs[p][i])
		}
	}

	if err := d.plotStatsSamples(ctx, d.config.CumulMeanGraph, "mean", cumulMean); err != nil {
		return errors.Annotate(err, "failed to plot cumulative mean")
	}
	if err := d.plotStatsSamples(ctx, d.config.CumulMADGraph, "MAD", cumulMAD); err != nil {
		return errors.Annotate(err, "failed to plot cumulative MAD")
	}
	if err := d.plotStatsSamples(ctx, d.config.CumulSigmaGraph, "sigma", cumulSigma); err != nil {
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
	statDist := stats.NewRandDistribution(dist, xform, d.config.Samples, &c.Buckets)
	h := statDist.Histogram()
	fullName := d.prefix(distName + " " + name)
	if err = experiments.PlotDistribution(ctx, h, c, fullName); err != nil {
		return errors.Annotate(err, "failed to plot %s", fullName)
	}
	return nil
}

func (d *PowerDist) plotStatsSamples(ctx context.Context, graph, name string, c *cumulativeStatistic) error {
	if graph == "" {
		return nil
	}
	legend := d.prefix(name)
	plt, err := plot.NewXYPlot(c.xs, c.ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetLegend(legend).SetYLabel(name)
	if err := plot.Add(ctx, plt, graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	for i, p := range d.config.Percentiles {
		pLegend := fmt.Sprintf("%s %.3g-th %%-ile", legend, p)
		plt, err = plot.NewXYPlot(c.xs, c.percentilesYs[i])
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s'", pLegend)
		}
		plt.SetLegend(pLegend).SetYLabel(name).SetChartType(plot.ChartDashed)
		if err := plot.Add(ctx, plt, graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s'", pLegend)
		}
	}
	return nil
}
