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
	"math"
	"runtime"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/parallel"
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
	sts := []*statistic{}
	if d.config.MeanDist != nil {
		sts = append(sts, &statistic{
			c:    d.config.MeanDist,
			f:    func(d *stats.Histogram) float64 { return d.Mean() },
			name: "means",
		})
	}
	if d.config.MADDist != nil {
		sts = append(sts, &statistic{
			c:    d.config.MADDist,
			f:    func(d *stats.Histogram) float64 { return d.MAD() },
			name: "MADs",
		})
	}
	if d.config.SigmaDist != nil {
		sts = append(sts, &statistic{
			c:    d.config.SigmaDist,
			f:    func(d *stats.Histogram) float64 { return d.Sigma() },
			name: "Sigmas",
		})
	}
	if d.config.AlphaDist != nil {
		alphaFn := func() func(*stats.Histogram) float64 {
			// Add an extra function closure to cache and hide these vars.
			mean := d.source.Mean()
			mad := d.source.MAD()
			return func(h *stats.Histogram) float64 {
				return experiments.DeriveAlpha(h, mean, mad, d.config.AlphaParams)
			}
		}
		sts = append(sts, &statistic{
			c:    d.config.AlphaDist,
			f:    alphaFn(),
			name: "Alphas",
		})
	}
	if err := d.plotStatistics(ctx, sts); err != nil {
		return errors.Annotate(err, "failed to plot statistics distributions")
	}

	var cumulMean, cumulMAD *experiments.CumulativeStatistic
	var cumulSigma, cumulAlpha *experiments.CumulativeStatistic
	var cumulSkew, cumulKurt *experiments.CumulativeStatistic
	expectVariance := d.source.Variance()
	expectSigma := math.Sqrt(expectVariance)
	if d.config.CumulMean != nil {
		cumulMean = experiments.NewCumulativeStatistic(d.config.CumulMean)
		cumulMean.SetExpected(d.source.Mean())
	}
	if d.config.CumulMAD != nil {
		cumulMAD = experiments.NewCumulativeStatistic(d.config.CumulMAD)
		cumulMAD.SetExpected(d.source.MAD())
	}
	if d.config.CumulSigma != nil {
		cumulSigma = experiments.NewCumulativeStatistic(d.config.CumulSigma)
		cumulSigma.SetExpected(expectSigma)
	}
	if d.config.CumulAlpha != nil {
		cumulAlpha = experiments.NewCumulativeStatistic(d.config.CumulAlpha)
		cumulAlpha.SetExpected(d.config.Dist.Alpha)
	}
	if d.config.CumulSkew != nil {
		cumulSkew = experiments.NewCumulativeStatistic(d.config.CumulSkew)
	}
	if d.config.CumulKurt != nil {
		cumulKurt = experiments.NewCumulativeStatistic(d.config.CumulKurt)
	}

	cumulHist := stats.NewHistogram(&d.config.Dist.Buckets)
	for i := 0; i < d.config.CumulSamples; i++ {
		y := d.rand.Rand()
		cumulMean.AddToAverage(y)
		diff := y - d.config.Dist.Mean
		cumulMAD.AddToAverage(math.Abs(diff))
		dd := diff * diff
		cumulSigma.AddToAverage(dd)
		cumulSkew.AddToAverage(dd * diff)
		cumulKurt.AddToAverage(dd * dd)
		cumulHist.Add(y)
		// Deriving alpha is expensive, skip if not needed.
		if cumulAlpha != nil {
			cumulAlpha.AddDirect(experiments.DeriveAlpha(
				cumulHist,
				d.config.Dist.Mean,
				d.config.Dist.MAD,
				d.config.AlphaParams,
			))
		}
	}
	cumulSigma.Map(func(y float64) float64 {
		if y < 0.0 {
			y = 0
		}
		return math.Sqrt(y)
	})

	cumulSkew.Map(func(y float64) float64 {
		return y / (expectVariance * expectSigma)
	})

	cumulKurt.Map(func(y float64) float64 {
		return y / (expectVariance * expectVariance)
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
	if err := cumulSkew.Plot(ctx, "skewness", d.prefix("skewness")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative skewness")
	}
	if err := cumulKurt.Plot(ctx, "kurtosis", d.prefix("kurtosis")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative kurtosis")
	}
	return nil
}

type statistic struct {
	c    *config.DistributionPlot
	f    func(*stats.Histogram) float64 // compute the statistic
	name string
}

func (d *PowerDist) plotStatistics(ctx context.Context, sts []*statistic) error {
	if len(sts) == 0 {
		return nil
	}
	// Do NOT directly compute dist.Histogram() or statistics that require it, so
	// that copies would have to compute it every time.
	_, dist, distName, err := randDistribution(ctx, &d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create source distribution")
	}
	hs := make([]*stats.Histogram, len(sts))
	for j := 0; j < len(sts); j++ {
		hs[j] = stats.NewHistogram(&sts[j].c.Buckets)
	}

	jobs := []parallel.Job{}
	workers := 2 * runtime.NumCPU()
	step := d.config.StatSamples / workers
	if step < 1 {
		step = 1
	}
	for i := 0; i < d.config.StatSamples; i += step {
		start := i
		end := start + step
		if end > d.config.StatSamples {
			end = d.config.StatSamples
		}
		jobs = append(jobs, func() interface{} {
			hs := make([]*stats.Histogram, len(sts))
			for j := 0; j < len(sts); j++ {
				hs[j] = stats.NewHistogram(&sts[j].c.Buckets)
			}
			for k := start; k < end; k++ {
				h := dist.Copy().(*stats.RandDistribution).Histogram()
				for j, s := range sts {
					hs[j].Add(s.f(h))
				}
			}
			return hs
		})
	}
	res := parallel.MapSlice(ctx, workers, jobs)
	for i := 0; i < len(res); i++ {
		hr := res[i].([]*stats.Histogram)
		for i, h := range hr {
			hs[i].AddHistogram(h)
		}
	}
	// for i := 0; i < d.config.StatSamples; i++ {
	// 	h := dist.Copy().(*stats.RandDistribution).Histogram()
	// 	for j, s := range sts {
	// 		hs[j].Add(s.f(h))
	// 	}
	// }
	for j, s := range sts {
		fullName := d.prefix(distName + " " + s.name)
		err := experiments.PlotDistribution(ctx, hs[j], s.c, fullName)
		if err != nil {
			return errors.Annotate(err, "failed to plot %s", fullName)
		}
	}
	return nil
}
