// Copyright 2023 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package autocorr is an experiment with auto-correlation of log-profit series.
package autocorr

import (
	"context"
	"fmt"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

type AutoCorrelation struct {
	config  *config.AutoCorrelation
	context context.Context
}

var _ experiments.Experiment = &AutoCorrelation{}

func (e *AutoCorrelation) Prefix(s string) string {
	return experiments.Prefix(e.config.ID, s)
}

func (e *AutoCorrelation) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, e.config.ID, k, v)
}

func (e *AutoCorrelation) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if e.config, ok = cfg.(*config.AutoCorrelation); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	e.context = ctx
	it, err := experiments.SourceMap(ctx, e.config.Data, e.processLogProfits)
	if err != nil {
		return errors.Annotate(err, "failed to process data")
	}
	defer it.Close()

	f := func(j1, j2 *jobResult) *jobResult { return j1.Merge(j2) }
	total := iterator.Reduce[*jobResult, *jobResult](it, e.newJobResult(), f)
	if err := e.processTotal(total); err != nil {
		return errors.Annotate(err, "failed to process final tally")
	}
	return nil
}

type jobResult struct {
	sums       []float64 // sums of X[i] * X[i+shift] for the range of shifts
	ns         []int     // number of samples for each sum
	numTickers int
}

func (e *AutoCorrelation) newJobResult() *jobResult {
	return &jobResult{
		sums: make([]float64, e.config.MaxShift),
		ns:   make([]int, e.config.MaxShift),
	}
}

func (j *jobResult) Add(samples []float64, maxShift int) error {
	sample := stats.NewSample(samples)
	mean := sample.Mean()
	variance := sample.Variance()
	if variance == 0 {
		return errors.Reason("log-profits have zero variance")
	}
	j.numTickers++
	for i := 0; i < len(samples); i++ {
		for k := 0; k < maxShift; k++ {
			shift := k + 1
			if i+shift >= len(samples) {
				break
			}
			j.sums[k] += (samples[i] - mean) * (samples[i+shift] - mean) / variance
			j.ns[k]++
		}
	}
	return nil
}

func (j *jobResult) Merge(j2 *jobResult) *jobResult {
	if len(j.sums) != len(j2.sums) {
		panic(errors.Reason("jobResult: size=%d != size=%d",
			len(j.sums), len(j2.sums)))
	}
	for i := 0; i < len(j.sums); i++ {
		j.sums[i] += j2.sums[i]
		j.ns[i] += j2.ns[i]
	}
	j.numTickers += j2.numTickers
	return j
}

func (e *AutoCorrelation) processLogProfits(lps []experiments.LogProfits) *jobResult {
	res := e.newJobResult()
	for _, lp := range lps {
		if len(lp.Timeseries.Data()) < e.config.MaxShift+2 {
			logging.Warningf(e.context, "skipping %s, too few samples: %d",
				lp.Ticker, len(lp.Timeseries.Data()))
			continue
		}
		if err := res.Add(lp.Timeseries.Data(), e.config.MaxShift); err != nil {
			logging.Warningf(e.context, "skipping %s: %s", err.Error())
		}
	}
	return res
}

func (e *AutoCorrelation) addPlot(total *jobResult) error {
	xs := make([]float64, e.config.MaxShift)
	ys := make([]float64, e.config.MaxShift)
	for i := 0; i < e.config.MaxShift; i++ {
		xs[i] = float64(i + 1)
		if total.ns[i] != 0 {
			ys[i] = total.sums[i] / float64(total.ns[i])
		}
	}
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create auto-correlation plot")
	}
	legend := e.Prefix("Auto-correlation")
	plt.SetLegend(legend).SetYLabel("correlation")
	if err := plot.Add(e.context, plt, e.config.Graph); err != nil {
		return errors.Annotate(err, "failed to add '%s' plot", legend)
	}
	return nil
}

func (e *AutoCorrelation) processTotal(total *jobResult) error {
	err := e.AddValue(e.context, "tickers", fmt.Sprintf("%d", total.numTickers))
	if err != nil {
		return errors.Annotate(err, "failed to add value for number of tickers")
	}
	err = e.AddValue(e.context, "samples", fmt.Sprintf("%d", total.ns[0]))
	if err != nil {
		return errors.Annotate(err, "failed to add value for number of samples")
	}
	if err := e.addPlot(total); err != nil {
		return errors.Annotate(err, "failed to add correlation plot")
	}
	return nil
}
