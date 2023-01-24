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
	"runtime"

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

func (e *AutoCorrelation) prefix(s string) string {
	if e.config.ID == "" {
		return s
	}
	return e.config.ID + " " + s
}

func (e *AutoCorrelation) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if e.config, ok = cfg.(*config.AutoCorrelation); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	e.context = ctx
	if e.config.Reader != nil {
		if err := e.processPrices(); err != nil {
			return errors.Annotate(err, "failed to process price data")
		}
	}
	if e.config.Analytical != nil {
		if err := e.processAnalytical(); err != nil {
			return errors.Annotate(err, "failed to process synthetic data")
		}
	}
	return nil
}

func (e *AutoCorrelation) processPrices() error {
	tickers, err := e.config.Reader.Tickers(e.context)
	if err != nil {
		return errors.Annotate(err, "failed to list tickers")
	}
	if err := e.processTickers(tickers); err != nil {
		return errors.Annotate(err, "failed to process tickers")
	}
	return nil
}

type synthBatch struct {
	dist stats.Distribution // a copy of the Distribution
	n    int                // number of samples to generate in this batch
}

// synthIter is an iterator generating batch intervals and a Distribution clone.
type synthIter struct {
	dist      stats.Distribution // original distribution
	batchSize int
	n         int // number of samples to generate (sum of all intervals)
	i         int // number of samples generated so far
}

var _ iterator.Iterator[synthBatch] = &synthIter{}

func (it *synthIter) Next() (synthBatch, bool) {
	if it.i >= it.n {
		return synthBatch{}, false
	}
	n := it.batchSize
	it.i += n
	if it.i > it.n {
		n -= it.i - it.n
		it.i = it.n
	}
	batch := synthBatch{
		dist: it.dist.Copy(),
		n:    n,
	}
	return batch, true
}

func (e *AutoCorrelation) addPlot(total jobResult) error {
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
	legend := e.prefix("Auto-correlation")
	plt.SetLegend(legend).SetYLabel("correlation")
	if err := plot.Add(e.context, plt, e.config.Graph); err != nil {
		return errors.Annotate(err, "failed to add '%s' plot", legend)
	}
	return nil
}

func (e *AutoCorrelation) processAnalytical() error {
	dist, name, err := experiments.AnalyticalDistribution(e.context, e.config.Analytical)
	if err != nil {
		return errors.Annotate(err, "failed to instantiate analytical distribution")
	}
	it := &synthIter{
		dist:      dist,
		batchSize: e.config.BatchSize,
		n:         e.config.Samples,
	}
	f := func(batch synthBatch) jobResult {
		buf := make([]float64, batch.n)
		for i := 0; i < batch.n; i++ {
			buf[i] = batch.dist.Rand()
		}
		j := e.newJobResult(name)
		j.Add(stats.NewSample().Init(buf), e.config.MaxShift)
		return j
	}
	pm := iterator.ParallelMap[synthBatch, jobResult](e.context, 2*runtime.NumCPU(), it, f)
	total := e.newJobResult("total")
	for r, ok := pm.Next(); ok; r, ok = pm.Next() {
		if r.err != nil {
			continue
		}
		total.Merge(r)
	}
	err = experiments.AddValue(e.context, e.prefix("samples"), fmt.Sprintf("%d", total.ns[0]))
	if err != nil {
		return errors.Annotate(err, "failed to add value for number of samples")
	}
	if err := e.addPlot(total); err != nil {
		return errors.Annotate(err, "failed to add correlation plot")
	}
	return nil
}

type jobResult struct {
	ticker string
	sums   []float64 // sums of X[i] * X[i+shift] for the range of shifts
	ns     []int     // number of samples for each sum
	err    error
}

func (e *AutoCorrelation) newJobResult(ticker string) jobResult {
	return jobResult{
		ticker: ticker,
		sums:   make([]float64, e.config.MaxShift),
		ns:     make([]int, e.config.MaxShift),
	}
}

func (j *jobResult) Add(sample *stats.Sample, maxShift int) {
	sample, err := sample.Normalize()
	if err != nil {
		j.err = errors.Annotate(err, "failed to normalize log-profits")
		return
	}
	samples := sample.Data()
	for i := 0; i < len(samples); i++ {
		for k := 0; k < maxShift; k++ {
			shift := k + 1
			if i+shift >= len(samples) {
				continue
			}
			j.sums[k] += samples[i] * samples[i+shift]
			j.ns[k]++
		}
	}
}

func (j *jobResult) Merge(j2 jobResult) {
	if len(j.sums) != len(j2.sums) {
		panic(errors.Reason("jobResult: %s size=%d != %s size=%d",
			j.ticker, len(j.sums), j2.ticker, len(j2.sums)))
	}
	for i := 0; i < len(j.sums); i++ {
		j.sums[i] += j2.sums[i]
		j.ns[i] += j2.ns[i]
	}
}

func (e *AutoCorrelation) processTicker(ticker string) jobResult {
	res := e.newJobResult(ticker)
	rows, err := e.config.Reader.Prices(ticker)
	if err != nil {
		res.err = errors.Annotate(err, "failed to read ticker %s", ticker)
		return res
	}
	ts := stats.NewTimeseries().FromPrices(rows, stats.PriceFullyAdjusted)
	sample := ts.LogProfits(1)
	if len(sample.Data()) < e.config.MaxShift+2 {
		res.err = errors.Reason("too few samples: %d", len(sample.Data()))
		return res
	}
	res.Add(sample, e.config.MaxShift)
	return res
}

func (e *AutoCorrelation) processTickers(tickers []string) error {
	pm := iterator.ParallelMap[string, jobResult](
		e.context, 2*runtime.NumCPU(), iterator.FromSlice(tickers), e.processTicker)
	total := e.newJobResult("total")
	var numTickers int
	for r, ok := pm.Next(); ok; r, ok = pm.Next() {
		if r.err != nil {
			logging.Debugf(e.context, "skipping %s: %s", r.ticker, r.err.Error())
			continue
		}
		numTickers++
		total.Merge(r)
	}
	err := experiments.AddValue(e.context, e.prefix("tickers"), fmt.Sprintf("%d", numTickers))
	if err != nil {
		return errors.Annotate(err, "failed to add value for number of tickers")
	}
	err = experiments.AddValue(e.context, e.prefix("samples"), fmt.Sprintf("%d", total.ns[0]))
	if err != nil {
		return errors.Annotate(err, "failed to add value for number of samples")
	}
	if err := e.addPlot(total); err != nil {
		return errors.Annotate(err, "failed to add correlation plot")
	}
	return nil
}
