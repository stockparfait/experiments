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

// Name implements ExperimentConfig.
func (t *TestExperimentConfig) Name() string { return "test" }

// InitMessage implements message.Message.
func (t *TestExperimentConfig) InitMessage(js any) error {
	return errors.Annotate(message.Init(t, js), "failed to parse test config")
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
	Graph          string                `json:"graph" required:"true"`
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
	Reader     *db.Reader        `json:"data" required:"true"`
	LogProfits *DistributionPlot `json:"log-profits"`
	Means      *DistributionPlot `json:"means"`
	MADs       *DistributionPlot `json:"MADs"`
	Compound   int               `json:"compound" default:"1"`    // log-profit step size; must be >= 1
	BatchSize  int               `json:"batch size" default:"10"` // must be >0
	Workers    int               `json:"parallel workers"`        // >0; default = 2*runtime.NumCPU()
}

var _ ExperimentConfig = &Distribution{}

func (e *Distribution) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Distribution")
	}
	if e.Compound < 1 {
		return errors.Reason("compound = %d must be >= 1", e.Compound)
	}
	if e.BatchSize <= 0 {
		return errors.Reason("batch size = %d must be positive", e.BatchSize)
	}
	if e.Workers <= 0 {
		e.Workers = 2 * runtime.NumCPU()
	}
	return nil
}

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

func (e *Portfolio) Name() string { return "portfolio" }

// AutoCorrelation is a config for the auto-correlation experiment.
type AutoCorrelation struct {
	ID string `json:"id"` // experiment ID, for multiple instances
	// Exactly one of Reader or Analytical must be present.
	Reader     *db.Reader              `json:"data"`              // price data
	Analytical *AnalyticalDistribution `json:"analytical source"` // synthetic data
	// Number of synthetic points to generate.
	Samples int `json:"samples" default:"5000"`
	// Number of synthetic samples to process within each parallel job.
	BatchSize int    `json:"batch size" default:"5000"`
	Graph     string `json:"graph" required:"true"` // plot correlation vs. shift
	MaxShift  int    `json:"max shift" default:"5"` // shift range [1..max]
}

var _ ExperimentConfig = &AutoCorrelation{}

func (e *AutoCorrelation) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init AutoCorrelation")
	}
	if (e.Reader == nil) == (e.Analytical == nil) {
		return errors.Reason(
			`exactly one of "data" or "analytical source" must be specified`)
	}
	if e.MaxShift <= 0 {
		return errors.Reason("max shift = %d must be >= 1", e.MaxShift)
	}
	if e.Samples <= e.MaxShift+2 {
		return errors.Reason("samples=%d must be >= %d", e.Samples, e.MaxShift+2)
	}
	if e.BatchSize <= 1 {
		return errors.Reason("batch size=%d must be >= 1", e.BatchSize)
	}
	return nil
}

func (e *AutoCorrelation) Name() string { return "auto-correlation" }

// Beta experiment studies cross-correlation between stocks and/or an index.
type Beta struct {
	ID string `json:"id"` // experiment ID, for multiple instances
	// Exactly one of RefData or RefAnalytical must be non-nil.
	RefData       *db.Reader              `json:"reference data"`
	RefAnalytical *AnalyticalDistribution `json:"reference analytical"`
	// Exactly one of Data or AnalyticalR must be non-nil. Each ticker in Data is
	// analysed separately, contributing to statistics about beta and R.
	// AnalyticalR is the distribution of R for synthetic tickers.
	Data        *db.Reader              `json:"data"`
	AnalyticalR *AnalyticalDistribution `json:"analytical R"`
	// Model P = beta * Ref + R for synthetic price series.
	Beta    float64 `json:"beta" default:"1.0"`
	Tickers int     `json:"tickers" default:"1"`    // #synthetic tickers
	Samples int     `json:"samples" default:"5000"` // #synthetic prices per ticker
	// All synthetic sequences start on this day.
	StartDate db.Date `json:"start date"` // default:"1998-01-02"
	// CSV dump with info about each stock's beta and R parameters. When set to
	// "-", print the table to stdout.
	File       string            `json:"file"`
	BetaPlot   *DistributionPlot `json:"beta plot"` // distribution of betas
	RPlot      *DistributionPlot `json:"R plot"`    // combined distribution of R
	RMeansPlot *DistributionPlot `json:"R means"`   // distribution of means of R
	RMADsPlot  *DistributionPlot `json:"R MADs"`    // distribution of MADs of R
}

var _ ExperimentConfig = &Beta{}

func (e *Beta) InitMessage(js any) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Beta")
	}
	if (e.RefData == nil) == (e.RefAnalytical == nil) {
		return errors.Reason(
			`exactly one of "reference data" or "reference analytical" must be specified`)
	}
	if (e.Data == nil) == (e.AnalyticalR == nil) {
		return errors.Reason(
			`exactly one of "data" or "analytical R" must be specified`)
	}
	if e.Tickers < 1 {
		return errors.Reason(`"tickers"=%d must be >= 1`, e.Tickers)
	}
	if e.Samples < 5 {
		return errors.Reason(`"samples"=%d must be >= 5`, e.Samples)
	}
	if e.StartDate.IsZero() {
		e.StartDate = db.NewDate(1998, 1, 2)
	}
	return nil
}

func (e *Beta) Name() string { return "beta" }

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
		case "test":
			e.Config = &TestExperimentConfig{}
		case "hold":
			e.Config = &Hold{}
		case "distribution":
			e.Config = &Distribution{}
		case "power distribution":
			e.Config = &PowerDist{}
		case "portfolio":
			e.Config = &Portfolio{}
		case "auto-correlation":
			e.Config = &AutoCorrelation{}
		case "beta":
			e.Config = &Beta{}
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
