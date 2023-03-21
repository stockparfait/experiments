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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// Experiment is a generic interface for a single experiment.
type Experiment interface {
	Prefix(s string) string
	AddValue(ctx context.Context, key, value string) error
	Run(ctx context.Context, cfg config.ExperimentConfig) error
}

// Prefix adds a space-separated prefix to s, unless prefix is empty.
func Prefix(prefix, s string) string {
	if prefix == "" {
		return s
	}
	return prefix + " " + s
}

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

// AddValue adds (or overwrites) a <prefix key>:value pair to the Values in the
// context.
func AddValue(ctx context.Context, prefix, key, value string) error {
	v := GetValues(ctx)
	if v == nil {
		return errors.Reason("no values map in context")
	}
	v[Prefix(prefix, key)] = value
	return nil
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
func PlotDistribution(ctx context.Context, dh stats.DistributionWithHistogram, c *config.DistributionPlot, prefix, legend string) error {
	if c == nil {
		return nil
	}
	var xs0 []float64
	var ys []float64

	h := dh.Histogram()
	if c.UseMeans {
		xs0 = h.Xs()
	} else {
		xs0 = h.Buckets().Xs(0.5)
	}

	ys = h.PDFs()
	xs, ys := filterXY(xs0, ys, c)
	min, max := minMax(ys)
	prefixedLegend := Prefix(prefix, legend)
	if err := plotDist(ctx, h, xs, ys, c, prefixedLegend); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", legend)
	}
	if err := plotCounts(ctx, h, xs0, c, prefixedLegend); err != nil {
		return errors.Annotate(err, "failed to plot '%s counts'", legend)
	}
	if err := plotErrors(ctx, h, xs0, c, prefixedLegend); err != nil {
		return errors.Annotate(err, "failed to plot '%s errors'", legend)
	}
	if c.PlotMean {
		if err := plotMean(ctx, dh, c.Graph, min, max, prefixedLegend); err != nil {
			return errors.Annotate(err, "failed to plot '%s mean'", legend)
		}
	}
	if err := plotPercentiles(ctx, dh, c, min, max, prefixedLegend); err != nil {
		return errors.Annotate(err, "failed to plot '%s percentiles'", legend)
	}
	if err := plotAnalytical(ctx, dh, c, prefix, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s ref dist'", legend)
	}
	if err := AddValue(ctx, prefix, legend+" P(X < mean-10*sigma)", fmt.Sprintf("%.4g", dh.CDF(dh.Mean()-10*math.Sqrt(dh.Variance())))); err != nil {
		return errors.Annotate(err, "failed to add value for '%s P(X < mean-10*sigma)'", legend)
	}
	if err := AddValue(ctx, prefix, legend+" P(X > mean+10*sigma)", fmt.Sprintf("%.4g", 1.0-dh.CDF(dh.Mean()+10*math.Sqrt(dh.Variance())))); err != nil {
		return errors.Annotate(err, "failed to add value for '%s P(X > mean+10*sigma)'", legend)
	}
	return nil
}

func plotDist(ctx context.Context, h *stats.Histogram, xs, ys []float64, c *config.DistributionPlot, legend string) error {
	if c.Graph == "" {
		return nil
	}
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	yLabel := "p.d.f."
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
	if graph == "" {
		return nil
	}
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
	if c.Graph == "" {
		return nil
	}
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

// Compound the distribution d; that is, return the distribution of the sum of n
// samples of d. The compounding is performed according to compType: "direct" (n
// samples per 1 compounded sample), "fast" (sliding window sum) or "biased"
// (based on Monte Carlo integration with an appropriate variable substitution),
// and the configuration of parallel sampling.
func Compound(ctx context.Context, d stats.Distribution, n int, compType string, c *stats.ParallelSamplingConfig) (dist stats.DistributionWithHistogram, err error) {
	switch compType {
	case "direct":
		dist = stats.CompoundRandDistribution(ctx, d, n, c)
	case "fast":
		dist = stats.FastCompoundRandDistribution(ctx, d, n, c)
	case "biased":
		h := stats.CompoundHistogram(ctx, d, n, c)
		dist = stats.NewHistogramDistribution(h)
	default:
		err = errors.Reason("unsupported compound type: %s", compType)
		return
	}
	return
}

// AnalyticalDistribution instantiates a distribution from config.
func AnalyticalDistribution(ctx context.Context, c *config.AnalyticalDistribution) (dist stats.Distribution, distName string, err error) {
	if c == nil {
		err = errors.Reason("config is nil")
		return
	}
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
	return
}

// CompoundDistribution instantiates a compounded distribution from config.
// When c.N=1, the source distribution is passed through as is.
func CompoundDistribution(ctx context.Context, c *config.CompoundDistribution) (dist stats.Distribution, distName string, err error) {
	switch {
	case c.AnalyticalSource != nil:
		dist, distName, err = AnalyticalDistribution(ctx, c.AnalyticalSource)
		if err != nil {
			err = errors.Annotate(err, "failed to create analytical distribution")
			return
		}
	case c.CompoundSource != nil:
		dist, distName, err = CompoundDistribution(ctx, c.CompoundSource)
		if err != nil {
			err = errors.Annotate(err, "failed to create inner compound distribution")
			return
		}
	default:
		err = errors.Reason("both analytical and compound sources are nil")
		return
	}
	if c.SourceSamples > 0 {
		if c.SeedSamples > 0 {
			dist.Seed(uint64(c.SeedSamples))
		}
		dist = stats.NewSampleDistributionFromRand(
			dist, c.SourceSamples, &c.Params.Buckets)
		distName += fmt.Sprintf("[samples=%d]", c.SourceSamples)
	}
	if c.N == 1 {
		return
	}
	dist, err = Compound(ctx, dist, c.N, c.CompoundType, &c.Params)
	if err != nil {
		err = errors.Annotate(err, "failed to compound the distribution")
		return
	}
	distName += fmt.Sprintf(" x %d", c.N)
	return
}

// synthConfig stores parameters for a single synthetic ticker sequence.
type synthConfig struct {
	Start  db.Date
	Length int
}

func saveLengths(lengths []synthConfig, fileName string) error {
	if fileName == "" {
		return nil
	}
	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Annotate(err, "failed to open lengths file '%s'", fileName)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(lengths); err != nil {
		return errors.Annotate(err, "failed to write JSON to '%s'", fileName)
	}
	return nil
}

func readLengths(fileName string) ([]synthConfig, error) {
	if fileName == "" {
		return nil, nil
	}
	f, err := os.Open(fileName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open lengths file '%s'", fileName)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var lengths []synthConfig
	if err := dec.Decode(&lengths); err != nil {
		return nil, errors.Annotate(err, "failed to decode lengths file '%s'", fileName)
	}
	return lengths, nil
}

type Prices struct {
	Ticker string
	Rows   []db.PriceRow
}

type LogProfits struct {
	Ticker     string
	Timeseries *stats.Timeseries
}

type withConf[T any] struct {
	v  T
	cs []synthConfig
}

func sourceDBPrices[T any](ctx context.Context, c *config.Source, f func([]Prices) T) (iterator.IteratorCloser[T], error) {
	if c.DB == nil {
		return nil, errors.Reason("DB must not be nil")
	}
	mapF := func(tickers []string) withConf[T] {
		var cs []synthConfig
		var prices []Prices
		for _, ticker := range tickers {
			rows, err := c.DB.Prices(ticker)
			if err != nil {
				logging.Warningf(ctx, "failed to read prices for %s: %s",
					ticker, err.Error())
				continue
			}
			length := len(rows)
			if length == 0 {
				logging.Warningf(ctx, "%s has no prices, skipping", ticker)
				continue
			}
			p := Prices{
				Ticker: ticker,
				Rows:   rows,
			}
			prices = append(prices, p)
			cs = append(cs, synthConfig{
				Length: length,
				Start:  rows[0].Date,
			})
		}
		return withConf[T]{v: f(prices), cs: cs}
	}
	tickers, err := c.DB.Tickers(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to list tickers")
	}
	batchIt := iterator.Batch[string](iterator.FromSlice(tickers), c.BatchSize)
	pm := iterator.ParallelMap(ctx, c.Workers, batchIt, mapF)
	var cs []synthConfig
	addLength := func(vc withConf[T]) T {
		cs = append(cs, vc.cs...)
		return vc.v
	}
	it := iterator.WithClose(iterator.Map[withConf[T], T](pm, addLength), func() {
		pm.Close()
		if err := saveLengths(cs, c.LengthsFile); err != nil {
			logging.Warningf(ctx, "failed to save lengths file: %s", err.Error())
		}
	})
	return it, nil
}

// tsConfig configures synthetic OHLC Timeseries of length n starting from the
// start date and using the corresponding distributions.
type tsConfig struct {
	open  stats.Distribution
	high  stats.Distribution
	low   stats.Distribution
	close stats.Distribution
	start db.Date
	n     int
}

func generateDates(start db.Date, n int) []db.Date {
	t := start.ToTime()
	dates := make([]db.Date, n)
	for i := 0; i < n; i++ {
		if t.Weekday() == time.Saturday {
			t = t.Add(2 * 24 * time.Hour)
		} else if t.Weekday() == time.Sunday {
			t = t.Add(24 * time.Hour)
		}
		dates[i] = db.NewDateFromTime(t)
		t = t.Add(24 * time.Hour)
	}
	return dates
}

// generateLogProfits generates a synthetic log-profit Timeseries.
func generateLogProfits(d stats.Distribution, start db.Date, n int) LogProfits {
	dates := generateDates(start, n)
	data := make([]float64, n)
	for i := 0; i < n; i++ {
		data[i] = d.Rand()
	}
	return LogProfits{
		Ticker:     "synthetic",
		Timeseries: stats.NewTimeseries(dates, data),
	}
}

func priceRow(date db.Date, open, high, low, close float32) db.PriceRow {
	p := db.PriceRow{
		Date:               date,
		Close:              close,
		CloseSplitAdjusted: close,
		CloseFullyAdjusted: close,
		Open:               open,
		High:               high,
		Low:                low,
		CashVolume:         1000,
	}
	p.SetActive(true)
	return p
}

func generatePrices(cfg tsConfig) Prices {
	dates := generateDates(cfg.start, cfg.n)
	rows := make([]db.PriceRow, cfg.n)
	// Set the initial price row before the first date at an arbitrary price of
	// 100. All the analysis uses relative price moves, so the initial value is
	// not important.
	curr := priceRow(cfg.start, 100.0, 100.0, 100.0, 100.0)
	rnd := func(d stats.Distribution, x float64) float64 {
		if d == nil {
			return cfg.close.Rand()
		}
		return x
	}
	for i := 0; i < cfg.n; i++ {
		close := float64(curr.Close) * math.Exp(rnd(cfg.close, 100.0))
		open := float64(curr.Open) * math.Exp(rnd(cfg.open, close))
		high := float64(curr.High) * math.Exp(rnd(cfg.high, close))
		low := float64(curr.Low) * math.Exp(rnd(cfg.low, close))
		if high < open {
			high = open
		}
		if high < close {
			high = close
		}
		if low > open {
			low = open
		}
		if low > close {
			low = close
		}
		rows[i] = priceRow(dates[i], float32(open), float32(high),
			float32(low), float32(close))
		curr = rows[i]
	}
	return Prices{
		Ticker: "synthetic",
		Rows:   rows,
	}
}

// distIter generates tsConfig sequence based on the iterator for the sequence
// lengths.
type distIter struct {
	open        stats.Distribution
	high        stats.Distribution
	low         stats.Distribution
	close       stats.Distribution
	lengthsIter iterator.Iterator[synthConfig]
}

var _ iterator.Iterator[tsConfig] = &distIter{}

func (it *distIter) Next() (tsConfig, bool) {
	c, ok := it.lengthsIter.Next()
	if !ok {
		return tsConfig{}, false
	}
	cp := func(d stats.Distribution) stats.Distribution {
		if d == nil {
			return nil
		}
		return d.Copy()
	}
	tsc := tsConfig{
		open:  cp(it.open),
		high:  cp(it.high),
		low:   cp(it.low),
		close: cp(it.close),
		start: c.Start,
		n:     c.Length,
	}
	return tsc, true
}

// sourceSynthehtic directly generates LogProfits rather than using
// sourceSyntheticPrices, for efficiency.
func sourceSynthetic[T any](ctx context.Context, c *config.Source, f func([]LogProfits) T) (iterator.IteratorCloser[T], error) {
	d, _, err := AnalyticalDistribution(ctx, c.Close)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create synthetic distribution")
	}
	var lengthsIter iterator.Iterator[synthConfig]
	if c.LengthsFile != "" {
		lengths, err := readLengths(c.LengthsFile)
		if err != nil {
			return nil, errors.Annotate(err, "failed to read lengths")
		}
		lengthsIter = iterator.FromSlice(lengths)
	} else {
		lengthsIter = iterator.Repeat(
			synthConfig{Start: c.StartDate, Length: c.Samples}, c.Tickers)
	}
	distIt := &distIter{close: d, lengthsIter: lengthsIter}
	batchIt := iterator.Batch[tsConfig](distIt, c.BatchSize)
	pf := func(cs []tsConfig) T {
		var lps []LogProfits
		for _, c := range cs {
			if c.n < 2 { // n = number of raw prices, need at least 2
				continue
			}
			lp := generateLogProfits(c.close, c.start, c.n)
			// Skip the first spurious log-profit.
			ts := lp.Timeseries
			lp.Timeseries = stats.NewTimeseries(ts.Dates()[1:], ts.Data()[1:])
			lps = append(lps, lp)
		}
		return f(lps)
	}
	pm := iterator.ParallelMap[[]tsConfig, T](ctx, c.Workers, batchIt, pf)
	return pm, nil
}

func sourceSyntheticPrices[T any](ctx context.Context, c *config.Source, f func([]Prices) T) (iterator.IteratorCloser[T], error) {
	if c.Close == nil {
		return nil, errors.Reason("close distribution is nil")
	}
	close, _, err := AnalyticalDistribution(ctx, c.Close)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create close distribution")
	}
	var open, high, low stats.Distribution
	if c.Open != nil {
		open, _, err = AnalyticalDistribution(ctx, c.Open)
		if err != nil {
			return nil, errors.Annotate(err, "failed to create open distribution")
		}
	}
	if c.High != nil {
		high, _, err = AnalyticalDistribution(ctx, c.High)
		if err != nil {
			return nil, errors.Annotate(err, "failed to create high distribution")
		}
	}
	if c.Low != nil {
		low, _, err = AnalyticalDistribution(ctx, c.Low)
		if err != nil {
			return nil, errors.Annotate(err, "failed to create low distribution")
		}
	}
	var lengthsIter iterator.Iterator[synthConfig]
	if c.LengthsFile != "" {
		lengths, err := readLengths(c.LengthsFile)
		if err != nil {
			return nil, errors.Annotate(err, "failed to read lengths")
		}
		lengthsIter = iterator.FromSlice(lengths)
	} else {
		lengthsIter = iterator.Repeat(
			synthConfig{Start: c.StartDate, Length: c.Samples}, c.Tickers)
	}
	distIt := &distIter{
		open:        open,
		high:        high,
		low:         low,
		close:       close,
		lengthsIter: lengthsIter,
	}
	batchIt := iterator.Batch[tsConfig](distIt, c.BatchSize)
	pf := func(cs []tsConfig) T {
		var prices []Prices
		for _, c := range cs {
			if c.n < 1 { // n = number of raw prices, need at least 1
				continue
			}
			prices = append(prices, generatePrices(c))
		}
		return f(prices)
	}
	pm := iterator.ParallelMap[[]tsConfig, T](ctx, c.Workers, batchIt, pf)
	return pm, nil
}

// Source generates log-profit sequence according to the config. Please remember
// to close the resulting iterator.
func Source(ctx context.Context, c *config.Source) (iterator.IteratorCloser[LogProfits], error) {
	sm, err := SourceMap(ctx, c, func(l []LogProfits) []LogProfits { return l })
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate log-profits")
	}
	it := iterator.Unbatch[LogProfits](sm)
	return iterator.WithClose(it, func() { sm.Close() }), nil
}

// SourceMap generates log-profit sequences according to the config, processes
// them with f in batches and returns an iterator of f([]LogProfits). The
// advantage over Source() followed by Map or ParallelMap is that f() is called
// in the same parallel worker that processes each batch of tickers, thus
// reducing inter-process communications.
//
// Please remember to close the resulting iterator.
func SourceMap[T any](ctx context.Context, c *config.Source, f func([]LogProfits) T) (iterator.IteratorCloser[T], error) {
	switch {
	case c.DB != nil:
		rowF := func(prices []Prices) T {
			var lps []LogProfits
			for _, p := range prices {
				ts := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceCloseFullyAdjusted)
				ts = ts.LogProfits(c.Compound, c.Intraday)
				lp := LogProfits{
					Ticker:     p.Ticker,
					Timeseries: ts,
				}
				if len(lp.Timeseries.Data()) == 0 {
					logging.Warningf(ctx, "%s has no log-profits, skipping", p.Ticker)
					continue
				}
				lps = append(lps, lp)
			}
			return f(lps)
		}
		return SourceMapPrices[T](ctx, c, rowF)
	case c.Close != nil:
		return sourceSynthetic[T](ctx, c, f)
	}
	return nil, errors.Reason(`one of "DB" or "close" must be configured`)
}

func SourceMapPrices[T any](ctx context.Context, c *config.Source, f func([]Prices) T) (iterator.IteratorCloser[T], error) {
	switch {
	case c.DB != nil:
		return sourceDBPrices[T](ctx, c, f)
	case c.Close != nil:
		return sourceSyntheticPrices[T](ctx, c, f)
	}
	return nil, errors.Reason(`one of "DB" or "close" must be configured`)
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

func plotAnalytical(ctx context.Context, dh stats.DistributionWithHistogram, c *config.DistributionPlot, prefix, legend string) error {
	if c.RefDist == nil || c.Graph == "" {
		return nil
	}
	dc := *c.RefDist // semi-deep copy, to modify locally
	var ac config.AnalyticalDistribution
	if dc.AnalyticalSource != nil {
		ac = *dc.AnalyticalSource
		dc.AnalyticalSource = &ac
	}
	if c.AdjustRef && dc.N == 1 && dc.AnalyticalSource != nil {
		ac.Mean = dh.Mean()
		ac.MAD = dh.MAD()
	}

	h := dh.Histogram()
	var xs []float64
	if c.UseMeans {
		xs = h.Xs()
	} else {
		xs = h.Buckets().Xs(0.5)
	}
	if c.DeriveAlpha != nil && dc.N == 1 && dc.AnalyticalSource != nil && ac.Name == "t" {
		ac.Alpha = DeriveAlpha(h, ac.Mean, ac.MAD, c.DeriveAlpha)
	}

	if err := AddValue(ctx, prefix, legend+" mean", fmt.Sprintf("%.4g", dh.Mean())); err != nil {
		return errors.Annotate(err, "failed to add value for '%s mean'", legend)
	}
	if err := AddValue(ctx, prefix, legend+" MAD", fmt.Sprintf("%.4g", dh.MAD())); err != nil {
		return errors.Annotate(err, "failed to add value for '%s MAD'", legend)
	}
	if dc.AnalyticalSource != nil && dc.AnalyticalSource.Name == "t" {
		alpha := fmt.Sprintf("%.4g", dc.AnalyticalSource.Alpha)
		if err := AddValue(ctx, prefix, legend+" alpha", alpha); err != nil {
			return errors.Annotate(err, "failed to add value for '%s alpha'", legend)
		}
	}
	dist, distName, err := CompoundDistribution(ctx, &dc)
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
	plt.SetLegend(Prefix(prefix, legend) + " ref:" + distName)
	plt.SetChartType(plot.ChartDashed)
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

// LeastSquares computes 1-D linear regression for Y = incline*X + intercept
// based on the given data. The number of elements in xs and ys must be the
// same. It is possible for the incline to be +Inf (when all xs are the
// same), which is not an error.
func LeastSquares(xs, ys []float64) (incline float64, intercept float64, err error) {
	if len(xs) != len(ys) {
		err = errors.Reason("len(xs)=%d != len(ys)=%d", len(xs), len(ys))
		return
	}
	if len(xs) < 2 {
		err = errors.Reason("len(xs)=%d < 2: not enough points", len(xs))
		return
	}
	sampleX := stats.NewSample(xs)
	sampleY := stats.NewSample(ys)
	varX := sampleX.Variance()
	if varX == 0 {
		incline = math.Inf(1)
		return
	}
	meanX := sampleX.Mean()
	meanY := sampleY.Mean()

	var cov float64
	for i, x := range xs {
		cov += (x - meanX) * (ys[i] - meanY)
	}
	cov /= float64(len(xs))
	incline = cov / varX
	intercept = meanY - incline*meanX
	return
}

// PlotScatter plots the unordered points given as xs and ys as a scatter plot,
// according to the config.
func PlotScatter(ctx context.Context, xs, ys []float64, c *config.ScatterPlot, prefix, legend, yLabel string) error {
	if c.Graph == "" {
		return nil
	}
	if len(xs) != len(ys) {
		return errors.Reason("len(xs)=%d != len(ys)=%d", len(xs), len(ys))
	}
	prefixedLegend := Prefix(prefix, legend)

	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetChartType(plot.ChartScatter).SetYLabel(yLabel).SetLegend(prefixedLegend)
	if err := plot.Add(ctx, plt, c.Graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	minX, maxX := minMax(xs)
	if c.PlotExpected {
		lgd := prefixedLegend + " expected"
		line := []float64{minX*c.Incline + c.Intercept, maxX*c.Incline + c.Intercept}
		plt, err := plot.NewXYPlot([]float64{minX, maxX}, line)
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s'", lgd)
		}
		plt.SetChartType(plot.ChartDashed).SetYLabel(yLabel).SetLegend(lgd)
		if err := plot.Add(ctx, plt, c.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s'", lgd)
		}
	}
	if c.DeriveLine {
		a, b, err := LeastSquares(xs, ys)
		lgd := prefixedLegend + " derived"
		if err != nil {
			logging.Warningf(ctx, "skipping %s: %s", lgd, err.Error())
		}
		if math.IsInf(a, 0) {
			logging.Warningf(ctx, "skipping %s: incline is infinite", lgd)
		}
		line := []float64{minX*a + b, maxX*a + b}
		plt, err := plot.NewXYPlot([]float64{minX, maxX}, line)
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s'", lgd)
		}
		plt.SetYLabel(yLabel).SetLegend(lgd)
		if err := plot.Add(ctx, plt, c.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s'", lgd)
		}
	}
	return nil
}

// Stability returns a series of deviations of the statistic f over a Timeseries
// of size `length`, as specified by the config.
//
// Here f computes the statistic for the given range [low..high) (includes low,
// excludes high).
func Stability(length int, f func(low, high int) float64, c *config.StabilityPlot) []float64 {
	if c == nil {
		return nil
	}
	if length < c.Step+c.Window {
		return nil
	}
	var norm float64 = 1
	if c.Normalize {
		norm = f(0, length)
		threshold := c.Threshold
		if threshold < 0 {
			threshold = 0
		}
		if math.Abs(norm) <= threshold {
			return nil
		}
	}
	var res []float64
	for h := length; h >= c.Window; h -= c.Step {
		res = append(res, f(h-c.Window, h)/norm)
	}
	return res
}

// TestExperiment is a fake experiment used in tests. Define actual experiments
// in their own subpackages.
type TestExperiment struct {
	cfg *config.TestExperimentConfig
}

var _ Experiment = &TestExperiment{}

func (t *TestExperiment) Prefix(s string) string {
	return Prefix(t.cfg.ID, s)
}

func (t *TestExperiment) AddValue(ctx context.Context, k, v string) error {
	return AddValue(ctx, t.cfg.ID, k, v)
}

func (t *TestExperiment) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	t.cfg, ok = cfg.(*config.TestExperimentConfig)
	if !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	if err := t.AddValue(ctx, "grade", fmt.Sprintf("%g", t.cfg.Grade)); err != nil {
		return errors.Annotate(err, "cannot add grade value")
	}
	passed := "failed"
	if t.cfg.Passed {
		passed = "passed"
	}
	if err := t.AddValue(ctx, "test", passed); err != nil {
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
