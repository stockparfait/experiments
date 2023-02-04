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

// Package beta is an experiment with cross-correlation between stocks.
//
// Specifically, it models a stock as P = beta*I+R relative to the reference
// price series I (typically, an index such as S&P500 or Nasdaq Composite) and
// studies the properties of beta and R.
package beta

import (
	"context"
	"runtime"
	"time"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
)

type Beta struct {
	config  *config.Beta
	context context.Context
	refTS   *stats.Timeseries // reference log-profit timeseries
}

var _ experiments.Experiment = &Beta{}

func (e *Beta) Prefix(s string) string {
	return experiments.Prefix(e.config.ID, s)
}

func (e *Beta) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, e.config.ID, k, v)
}

func (e *Beta) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if e.config, ok = cfg.(*config.Beta); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	e.context = ctx
	// process* methods do nothing and return no error when their configs are nil.
	if err := e.processRefData(); err != nil {
		return errors.Annotate(err, "failed to process reference data")
	}
	if err := e.processRefAnalytical(); err != nil {
		return errors.Annotate(err, "failed to process synthetic reference")
	}
	if err := e.processData(); err != nil {
		return errors.Annotate(err, "failed to process price data")
	}
	if err := e.processAnalyticalR(); err != nil {
		return errors.Annotate(err, "failed to process synthetic R")
	}
	return nil
}

func (e *Beta) processRefData() error {
	if e.config.RefData == nil {
		return nil
	}
	tickers, err := e.config.RefData.Tickers(e.context)
	if err != nil {
		return errors.Annotate(err, "failed to list reference tickers")
	}
	if len(tickers) != 1 {
		return errors.Reason("num. reference tickers=%d, must be 1", len(tickers))
	}
	rows, err := e.config.RefData.Prices(tickers[0])
	if err != nil {
		return errors.Annotate(err, "failed to read reference prices for %s",
			tickers[0])
	}
	ts := stats.NewTimeseries().FromPrices(rows, stats.PriceFullyAdjusted)
	e.refTS = ts.LogProfits(1)
	return nil
}

// logProfits is a value returned by the data iterator.
type logProfits struct {
	ticker string
	ts     *stats.Timeseries
	err    error
}

// generateTS generates a synthetic log-profit Timeseries based on config and
// log-profit Distribution.
func (e *Beta) generateTS(d stats.Distribution) logProfits {
	date := e.config.StartDate
	dates := make([]db.Date, e.config.Samples)
	data := make([]float64, e.config.Samples)
	for i := 0; i < e.config.Samples; i++ {
		t := date.ToTime()
		if t.Weekday() == time.Saturday {
			t = t.Add(2 * 24 * time.Hour)
		} else if t.Weekday() == time.Sunday {
			t = t.Add(24 * time.Hour)
		}
		dates[i] = db.NewDateFromTime(t)
		data[i] = d.Rand()
		t = t.Add(24 * time.Hour)
		date = db.NewDateFromTime(t)
	}
	return logProfits{
		ticker: "synthetic",
		ts:     stats.NewTimeseries().Init(dates, data),
	}
}

func (e *Beta) processRefAnalytical() error {
	if e.config.RefAnalytical == nil {
		return nil
	}
	d, _, err := experiments.AnalyticalDistribution(
		e.context, e.config.RefAnalytical)
	if err != nil {
		return errors.Annotate(err, "failed to create synthetic ref. distribution")
	}
	e.refTS = e.generateTS(d).ts
	return nil
}

type dataIter struct {
	r       *db.Reader
	tickers []string
	context context.Context
}

var _ iterator.Iterator[logProfits] = &dataIter{}

func (it *dataIter) Next() (logProfits, bool) {
	var res logProfits
	if len(it.tickers) == 0 {
		return res, false
	}
	ticker := it.tickers[0]
	it.tickers = it.tickers[1:]
	rows, err := it.r.Prices(ticker)
	if err != nil {
		res.err = errors.Annotate(err, "failed to read prices for %s", ticker)
		return res, true
	}
	ts := stats.NewTimeseries().FromPrices(rows, stats.PriceFullyAdjusted)
	res.ts = ts.LogProfits(1)
	res.ticker = ticker
	return res, true
}

// distIter generates N copies of a Distribution.
type distIter struct {
	d stats.Distribution
	n int
}

var _ iterator.Iterator[stats.Distribution] = &distIter{}

func (it *distIter) Next() (stats.Distribution, bool) {
	if it.n <= 0 {
		return nil, false
	}
	it.n--
	return it.d.Copy(), true
}

func (e *Beta) processData() error {
	if e.config.Data == nil {
		return nil
	}
	tickers, err := e.config.Data.Tickers(e.context)
	if err != nil {
		return errors.Annotate(err, "failed to list data tickers")
	}
	it := &dataIter{
		r:       e.config.Data,
		tickers: tickers,
		context: e.context,
	}
	if err := e.processLogProfitsIter(it); err != nil {
		return errors.Annotate(err, "failed to process log-profits")
	}
	return nil
}

func (e *Beta) processAnalyticalR() error {
	if e.config.AnalyticalR == nil {
		return nil
	}
	d, _, err := experiments.AnalyticalDistribution(
		e.context, e.config.AnalyticalR)
	if err != nil {
		return errors.Annotate(err, "failed to create synthetic R distribution")
	}
	it := &distIter{d: d, n: e.config.Tickers}
	f := func(d stats.Distribution) logProfits {
		lp := e.generateTS(d)
		if lp.err != nil {
			return logProfits{err: errors.Annotate(lp.err, "failed to generate R")}
		}
		tss := stats.TimeseriesIntersect(e.refTS, lp.ts)
		return logProfits{
			ticker: lp.ticker,
			ts:     tss[0].MultC(e.config.Beta).Add(tss[1]),
		}
	}
	pm := iterator.ParallelMap[stats.Distribution, logProfits](
		e.context, 2*runtime.NumCPU(), it, f)
	defer pm.Close()

	if err := e.processLogProfitsIter(pm); err != nil {
		return errors.Annotate(err, "failed to process synthetic log-profits")
	}
	return nil
}

// computeBeta for p = beta*ref+R which minimizes Var[R]. Assumes that p and ref
// have the same length.
func computeBeta(p, ref []float64) float64 {
	if len(p) != len(ref) {
		panic(errors.Reason("len(p)=%d != len(ref)=%d", len(p), len(ref)))
	}
	if len(p) < 2 {
		return 0
	}
	sampleP := stats.NewSample().Init(p)
	sampleRef := stats.NewSample().Init(ref)
	varRef := sampleRef.Variance()
	if varRef == 0 {
		return 0
	}
	meanP := sampleP.Mean()
	meanRef := sampleRef.Mean()

	var cov float64
	for i := range p {
		cov += (p[i] - meanP) * (ref[i] - meanRef)
	}
	return cov / float64(len(p)) / varRef
}

type lpStats struct {
	ticker string
	betas  []float64
	means  []float64
	mads   []float64
	sigmas []float64
	histR  *stats.Histogram
	err    error
}

// Merge s2 into s. If error is returned, s remains unmodified.
func (s *lpStats) Merge(s2 *lpStats) error {
	if s.err != nil {
		return errors.Annotate(s.err, "merging into an error lpStats for %s", s.ticker)
	}
	if s2.err != nil {
		return errors.Annotate(s2.err, "merging an error lpStats for %s", s2.ticker)
	}
	if s.histR != nil {
		if err := s.histR.AddHistogram(s2.histR); err != nil {
			return errors.Annotate(err, "failed to merge Histograms")
		}
	}
	s.betas = append(s.betas, s2.betas...)
	s.means = append(s.means, s2.means...)
	s.mads = append(s.mads, s2.mads...)
	s.sigmas = append(s.sigmas, s2.sigmas...)
	return nil
}

func (e *Beta) processLogProfits(lp logProfits) *lpStats {
	res := lpStats{ticker: lp.ticker}
	if e.config.RPlot != nil {
		res.histR = stats.NewHistogram(&e.config.RPlot.Buckets)
	}
	if lp.err != nil {
		res.err = errors.Annotate(lp.err, "log-profits error for %s", lp.ticker)
		return &res
	}
	tss := stats.TimeseriesIntersect(lp.ts, e.refTS)
	p := tss[0]
	ref := tss[1]
	beta := computeBeta(p.Data(), ref.Data())
	r := p.Sub(ref.MultC(beta))
	sampleP := stats.NewSample().Init(p.Data())
	sampleR := stats.NewSample().Init(r.Data())
	if sampleR.MAD() == 0 {
		res.err = errors.Reason("skipping %s: MAD = 0", lp.ticker)
		return &res
	}
	sampleNorm, err := sampleR.Normalize()
	if err != nil {
		res.err = errors.Annotate(err, "failed to normalize R for %s", lp.ticker)
		return &res
	}
	if res.histR != nil {
		res.histR.Add(sampleNorm.Data()...)
	}
	res.betas = append(res.betas, beta)
	res.means = append(res.means, sampleR.Mean())
	if madP := sampleP.MAD(); madP != 0 {
		res.mads = append(res.mads, sampleR.MAD()/madP)
	}
	if sigmaP := sampleP.Sigma(); sigmaP != 0 {
		res.sigmas = append(res.sigmas, sampleR.Sigma()/sigmaP)
	}
	return &res
}

// processLogProfitsIter accumulates statistics from the iterator over log-profits
// Timeseries. The log-profits may be either the actual historical price series
// or synthetically generated series.
func (e *Beta) processLogProfitsIter(it iterator.Iterator[logProfits]) error {
	pm := iterator.ParallelMap(e.context, runtime.NumCPU(), it, e.processLogProfits)
	defer pm.Close()

	var res lpStats
	if e.config.RPlot != nil {
		res.histR = stats.NewHistogram(&e.config.RPlot.Buckets)
	}
	for s, ok := pm.Next(); ok; s, ok = pm.Next() {
		if s.err != nil {
			logging.Warningf(e.context, "skipping %s: %s", s.ticker, s.err.Error())
			continue
		}
		if err := res.Merge(s); err != nil {
			logging.Warningf(e.context, "failed to merge %s, skipping: %s",
				s.ticker, s.err.Error())
		}
	}
	if e.config.BetaPlot != nil {
		betasDist := stats.NewSampleDistribution(res.betas, &e.config.BetaPlot.Buckets)
		err := experiments.PlotDistribution(e.context, betasDist, e.config.BetaPlot,
			e.config.ID, "betas")
		if err != nil {
			return errors.Annotate(err, "failed to plot betas")
		}
	}
	if e.config.RPlot != nil {
		RDist := stats.NewHistogramDistribution(res.histR)
		err := experiments.PlotDistribution(e.context, RDist, e.config.RPlot,
			e.config.ID, "normalized R")
		if err != nil {
			return errors.Annotate(err, "failed to plot normalized R")
		}
	}
	if e.config.RMeansPlot != nil {
		meansDist := stats.NewSampleDistribution(
			res.means, &e.config.RMeansPlot.Buckets)
		err := experiments.PlotDistribution(e.context, meansDist, e.config.RMeansPlot,
			e.config.ID, "R means")
		if err != nil {
			return errors.Annotate(err, "failed to plot R means")
		}
	}
	if e.config.RMADsPlot != nil {
		MADsDist := stats.NewSampleDistribution(res.mads, &e.config.RMADsPlot.Buckets)
		err := experiments.PlotDistribution(e.context, MADsDist, e.config.RMADsPlot,
			e.config.ID, "R MADs")
		if err != nil {
			return errors.Annotate(err, "failed to plot R MADs")
		}
	}
	if e.config.RSigmasPlot != nil {
		SigmasDist := stats.NewSampleDistribution(res.sigmas, &e.config.RSigmasPlot.Buckets)
		err := experiments.PlotDistribution(e.context, SigmasDist, e.config.RSigmasPlot,
			e.config.ID, "R Sigmas")
		if err != nil {
			return errors.Annotate(err, "failed to plot R Sigmas")
		}
	}
	return nil
}
