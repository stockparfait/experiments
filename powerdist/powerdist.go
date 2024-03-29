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
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/stockparfait/stats"
)

type statistic struct {
	c    *config.DistributionPlot
	f    func(stats.DistributionWithHistogram) float64 // compute the statistic
	name string
}

// PowerDist is an Experiment implementation for studying properties of
// analytical power and normal distributions.
type PowerDist struct {
	config   *config.PowerDist
	distName string
	source   stats.Distribution // true source distribution
	rand     stats.DistributionWithHistogram
}

var _ experiments.Experiment = &PowerDist{}

func (d *PowerDist) Prefix(s string) string {
	return experiments.Prefix(d.config.ID, s)
}

func (d *PowerDist) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, d.config.ID, k, v)
}

// distributionWithHistogram wraps analytical distribution into RandDistribution, as
// necessary.
func distributionWithHistogram(ctx context.Context, c *config.CompoundDistribution) (source stats.Distribution, dh stats.DistributionWithHistogram, name string, err error) {
	source, name, err = experiments.CompoundDistribution(ctx, c)
	if err != nil {
		err = errors.Annotate(err, "failed to create analytical distribution")
		return
	}
	var ok bool
	if dh, ok = source.(stats.DistributionWithHistogram); !ok {
		dh, err = experiments.Compound(ctx, source, 1, c.CompoundType, &c.Params)
		if err != nil {
			err = errors.Annotate(err, "failed to compound the source")
			return
		}
	}
	return
}

func (d *PowerDist) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	var err error
	if d.config, ok = cfg.(*config.PowerDist); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	d.source, d.rand, d.distName, err = distributionWithHistogram(ctx, &d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create RandDistribution")
	}
	if d.config.SamplePlot != nil {
		c := d.config.SamplePlot
		if err := experiments.PlotDistribution(ctx, d.rand, c, d.config.ID, d.distName); err != nil {
			return errors.Annotate(err, "failed to plot %s", d.distName)
		}
	}
	sts := []*statistic{}
	if d.config.MeanDist != nil {
		sts = append(sts, &statistic{
			c: d.config.MeanDist,
			f: func(dh stats.DistributionWithHistogram) float64 {
				return dh.Mean()
			},
			name: "means",
		})
	}
	if d.config.MADDist != nil {
		sts = append(sts, &statistic{
			c: d.config.MADDist,
			f: func(dh stats.DistributionWithHistogram) float64 {
				return dh.MAD()
			},
			name: "MADs",
		})
	}
	if d.config.SigmaDist != nil {
		sts = append(sts, &statistic{
			c: d.config.SigmaDist,
			f: func(dh stats.DistributionWithHistogram) float64 {
				return math.Sqrt(dh.Variance())
			},
			name: "Sigmas",
		})
	}
	if d.config.AlphaDist != nil {
		alphaFn := func() func(stats.DistributionWithHistogram) float64 {
			// Add an extra function closure to cache and hide these vars.
			mean := d.source.Mean()
			mad := d.source.MAD()
			return func(dh stats.DistributionWithHistogram) float64 {
				h := dh.Histogram()
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
		if d.config.Dist.AnalyticalSource != nil {
			cumulAlpha.SetExpected(d.config.Dist.AnalyticalSource.Alpha)
		}
	}
	if d.config.CumulSkew != nil {
		cumulSkew = experiments.NewCumulativeStatistic(d.config.CumulSkew)
	}
	if d.config.CumulKurt != nil {
		cumulKurt = experiments.NewCumulativeStatistic(d.config.CumulKurt)
	}

	cumulHist := stats.NewHistogram(&d.config.Dist.Params.Buckets)
	for i := 0; i < d.config.CumulSamples; i++ {
		y := d.rand.Rand()
		cumulMean.AddToAverage(y)
		var mean, mad float64
		if d.config.Dist.AnalyticalSource != nil {
			mean = d.config.Dist.AnalyticalSource.Mean
			mad = d.config.Dist.AnalyticalSource.MAD
		} else {
			mean = cumulHist.Mean()
			mad = cumulHist.MAD()
		}
		diff := y - mean
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
				mean,
				mad,
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

	if err := cumulMean.Plot(ctx, "mean", d.Prefix("mean")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative mean")
	}
	if err := cumulMAD.Plot(ctx, "MAD", d.Prefix("MAD")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative MAD")
	}
	if err := cumulSigma.Plot(ctx, "sigma", d.Prefix("sigma")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative sigma")
	}
	if err := cumulAlpha.Plot(ctx, "alpha", d.Prefix("alpha")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative alpha")
	}
	if err := cumulSkew.Plot(ctx, "skewness", d.Prefix("skewness")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative skewness")
	}
	if err := cumulKurt.Plot(ctx, "kurtosis", d.Prefix("kurtosis")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative kurtosis")
	}
	return nil
}

type interval struct {
	Start int
	End   int
}

type statsJobRes struct {
	samples [][]float64
	err     error
}

func (d *PowerDist) plotStatistics(ctx context.Context, sts []*statistic) error {
	if len(sts) == 0 {
		return nil
	}
	var dist stats.DistributionWithHistogram
	var distName string

	intervals := []interval{}
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
		intervals = append(intervals, interval{Start: start, End: end})
	}
	f := func(i interval) *statsJobRes {
		res := &statsJobRes{samples: make([][]float64, len(sts))}
		for k := i.Start; k < i.End; k++ {
			var err error
			// Create a fresh distribution every time. This is particularly important
			// for HistogramDistribution, as its histogram is always fixed.
			_, dist, distName, err = distributionWithHistogram(ctx, &d.config.Dist)
			if err != nil {
				res.err = errors.Annotate(err, "failed to create source distribution")
				return res
			}
			for j, s := range sts {
				res.samples[j] = append(res.samples[j], s.f(dist))
			}
		}
		return res
	}
	res := iterator.ParallelMapSlice(ctx, workers, intervals, f)

	samples := make([][]float64, len(sts))
	for i := 0; i < len(res); i++ {
		r := res[i]
		if r.err != nil {
			return errors.Annotate(r.err, "some jobs failed")
		}
		for i, s := range r.samples {
			samples[i] = append(samples[i], s...)
		}
	}
	for j, s := range sts {
		fullName := distName + " " + s.name
		dh := stats.NewSampleDistribution(samples[j], &s.c.Buckets)
		err := experiments.PlotDistribution(ctx, dh, s.c, d.config.ID, fullName)
		if err != nil {
			return errors.Annotate(err, "failed to plot %s", d.Prefix(fullName))
		}
	}
	return nil
}
