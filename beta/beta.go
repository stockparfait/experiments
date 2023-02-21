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
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
	"github.com/stockparfait/stockparfait/table"
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
	ts := stats.NewTimeseriesFromPrices(rows, stats.PriceFullyAdjusted)
	e.refTS = ts.LogProfits(1)
	return nil
}

// logProfits is a value returned by the data iterator.
type logProfits struct {
	ticker string
	ts     *stats.Timeseries
}

// generateTS generates a synthetic log-profit Timeseries based on config and
// log-profit Distribution.
func (e *Beta) generateTS(d stats.Distribution, samples int) logProfits {
	date := e.config.StartDate
	dates := make([]db.Date, samples)
	data := make([]float64, samples)
	for i := 0; i < samples; i++ {
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
		ts:     stats.NewTimeseries(dates, data),
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
	e.refTS = e.generateTS(d, e.config.Samples).ts
	return nil
}

// tsConfig configures generateTS to generate a synthetic Timeseries of length n
// using the given distribution copy.
type tsConfig struct {
	d stats.Distribution
	n int
}

// repeatIter repeats n times the value v.
type repeatIter struct {
	v int
	n int
}

var _ iterator.Iterator[int] = &repeatIter{}

func (it *repeatIter) Next() (int, bool) {
	if it.n <= 0 {
		return 0, false
	}
	it.n--
	return it.v, true
}

// distIter generates tsConfig sequence based on the iterator for the sequence
// lengths.
type distIter struct {
	d           stats.Distribution
	lengthsIter iterator.Iterator[int]
}

var _ iterator.Iterator[tsConfig] = &distIter{}

func (it *distIter) Next() (tsConfig, bool) {
	n, ok := it.lengthsIter.Next()
	if !ok {
		return tsConfig{}, false
	}
	return tsConfig{d: it.d.Copy(), n: n}, true
}

func (e *Beta) processData() error {
	if e.config.Data == nil {
		return nil
	}
	tickers, err := e.config.Data.Tickers(e.context)
	if err != nil {
		return errors.Annotate(err, "failed to list data tickers")
	}
	it := iterator.Batch(iterator.FromSlice(tickers), e.config.BatchSize)
	f := func(tickers []string) *lpStats {
		var res []logProfits
		for _, ticker := range tickers {
			var lp logProfits
			rows, err := e.config.Data.Prices(ticker)
			if err != nil {
				logging.Warningf(e.context, "skipping %s: %s", ticker, err.Error())
				continue
			}
			ts := stats.NewTimeseriesFromPrices(rows, stats.PriceFullyAdjusted)
			lp.ts = ts.LogProfits(1)
			lp.ticker = ticker
			res = append(res, lp)
		}
		return e.processLogProfits(res)
	}
	pm := iterator.ParallelMap(e.context, 2*runtime.NumCPU(), it, f)
	defer pm.Close()

	if err := e.processLpStats(pm); err != nil {
		return errors.Annotate(err, "failed to process log-profit stats")
	}
	return nil
}

type csvRow struct {
	Ticker  string
	Samples int
	Beta    float64
	Pmean   float64
	PMAD    float64
	Rmean   float64
	RMAD    float64
}

func csvRowHeader() []string {
	return []string{"Ticker", "Samples", "Beta", "E[P]", "MAD[P]", "E[R]", "MAD[R]"}
}

func (r csvRow) CSV() []string {
	return []string{
		r.Ticker,
		fmt.Sprintf("%d", r.Samples),
		fmt.Sprintf("%f", r.Beta),
		fmt.Sprintf("%f", r.Pmean),
		fmt.Sprintf("%f", r.PMAD),
		fmt.Sprintf("%f", r.Rmean),
		fmt.Sprintf("%f", r.RMAD),
	}
}

type lpStats struct {
	betas      []float64 // average beta
	betaRatios []float64 // beta[subrange]/beta - 1
	means      []float64
	mads       []float64
	sigmas     []float64
	lengths    []float64
	histR      *stats.Histogram
	rs         []*stats.Timeseries // for computing cross-correlations
	tickers    int
	samples    int
	rows       []table.Row
}

// Merge s2 into s. If error is returned, s remains unmodified.
func (s *lpStats) Merge(s2 *lpStats) error {
	if s.histR != nil {
		if err := s.histR.AddHistogram(s2.histR); err != nil {
			return errors.Annotate(err, "failed to merge R histograms")
		}
	}
	s.betas = append(s.betas, s2.betas...)
	s.betaRatios = append(s.betaRatios, s2.betaRatios...)
	s.means = append(s.means, s2.means...)
	s.mads = append(s.mads, s2.mads...)
	s.sigmas = append(s.sigmas, s2.sigmas...)
	s.lengths = append(s.lengths, s2.lengths...)
	s.rs = append(s.rs, s2.rs...)
	s.tickers += s2.tickers
	s.samples += s2.samples
	s.rows = append(s.rows, s2.rows...)
	return nil
}

func (e *Beta) writeTable(rows []table.Row) error {
	if e.config.File == "" {
		return nil
	}
	t := table.NewTable(csvRowHeader()...)
	t.AddRow(rows...)
	if e.config.File == "-" {
		if err := t.WriteText(os.Stdout, table.Params{}); err != nil {
			return errors.Annotate(err, "failed to write table to stdout")
		}
		return nil
	}
	f, err := os.OpenFile(e.config.File, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Annotate(err, "failed to open output CSV file '%s'",
			e.config.File)
	}
	defer f.Close()
	if err = t.WriteCSV(f, table.Params{}); err != nil {
		return errors.Annotate(err, "failed to write CSV file '%s'", e.config.File)
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
	var lengthsIter iterator.Iterator[int]
	if len(e.config.SamplesLengths) > 0 {
		lengthsIter = iterator.FromSlice(e.config.SamplesLengths)
	} else {
		lengthsIter = &repeatIter{v: e.config.Samples, n: e.config.Tickers}
	}
	distIt := &distIter{d: d, lengthsIter: lengthsIter}
	it := iterator.Batch[tsConfig](distIt, e.config.BatchSize)
	f := func(cs []tsConfig) *lpStats {
		res := make([]logProfits, len(cs))
		for i, c := range cs {
			lp := e.generateTS(c.d, c.n)
			tss := stats.TimeseriesIntersect(e.refTS, lp.ts)
			lp.ts = tss[0].MultC(e.config.Beta).Add(tss[1])
			res[i] = lp
		}
		return e.processLogProfits(res)
	}
	pm := iterator.ParallelMap(e.context, 2*runtime.NumCPU(), it, f)
	defer pm.Close()

	if err := e.processLpStats(pm); err != nil {
		return errors.Annotate(err, "failed to process synthetic log-profit stats")
	}
	return nil
}

// computeBeta for p = beta*ref+R which minimizes Var[R]. Assumes that p and ref
// have the same length.
func computeBeta(p, ref []float64) float64 {
	if len(p) < 2 {
		return 0
	}
	beta, _, err := experiments.LeastSquares(ref, p)
	if err != nil {
		panic(errors.Annotate(err, "failed to compute beta"))
	}
	if math.IsInf(beta, 0) {
		return 0
	}
	return beta
}

func (e *Beta) processLogProfits(lps []logProfits) *lpStats {
	var res lpStats
	if e.config.RPlot != nil {
		res.histR = stats.NewHistogram(&e.config.RPlot.Buckets)
	}
	for _, lp := range lps {
		tss := stats.TimeseriesIntersect(lp.ts, e.refTS)
		p := tss[0]
		ref := tss[1]
		if c := e.config.BetaRatios; c != nil {
			f := func(low, high int) float64 {
				return computeBeta(p.Data()[low:high], ref.Data()[low:high])
			}
			res.betaRatios = append(res.betaRatios,
				experiments.Stability(len(p.Data()), f, c)...)
		}
		beta := computeBeta(p.Data(), ref.Data())
		r := p.Sub(ref.MultC(beta))
		if e.config.RCorrPlot != nil {
			res.rs = append(res.rs, r)
		}
		sampleP := stats.NewSample(p.Data())
		sampleR := stats.NewSample(r.Data())
		if sampleR.MAD() == 0 {
			logging.Warningf(e.context, "skipping %s: MAD = 0", lp.ticker)
			continue
		}
		sampleNorm, err := sampleR.Normalize()
		if err != nil {
			logging.Warningf(e.context, "skipping %s: failed to normalize R", lp.ticker)
			continue
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
		res.lengths = append(res.lengths, float64(len(p.Data())))
		res.tickers++
		res.samples += len(p.Data())
		res.rows = append(res.rows, csvRow{
			Ticker:  lp.ticker,
			Samples: len(p.Data()),
			Beta:    beta,
			Pmean:   sampleP.Mean(),
			PMAD:    sampleP.MAD(),
			Rmean:   sampleR.Mean(),
			RMAD:    sampleR.MAD(),
		})
	}
	return &res
}

type intPair struct {
	x int
	y int
}

// nxnIter produces all pairs (i, j) such that i in [0..n-1] and j in
// [i+1..n-1]. Total of n*(n-1)/2 values.
type nxnPairs struct {
	i int
	j int
	n int
}

var _ iterator.Iterator[intPair] = &nxnPairs{}

func (it *nxnPairs) Next() (intPair, bool) {
	if it.i+1 >= it.n {
		return intPair{}, false
	}
	if it.j <= it.i {
		it.j = it.i + 1
	}
	res := intPair{it.i, it.j}
	it.j++
	if it.j >= it.n {
		it.i++
		it.j = it.i + 1
	}
	return res, true
}

// randPairs returns k random pairs (i, j) such that i in [0..n-1] and j in
// [i+1..n-1].
type randPairs struct {
	rand *rand.Rand
	i    int
	n    int
	k    int
}

var _ iterator.Iterator[intPair] = &randPairs{}

// newRandPairs initializes the randPairs iterator. Use seed=0 in production
// (this creates a new random seed), and seed>=1 in tests for deterministic
// behavior.
func newRandPairs(n, k int, seed int64) *randPairs {
	if seed <= 0 {
		seed = int64(time.Now().UnixNano())
	}
	return &randPairs{
		rand: rand.New(rand.NewSource(seed)),
		n:    n,
		k:    k,
	}
}

func (it *randPairs) Next() (intPair, bool) {
	if it.n < 2 || it.i >= it.k {
		return intPair{}, false
	}
	it.i++
	i := it.rand.Intn(it.n - 1)
	j := it.rand.Intn(it.n-i-1) + i + 1
	return intPair{i, j}, true
}

// correlation between t1 and t2. When the second result is false, correlation
// is undefined.
func (e *Beta) correlation(t1, t2 *stats.Timeseries) (float64, bool) {
	aligned := stats.TimeseriesIntersect(t1, t2)
	t1 = aligned[0]
	t2 = aligned[1]
	if len(t1.Data()) < 3 {
		return 0, false
	}
	sample1 := stats.NewSample(t1.Data())
	sample2 := stats.NewSample(t2.Data())
	mean1 := sample1.Mean()
	sigma1 := sample1.Sigma()
	if sigma1 == 0 {
		return 0, false
	}
	mean2 := sample2.Mean()
	sigma2 := sample2.Sigma()
	if sigma2 == 0 {
		return 0, false
	}
	var sum float64
	for k := 0; k < len(t1.Data()); k++ {
		sum += (t1.Data()[k] - mean1) * (t2.Data()[k] - mean2)
	}
	corr := sum / float64(len(t1.Data())) / sigma1 / sigma2
	if corr < -1 || corr > 1 {
		// This usually happens when sigma is too close to 0.
		return 0, false
	}
	return corr, true
}

// crossCorrelations computes pairwise correlations between the Timeseries and
// populates a histogram with the results. The number of pairs is capped by
// e.config.RCorrSamples.
func (e *Beta) crossCorrelations(tss []*stats.Timeseries, buckets *stats.Buckets) stats.DistributionWithHistogram {
	f := func(pairs []intPair) *stats.Histogram {
		h := stats.NewHistogram(buckets)
		for _, p := range pairs {
			corr, ok := e.correlation(tss[p.x], tss[p.y])
			if !ok {
				continue
			}
			h.Add(corr)
		}
		return h
	}
	var pairsIter iterator.Iterator[intPair]
	if e.config.RCorrSamples <= 0 || len(tss)*(len(tss)-1)/2 <= e.config.RCorrSamples {
		pairsIter = &nxnPairs{n: len(tss)}
	} else {
		pairsIter = newRandPairs(len(tss), e.config.RCorrSamples, 0)
	}
	it := iterator.Batch(pairsIter, e.config.BatchSize)
	pm := iterator.ParallelMap(e.context, 2*runtime.NumCPU(), it, f)
	defer pm.Close()
	h := stats.NewHistogram(buckets)
	for v, ok := pm.Next(); ok; v, ok = pm.Next() {
		h.AddHistogram(v)
	}
	return stats.NewHistogramDistribution(h)
}

func (e *Beta) saveLengths(lengths []float64) error {
	if e.config.Data == nil || e.config.LengthsFile == "" {
		return nil
	}
	f, err := os.OpenFile(e.config.LengthsFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Annotate(err, "failed to open lengths file '%s'",
			e.config.LengthsFile)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(lengths); err != nil {
		return errors.Annotate(err, "failed to write JSON to '%s'",
			e.config.LengthsFile)
	}
	return nil
}

// processLpStats accumulates partially reduced statistics from the iterator and
// generates the necessary plots.
func (e *Beta) processLpStats(it iterator.Iterator[*lpStats]) error {
	var res lpStats
	if e.config.RPlot != nil {
		res.histR = stats.NewHistogram(&e.config.RPlot.Buckets)
	}
	for s, ok := it.Next(); ok; s, ok = it.Next() {
		if err := res.Merge(s); err != nil {
			logging.Warningf(e.context, "failed to merge some tickers", err.Error())
		}
	}
	if err := e.saveLengths(res.lengths); err != nil {
		return errors.Annotate(err, "failed to save lengths in '%s'",
			e.config.LengthsFile)
	}
	if err := e.AddValue(e.context, "tickers", fmt.Sprintf("%d", res.tickers)); err != nil {
		return errors.Annotate(err, "failed to add %s value", e.Prefix("tickers"))
	}
	if err := e.AddValue(e.context, "samples", fmt.Sprintf("%d", res.samples)); err != nil {
		return errors.Annotate(err, "failed to add %s value", e.Prefix("samples"))
	}
	if e.config.BetaPlot != nil {
		betasDist := stats.NewSampleDistribution(res.betas, &e.config.BetaPlot.Buckets)
		err := experiments.PlotDistribution(e.context, betasDist, e.config.BetaPlot,
			e.config.ID, "betas")
		if err != nil {
			return errors.Annotate(err, "failed to plot betas")
		}
	}
	if err := e.writeTable(res.rows); err != nil {
		return errors.Annotate(err, "failed to write table")
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
	if e.config.RCorrPlot != nil {
		corrDist := e.crossCorrelations(res.rs, &e.config.RCorrPlot.Buckets)
		counts := corrDist.Histogram().CountsTotal()
		if counts < 2 { // too few for a plot
			logging.Warningf(e.context,
				"skipping R correlations plot: only %d points", counts)
		} else {
			err := experiments.PlotDistribution(e.context, corrDist, e.config.RCorrPlot,
				e.config.ID, "R cross-correlations")
			if err != nil {
				return errors.Annotate(err, "failed to plot R cross-correlations")
			}
			err = e.AddValue(e.context, "R cross-correlations",
				fmt.Sprintf("%d", counts))
			if err != nil {
				return errors.Annotate(err, "failed to add %s value",
					e.Prefix("R cross-correlations"))
			}
		}
	}
	if e.config.LengthsPlot != nil {
		dist := stats.NewSampleDistribution(res.lengths, &e.config.LengthsPlot.Buckets)
		err := experiments.PlotDistribution(e.context, dist, e.config.LengthsPlot,
			e.config.ID, "lengths")
		if err != nil {
			return errors.Annotate(err, "failed to plot lengths")
		}
	}
	if e.config.BetaRatios != nil && len(res.betaRatios) > 1 {
		c := e.config.BetaRatios.Plot
		dist := stats.NewSampleDistribution(res.betaRatios, &c.Buckets)
		err := experiments.PlotDistribution(e.context, dist, c, e.config.ID, "beta ratios")
		if err != nil {
			return errors.Annotate(err, "failed to plot beta ratios")
		}
	}
	return nil
}
