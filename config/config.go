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

// Package config implements configuration schema for all the experiments.
package config

import (
	"runtime"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/message"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// ExperimentConfig is a custom configuration for an experiment.
type ExperimentConfig interface {
	message.Message
	experiment() // no-op method to contain implementations to this package
	Name() string
}

// TestExperimentConfig is only used in tests.
type TestExperimentConfig struct {
	ID     string  `json:"id"`
	Grade  float64 `json:"grade" default:"2.0"`
	Passed bool    `json:"passed"`
	Graph  string  `json:"graph" required:"true"`
}

var _ ExperimentConfig = &TestExperimentConfig{}

func (t *TestExperimentConfig) experiment()  {}
func (t *TestExperimentConfig) Name() string { return "test" }

// InitMessage implements message.Message.
func (t *TestExperimentConfig) InitMessage(js any) error {
	return errors.Annotate(message.Init(t, js), "failed to parse test config")
}

// ScatterPlot configures a generic scatter plot.
type ScatterPlot struct {
	Graph string `json:"graph" required:"true"`
	// Expected line Y = incline * X + intercept.
	Incline      float64 `json:"incline" default:"1.0"`
	Intercept    float64 `json:"intercept"`
	PlotExpected bool    `json:"plot expected"` // plot Y = incline*X+intercept
	DeriveLine   bool    `json:"plot derived"`  // plot line from data
}

var _ message.Message = &ScatterPlot{}

func (p *ScatterPlot) InitMessage(js any) error {
	if err := message.Init(p, js); err != nil {
		return errors.Annotate(err, "failed to init ScatterPlot")
	}
	return nil
}

// StabilityPlot specifies a histogram plot representing a measure of stability
// of a statistic s over a Timeseries.
//
// It computes (s[subrange] - s[total]), possibly normalized by s[total].  The
// subrange is of size Window, and the values are sampled every Step points
// along the Timeseries.
type StabilityPlot struct {
	Step      int  `json:"step" default:"1"`
	Window    int  `json:"window" default:"1"`
	Normalize bool `json:"normalize" default:"true"`
	// When Normalize is true, skip a ticker when the absolute value of its
	// normalization coefficient is below the threshold.
	Threshold float64           `json:"threshold"`
	Plot      *DistributionPlot `json:"plot" required:"true"`
}

var _ message.Message = &StabilityPlot{}

func (p *StabilityPlot) InitMessage(js any) error {
	if err := message.Init(p, js); err != nil {
		return errors.Annotate(err, "failed to init StabilityPlot")
	}
	if p.Step < 1 {
		return errors.Reason(`"step"=%d must be >= 1`, p.Step)
	}
	if p.Window < 1 {
		return errors.Reason(`"window"=%d must be >= 1`, p.Window)
	}
	if p.Threshold < 0 {
		return errors.Reason(`"threshold"=%f must be >= 0`, p.Threshold)
	}
	return nil
}

// HoldPosition configures a single position within the Hold portfolio. Exactly
// one of "shares" (possibly fractional) or "start value" (the initial market
// value at Hold.Data.Start date) must be non-zero.
type HoldPosition struct {
	Ticker     string  `json:"ticker" required:"true"`
	Shares     float64 `json:"shares"`
	StartValue float64 `json:"start value"`
}

func (p *HoldPosition) InitMessage(js any) error {
	if err := message.Init(p, js); err != nil {
		return errors.Annotate(err, "failed to parse HoldPosition")
	}
	if (p.Shares == 0.0) == (p.StartValue == 0.0) {
		return errors.Reason(
			`exactly one of "shares" or "start value" must be non-zero for ticker %s`,
			p.Ticker)
	}
	return nil
}

// Hold experiment configuration.
type Hold struct {
	ID             string         `json:"id"`
	Reader         *db.Reader     `json:"data" required:"true"`
	Positions      []HoldPosition `json:"positions"`
	PositionsGraph string         `json:"positions graph"` // plots per position
	PositionsAxis  string         `json:"positions axis" choices:"left,right" default:"right"`
	TotalGraph     string         `json:"total graph"` // plot portfolio value
	TotalAxis      string         `json:"total axis" choices:"left,right" default:"right"`
}

var _ ExperimentConfig = &Hold{}

func (h *Hold) InitMessage(js any) error {
	return errors.Annotate(message.Init(h, js), "failed to parse Hold config")
}

func (h *Hold) experiment()  {}
func (h *Hold) Name() string { return "hold" }

// AnalyticalDistribution configures the type and parameters of a distibution.
type AnalyticalDistribution struct {
	Name  string  `json:"name" required:"true" choices:"t,normal"`
	Mean  float64 `json:"mean" default:"0.0"`
	MAD   float64 `json:"MAD" default:"1.0"`
	Alpha float64 `json:"alpha" default:"3.0"` // T dist. parameter
}

var _ message.Message = &AnalyticalDistribution{}

func (d *AnalyticalDistribution) InitMessage(js any) error {
	if err := message.Init(d, js); err != nil {
		return errors.Annotate(err, "failed to init AnalyticalDistribution")
	}
	if d.Name == "t" && d.Alpha <= 1.0 {
		return errors.Reason("T-distribution requires alpha=%f > 1.0", d.Alpha)
	}
	if d.MAD <= 0.0 {
		return errors.Reason("MAD=%f must be positive", d.MAD)
	}
	return nil
}

// CompoundDistribution specifies a compounded source distribution, that is, the
// distribution of the sum of N samples from the source distribution. The source
// can in turn be a CompoundDistribution, yielding N1*N2 compounded source, to
// arbitrary depth.
type CompoundDistribution struct {
	// Exactly one of the AnalyticalSource or CompoundSource must be present.
	AnalyticalSource *AnalyticalDistribution `json:"analytical source"`
	CompoundSource   *CompoundDistribution   `json:"compound source"`
	// When > 0, use SampleDistribution with this many samples from the source
	// distribution, rather than the source distribution directly.
	SourceSamples int `json:"source samples"`
	// When > 0, use it to seed the source distribution populating the sample
	// distribution. For use in tests.
	SeedSamples int `json:"seed samples"`
	N           int `json:"n" default:"1"` // sum of n source samples
	// How to compute the histogram for the compounded distribution:
	//
	// - direct: just sample the source N times for each compound sample;
	// - fast: use Y_i = sum(X_i, ..., X_N+i) for a single stream of X_i;
	// - biased: use variable substitution and Monte Carlo integration.
	CompoundType string `json:"compound type" choices:"direct,fast,biased" default:"biased"`
	// Compound algorithm parameters.
	Params stats.ParallelSamplingConfig `json:"parameters"`
}

var _ message.Message = &CompoundDistribution{}

func (d *CompoundDistribution) InitMessage(js any) error {
	if err := message.Init(d, js); err != nil {
		return errors.Annotate(err, "failed to init CompoundDistribution")
	}
	if (d.AnalyticalSource == nil) == (d.CompoundSource == nil) {
		return errors.Reason(
			`exactly one of "analytical source" or "compound source" must be specified`)
	}
	if d.N < 1 {
		return errors.Reason("n=%d must be >= 1", d.N)
	}
	return nil
}

// Source is a generic config for a set of price series that come either from
// the actual price database or synthetically generated.
type Source struct {
	// Real price series database. When present, no synthetic distribution is
	// allowed.
	DB       *db.Reader `json:"DB"`
	Compound int        `json:"compound" default:"1"`
	// Log-profit distribution for close[t]/close[t-1] by default, or
	// open[t+1]/close[t] when intraday distribution is present.
	DailyDist *AnalyticalDistribution `json:"daily distribution"`
	// Skip log-profits that span two days.
	IntradayOnly bool `json:"intraday only"`
	// Required for generating OHLC prices or intraday series.
	IntradayDist *AnalyticalDistribution `json:"intraday distribution"`
	// Default: 9:30am - 4pm.
	IntradayRange *db.IntradayRange `json:"intraday range"`
	// Resolution of the intraday samples in minutes: 1, 5, 15 or 30.
	IntradayRes int `json:"intraday resolution" default:"1"`
	// With DB, saves the start date and the number of days for each ticker as a
	// JSON file.  With synthetic distributions, read this file and generate
	// synthetic tickers accordingly, overwriting the other parameters.
	LengthsFile string `json:"lengths file"`
	// Amount of synthetic data to generate. Note, that with intraday
	// distribution, the number of samples is N*days where N is the number of
	// intraday samples.
	Tickers int `json:"tickers" default:"1"` // #synthetic tickers
	Days    int `json:"days" default:"5000"` // #synthetic days per ticker
	// All synthetic sequences start on this day; default:"1998-01-02".
	StartDate db.Date `json:"start date"`
	// Parallel processing parameters.
	Workers   int `json:"workers"`                 // default: 2*runtime.NumCPU()
	BatchSize int `json:"batch size" default:"10"` // must be >= 1
}

func (s *Source) InitMessage(js any) error {
	if err := message.Init(s, js); err != nil {
		return errors.Annotate(err, "failed to init Source")
	}
	if s.DB != nil {
		if s.DailyDist != nil {
			return errors.Reason(`cannot have both "DB" and "daily distribution"`)
		}
		if s.IntradayDist != nil {
			return errors.Reason(`cannot have both "DB" and "intraday distribution"`)
		}
	}
	if s.IntradayRange == nil {
		start := db.NewTimeOfDay(9, 30, 0, 0)
		end := db.NewTimeOfDay(16, 0, 0, 0)
		s.IntradayRange = &db.IntradayRange{
			Start: &start,
			End:   &end,
		}
	}
	intradayResValid := false
	for _, v := range []int{1, 5, 15, 30} {
		if s.IntradayRes == v {
			intradayResValid = true
			break
		}
	}
	if !intradayResValid {
		return errors.Reason(`"intraday resolution"=%d must be 1, 5, 15 or 30`,
			s.IntradayRes)
	}
	if s.StartDate.IsZero() {
		s.StartDate = db.NewDate(1998, 1, 2)
	}
	if s.Workers <= 0 {
		s.Workers = 2 * runtime.NumCPU()
	}
	if s.BatchSize < 1 {
		return errors.Reason(`"batch size"=%d must be >= 1`, s.BatchSize)
	}
	return nil
}

// DeriveAlpha configures parameters for finding the alpha parameter for a
// Student's T distribution that fits best the data.
type DeriveAlpha struct {
	MinX          float64 `json:"min x" required:"true"`
	MaxX          float64 `json:"max x" required:"true"`
	Epsilon       float64 `json:"epsilon" default:"0.01"` // min size of the search interval
	MaxIterations int     `json:"max iterations" default:"1000"`
	IgnoreCounts  int     `json:"ignore counts" default:"10"`
}

var _ message.Message = &DeriveAlpha{}

func (f *DeriveAlpha) InitMessage(js any) error {
	if err := message.Init(f, js); err != nil {
		return errors.Annotate(err, "failed to init DeriveAlpha")
	}
	if f.MinX > f.MaxX {
		return errors.Reason("min x=%g must be <= max x=%g", f.MinX, f.MaxX)
	}
	if f.Epsilon <= 0.0 {
		return errors.Reason("epsilon=%g must be > 0.0", f.Epsilon)
	}
	if f.MaxIterations < 1 {
		return errors.Reason("max iterations = %d must be >= 1", f.MaxIterations)
	}
	if f.IgnoreCounts < 0 {
		return errors.Reason("ignore counts = %d must be >= 0", f.IgnoreCounts)
	}
	return nil
}

// DistributionPlot is a config for plotting a given distribution's histogram,
// its statistics, and its approximation by an analytical distribution.
type DistributionPlot struct {
	// At least one of Graph or CountsGraph must be present.
	Graph          string                `json:"graph"`        // plot distribution
	CountsGraph    string                `json:"counts graph"` // plot buckets' counts
	ErrorsGraph    string                `json:"errors graph"` // plot bucket's standard errors
	Buckets        stats.Buckets         `json:"buckets"`
	ChartType      string                `json:"chart type" choices:"line,bars" default:"line"`
	Normalize      bool                  `json:"normalize"`  // to mean=0, MAD=1
	UseMeans       bool                  `json:"use means"`  // use bucket means rather than middles
	KeepZeros      bool                  `json:"keep zeros"` // by default, skip y==0 points
	LogY           bool                  `json:"log Y"`      // plot log10(y)
	LeftAxis       bool                  `json:"left axis"`
	CountsLeftAxis bool                  `json:"counts left axis"`
	ErrorsLeftAxis bool                  `json:"errors left axis"`
	RefDist        *CompoundDistribution `json:"reference distribution"`
	// When RefDist is an uncompounded (N=1) analytical distribution, its mean and
	// MAD will be automatically adjusted when AdjustRef is true.
	AdjustRef bool `json:"adjust reference distribution"`
	// Similarly, for uncompound t-distribution RefDist, alpha is derived from the
	// data.
	DeriveAlpha *DeriveAlpha `json:"derive alpha"`
	PlotMean    bool         `json:"plot mean"`
	Percentiles []float64    `json:"percentiles"` // in [0..100]
}

var _ message.Message = &DistributionPlot{}

func (dp *DistributionPlot) InitMessage(js any) error {
	if err := message.Init(dp, js); err != nil {
		return errors.Annotate(err, "failed to init DistributionPlot")
	}
	if dp.Graph == "" && dp.CountsGraph == "" {
		return errors.Reason(`expected at least one of "graph" or "counts graph"`)
	}
	for _, p := range dp.Percentiles {
		if p < 0.0 || 100.0 < p {
			return errors.Reason("percentile=%g must be in [0..100]", p)
		}
	}
	return nil
}

// Distribution is the experiment config for deriving the distribution of
// log-profits. By default, it normalizes the log-profits to have 0.0 mean and
// 1.0 MAD; set "normalize" to false for the original distribution.  When
// plotting the reference (analytical) distribution for non-normalized samples,
// setting "adjust reference distribution" flag sets the mean and MAD of the
// reference to that of the sample.
type Distribution struct {
	ID         string            `json:"id"` // experiment ID, for multiple instances
	Data       *Source           `json:"data" required:"true"`
	LogProfits *DistributionPlot `json:"log-profits"`
	Means      *DistributionPlot `json:"means"`
	MADs       *DistributionPlot `json:"MADs"`
	// mean[subrange] / mean[overall]. Same for MAD.
	MeanStability *StabilityPlot `json:"mean stability"`
	MADStability  *StabilityPlot `json:"MAD stability"`
}

var _ ExperimentConfig = &Distribution{}

func (e *Distribution) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Distribution")
	}
	return nil
}

func (e *Distribution) experiment()  {}
func (e *Distribution) Name() string { return "distribution" }

// CumulativeStatistic is a statistic that accumulates over the number of
// samples, like a mean or a MAD.  This configures a plot showing how such
// accumulation behaves as the number of samples grow.  The plotted number of
// Points are logarithmically spread out for each multiple of Samlpes. By
// default, the first 10K samples are plotted with 200 points, 100M samples
// (10K^2) - with 400 points, and so on.
type CumulativeStatistic struct {
	Graph   string `json:"graph" required:"true"`
	Samples int    `json:"samples" default:"10000"` // >= 3
	Points  int    `json:"points" default:"200"`    // >= 3
	// Skip this many first points.
	Skip         int           `json:"skip"`
	Percentiles  []float64     `json:"percentiles"` // in [0..100]
	Buckets      stats.Buckets `json:"buckets"`     // for estimating percentiles
	PlotExpected bool          `json:"plot expected"`
}

var _ message.Message = &CumulativeStatistic{}

func (c *CumulativeStatistic) InitMessage(js any) error {
	if err := message.Init(c, js); err != nil {
		return errors.Annotate(err, "failed to init CumulativeDistribution")
	}
	if c.Samples < 3 {
		return errors.Reason("samples=%d must be >= 3", c.Samples)
	}
	if c.Points < 3 {
		return errors.Reason("points=%d must be >= 3", c.Points)
	}
	for _, p := range c.Percentiles {
		if p < 0.0 || 100.0 < p {
			return errors.Reason("percentile=%g must be in [0..100]", p)
		}
	}
	return nil
}

type PowerDist struct {
	ID         string               `json:"id"` // experiment ID, for multiple instances
	Dist       CompoundDistribution `json:"distribution"`
	SamplePlot *DistributionPlot    `json:"sample plot"` // sampled Dist

	// Graphs of cumulative statistics, up to Samples, all generated from the same
	// sequence of values.
	CumulMean    *CumulativeStatistic `json:"cumulative mean"`
	CumulMAD     *CumulativeStatistic `json:"cumulative MAD"`
	CumulSigma   *CumulativeStatistic `json:"cumulative sigma"`
	CumulAlpha   *CumulativeStatistic `json:"cumulative alpha"`
	CumulSkew    *CumulativeStatistic `json:"cumulative skewness"`
	CumulKurt    *CumulativeStatistic `json:"cumulative kurtosis"`
	CumulSamples int                  `json:"cumulative samples" default:"10000"` // >= 3

	// Distributions of derived statistics estimated by computing each statistic
	// StatsSamples number of times.
	MeanDist  *DistributionPlot `json:"mean distribution"`
	MADDist   *DistributionPlot `json:"MAD distribution"`
	SigmaDist *DistributionPlot `json:"sigma distribution"`
	AlphaDist *DistributionPlot `json:"alpha distribution"`
	// Default: alpha \in [1.01..100], e=0.01, max. iter=1000, ignore counts=10.
	AlphaParams *DeriveAlpha `json:"alpha params"`
	StatSamples int          `json:"statistic samples" default:"10000"` // >= 3
}

var _ ExperimentConfig = &PowerDist{}

func (e *PowerDist) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init PowerDist")
	}
	if e.CumulSamples < 3 {
		return errors.Reason("cumulative samples=%d must be >= 3", e.CumulSamples)
	}
	if e.StatSamples < 3 {
		return errors.Reason("statistic samples=%d must be >= 3", e.StatSamples)
	}
	if e.AlphaParams == nil {
		e.AlphaParams = &DeriveAlpha{
			MinX:          1.01,
			MaxX:          100.0,
			Epsilon:       0.01,
			MaxIterations: 1000,
			IgnoreCounts:  10,
		}
	}
	return nil
}

func (e *PowerDist) experiment()  {}
func (e *PowerDist) Name() string { return "power distribution" }

// PortfolioPosition is a single position in a portfolio: a certain number of
// split-adjusted shares of a particular ticker purchased at a certain total
// price (cost basis) on a given date.
type PortfolioPosition struct {
	Ticker string `json:"ticker" required:"true"`
	Shares int    `json:"shares" required:"true"` // number of shares owned, >= 0
	// Total cost of purchase; default is closing price at purchase date * shares.
	CostBasis    float64 `json:"cost basis"` // >= 0
	PurchaseDate db.Date `json:"purchase date" required:"true"`
}

var _ message.Message = &PortfolioPosition{}

func (e *PortfolioPosition) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init PortfolioPosition")
	}
	if e.Shares < 0 {
		return errors.Reason("shares=%d must be >= 0", e.Shares)
	}
	if e.CostBasis < 0 {
		return errors.Reason("cost basis=%g must be >= 0", e.CostBasis)
	}
	return nil
}

// PortfolioColumn defines the data for a single output table column.
type PortfolioColumn struct {
	Kind string  `json:"kind" required:"true" choices:"ticker,name,exchange,category,sector,industry,purchase date,cost basis,shares,price,value"`
	Date db.Date `json:"date"` // required for "price" and "value"
}

var _ message.Message = &PortfolioColumn{}

func (e *PortfolioColumn) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init PortfolioColumn")
	}
	switch e.Kind {
	case "value", "price":
		if e.Date.IsZero() {
			return errors.Reason("date is required for kind=%s", e.Kind)
		}
	}
	return nil
}

// Portfolio experiment takes a list of positions and generates a table of
// configurable position rows. This is not really an experiment but a
// convenience tool for analyizing an existing portfolio.
type Portfolio struct {
	Reader    *db.Reader          `json:"data" required:"true"`
	ID        string              `json:"id"`
	Positions []PortfolioPosition `json:"positions"`
	Columns   []PortfolioColumn   `json:"columns"` // default: [{"kind": "ticker"}]
	// CSV output file; empty string == text on stdout.
	File string `json:"file"`
}

var _ ExperimentConfig = &Portfolio{}

func (e *Portfolio) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Portfolio")
	}
	if len(e.Columns) == 0 {
		e.Columns = []PortfolioColumn{{Kind: "ticker"}}
	}
	return nil
}

func (e *Portfolio) experiment()  {}
func (e *Portfolio) Name() string { return "portfolio" }

// AutoCorrelation is a config for the auto-correlation experiment.
type AutoCorrelation struct {
	ID       string  `json:"id"` // experiment ID, for multiple instances
	Data     *Source `json:"data" required:"true"`
	Graph    string  `json:"graph" required:"true"` // plot correlation vs. shift
	MaxShift int     `json:"max shift" default:"5"` // shift range [1..max]
}

var _ ExperimentConfig = &AutoCorrelation{}

func (e *AutoCorrelation) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init AutoCorrelation")
	}
	if e.MaxShift <= 0 {
		return errors.Reason("max shift = %d must be >= 1", e.MaxShift)
	}
	return nil
}

func (e *AutoCorrelation) experiment()  {}
func (e *AutoCorrelation) Name() string { return "auto-correlation" }

// Beta experiment studies cross-correlation between stocks and/or an index.
type Beta struct {
	ID string `json:"id"` // experiment ID, for multiple instances
	// Reference is expected to produce exactly one price series.
	Reference *Source `json:"reference" required:"true"`
	// Data reads real prices from DB, or generates R sequences.
	Data *Source `json:"data" required:"true"`
	// Model P = beta * Ref + R for synthetic price series.
	Beta float64 `json:"beta" default:"1.0"`

	// CSV dump with info about each stock's beta and R parameters. When set to
	// "-", print the table to stdout.
	File        string            `json:"file"`
	BetaPlot    *DistributionPlot `json:"beta plot"` // distribution of betas
	RPlot       *DistributionPlot `json:"R plot"`    // combined distribution of R
	RMeansPlot  *DistributionPlot `json:"R means"`   // distribution of E[R]
	RMADsPlot   *DistributionPlot `json:"R MADs"`    // distribution of MAD[R]/MAD[P]
	RSigmasPlot *DistributionPlot `json:"R Sigmas"`  // distribution of sigma[R]/sigma[P]
	// Histogram of pairwise cross-correlations of R.
	RCorrPlot *DistributionPlot `json:"R correlations"`
	// When >0, sample this many random pairs to compute
	// cross-correlation. Enumerate all the pairs when 0.
	RCorrSamples int `json:"R correlations samples"`
	// Distribution of lengths of correlation log-profit sequences.
	LengthsPlot *DistributionPlot `json:"lengths plot"`
	// Histogram of beta[t-shift]/beta[t].
	BetaRatios *StabilityPlot `json:"beta ratios"`
}

var _ ExperimentConfig = &Beta{}

func (e *Beta) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Beta")
	}
	if e.RCorrSamples < 0 {
		return errors.Reason(`"R correlations samples"=%d must be >= 0`,
			e.RCorrSamples)
	}
	return nil
}

func (e *Beta) experiment()  {}
func (e *Beta) Name() string { return "beta" }

// Trading experiment studies possibilities of exploiting volatility without the
// need to predict the future.
type Trading struct {
	ID   string  `json:"id"` // experiment ID
	Data *Source `json:"data" required:"true"`
	// Log-profits of high and close relative to the same day open.
	HighOpenPlot  *DistributionPlot `json:"high/open plot"`
	CloseOpenPlot *DistributionPlot `json:"close/open plot"`
	// Optional threshold T to condition close/open distribution by high/open < T.
	Threshold *float64 `json:"threshold"`
	// Log-profits of OHLC relative to the previous Close.
	OpenPlot  *DistributionPlot `json:"open plot"`
	HighPlot  *DistributionPlot `json:"high plot"`
	LowPlot   *DistributionPlot `json:"low plot"`
	ClosePlot *DistributionPlot `json:"close plot"` // classical daily log-profits
}

var _ ExperimentConfig = &Trading{}

func (e *Trading) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Trading")
	}
	return nil
}

func (e *Trading) experiment()  {}
func (e *Trading) Name() string { return "trading" }

// StrategyConfig is a custom configuration for a strategy.
type StrategyConfig interface {
	message.Message
	strategy() // no-op method to prevent instances outside this package
	Name() string
}

// IntradaySell condition. Exactly one condition must be specified.
type IntradaySell struct {
	// Sell at market on or after this time.
	Time *db.TimeOfDay `json:"time"`
	// When > 1, sell at or above price*target.
	Target float64 `json:"target"`
	// When > 0 (and must be < 1), sell at market when the price drops <=price*X.
	StopLoss float64 `json:"stop loss"`
	// When > 0 (and must be < 1), sell at market when the price drops
	// <=maxPrice*X where maxPrice is observed while holding the position.
	StopLossTrailing float64 `json:"stop loss trailing"`
}

func (s *IntradaySell) InitMessage(js any) error {
	if err := message.Init(s, js); err != nil {
		return errors.Annotate(err, "failed to init IntradaySell")
	}
	count := 0
	if s.Time != nil {
		count++
	}
	if s.Target > 0 {
		if s.Target <= 1 {
			return errors.Reason("target factor = %f must be > 1", s.Target)
		}
		count++
	}
	if s.StopLoss > 0 {
		if s.StopLoss >= 1 {
			return errors.Reason("stop loss = %f must be < 1", s.StopLoss)
		}
		count++
	}
	if s.StopLossTrailing > 0 {
		if s.StopLossTrailing >= 1 {
			return errors.Reason("stop loss trailing = %f must be < 1", s.StopLossTrailing)
		}
		count++
	}
	if count != 1 {
		return errors.Reason("exactly one condition must be specified")
	}
	return nil
}

// BuySellIntradayStrategy is a simple day trading strategy which buys at
// certain time of day (usually at open or near close) and sells when one of the
// conditions holds, checked in order. It is restricted to at most one buy per
// day, but may keep position overnight.
type BuySellIntradayStrategy struct {
	Buy  db.TimeOfDay   `json:"buy"`
	Sell []IntradaySell `json:"sell"`
}

var _ StrategyConfig = &BuySellIntradayStrategy{}

func (*BuySellIntradayStrategy) strategy()    {}
func (*BuySellIntradayStrategy) Name() string { return "buy-sell intraday" }

func (s *BuySellIntradayStrategy) InitMessage(js any) error {
	if err := message.Init(s, js); err != nil {
		return errors.Annotate(err, "failed to init BuySellIntradayStrategy")
	}
	return nil
}

// Strategy is a union of all strategy configurations. A specific strategy is
// specified as a single-element map {"<strategy name>": {<strategy config>}}.
type Strategy struct {
	Config StrategyConfig
}

var _ message.Message = &Strategy{}

func (s *Strategy) InitMessage(js any) error {
	m, ok := js.(map[string]any)
	if !ok || len(m) != 1 {
		return errors.Reason("strategy must be a single-element map: %v", js)
	}
	for name, jsConfig := range m {
		switch name { // add specific experiment implementations here
		case new(BuySellIntradayStrategy).Name():
			s.Config = new(BuySellIntradayStrategy)
		default:
			return errors.Reason("unknown strategy %s", name)
		}
		return errors.Annotate(s.Config.InitMessage(jsConfig),
			`failed to parse "%s" strategy config`, s.Config.Name())
	}
	return nil
}

func (s *Strategy) Name() string { return s.Config.Name() }

// Simulator experiment implements a strategy simulator with statistical
// analysis of the results.
type Simulator struct {
	ID         string            `json:"id"`
	Data       *Source           `json:"data"`
	StartValue float64           `json:"start value" default:"1000"` // cost basis
	Strategy   *Strategy         `json:"strategy" required:"true"`
	ProfitPlot *DistributionPlot `json:"profit plot"` // profit factor distribution
	// Plot profit as annualized factor.
	Annualize bool `json:"annualize" default:"true"`
	LogProfit bool `json:"log-profit"` // plot as log-profit
}

var _ ExperimentConfig = &Simulator{}

func (e *Simulator) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Simulator")
	}
	return nil
}

func (e *Simulator) experiment()  {}
func (e *Simulator) Name() string { return "simulator" }

// ExpMap represents a Message which reads a single-element map {name:
// Experiment} and knows how to populate specific implementations of the
// Experiment interface.
type ExpMap struct {
	Config ExperimentConfig `json:"-"` // populated directly in Init
}

var _ message.Message = &ExpMap{}

func (e *ExpMap) InitMessage(js any) error {
	m, ok := js.(map[string]any)
	if !ok || len(m) != 1 {
		return errors.Reason("experiment must be a single-element map: %v", js)
	}
	for name, jsConfig := range m {
		switch name { // add specific experiment implementations here
		case new(TestExperimentConfig).Name():
			e.Config = new(TestExperimentConfig)
		case new(Hold).Name():
			e.Config = new(Hold)
		case new(Distribution).Name():
			e.Config = new(Distribution)
		case new(PowerDist).Name():
			e.Config = new(PowerDist)
		case new(Portfolio).Name():
			e.Config = new(Portfolio)
		case new(AutoCorrelation).Name():
			e.Config = new(AutoCorrelation)
		case new(Beta).Name():
			e.Config = new(Beta)
		case new(Trading).Name():
			e.Config = new(Trading)
		case new(Simulator).Name():
			e.Config = new(Simulator)
		default:
			return errors.Reason("unknown experiment %s", name)
		}
		return errors.Annotate(e.Config.InitMessage(jsConfig),
			"failed to parse experiment config")
	}
	return nil
}

// Config is the top-level configuration of the app.
type Config struct {
	Groups      []*plot.GroupConfig `json:"groups"`
	Experiments []*ExpMap           `json:"experiments"`
}

var _ message.Message = &Config{}

func (c *Config) InitMessage(js any) error {
	if err := message.Init(c, js); err != nil {
		return errors.Annotate(err, "failed to parse top-level config")
	}
	groups := make(map[string]struct{})
	graphs := make(map[string]struct{})
	for i, g := range c.Groups {
		if _, ok := groups[g.ID]; ok {
			return errors.Reason("group[%d] has a duplicate id '%s'", i, g.ID)
		}
		groups[g.ID] = struct{}{}
		for j, gr := range g.Graphs {
			if _, ok := graphs[gr.ID]; ok {
				return errors.Reason(
					"graph[%d] in group '%s' has a duplicate id '%s'",
					j, g.ID, gr.ID)
			}
			graphs[gr.ID] = struct{}{}
		}
	}
	return nil
}

func Load(configPath string) (*Config, error) {
	var c Config
	if err := message.FromFile(&c, configPath); err != nil {
		return nil, errors.Annotate(err, "cannot read config '%s'", configPath)
	}
	return &c, nil
}
