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
	"encoding/json"
	"io"
	"os"
	"runtime"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/message"
	"github.com/stockparfait/stockparfait/stats"
)

// ExperimentConfig is a custom configuration for an experiment.
type ExperimentConfig interface {
	message.Message
	Name() string
}

// TestExperimentConfig is only used in tests.
type TestExperimentConfig struct {
	Grade  float64 `json:"grade" default:"2.0"`
	Passed bool    `json:"passed"`
	Graph  string  `json:"graph" required:"true"`
}

var _ ExperimentConfig = &TestExperimentConfig{}

// Name implements ExperimentConfig.
func (t *TestExperimentConfig) Name() string { return "test" }

// InitMessage implements message.Message.
func (t *TestExperimentConfig) InitMessage(js interface{}) error {
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

func (p *HoldPosition) InitMessage(js interface{}) error {
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

// Hold "experiment" configuration.
type Hold struct {
	Reader         *db.Reader     `json:"data" required:"true"`
	Positions      []HoldPosition `json:"positions"`
	PositionsGraph string         `json:"positions graph"` // plots per position
	PositionsAxis  string         `json:"positions axis" choices:"left,right" default:"right"`
	TotalGraph     string         `json:"total graph"` // plot portfolio value
	TotalAxis      string         `json:"total axis" choices:"left,right" default:"right"`
}

var _ ExperimentConfig = &Hold{}

func (h *Hold) InitMessage(js interface{}) error {
	return errors.Annotate(message.Init(h, js), "failed to parse Hold config")
}

func (h *Hold) Name() string { return "hold" }

// AnalyticalDistribution configures the type and parameters of a distibution.
type AnalyticalDistribution struct {
	Name      string        `json:"name" required:"true" choices:"t,normal"`
	Mean      float64       `json:"mean" default:"0.0"`
	MAD       float64       `json:"MAD" default:"1.0"`
	Alpha     float64       `json:"alpha" default:"3.0"`    // T dist. parameter
	Compound  int           `json:"compound" default:"1"`   // sum of N samples
	Normalize bool          `json:"normalize"`              // divide by Compound
	Samples   int           `json:"samples" default:"1000"` // #samples for estimating statistics
	Buckets   stats.Buckets `json:"buckets"`
}

var _ message.Message = &AnalyticalDistribution{}

func (d *AnalyticalDistribution) InitMessage(js interface{}) error {
	if err := message.Init(d, js); err != nil {
		return errors.Annotate(err, "failed to init AnalyticalDistribution")
	}
	if d.Name == "t" && d.Alpha <= 1.0 {
		return errors.Reason("T-distribution requires alpha=%f > 1.0", d.Alpha)
	}
	if d.MAD <= 0.0 {
		return errors.Reason("MAD=%f must be positive", d.MAD)
	}
	if d.Compound < 1 {
		return errors.Reason("Compound=%d must be >= 1", d.Compound)
	}
	if d.Samples < 1 {
		return errors.Reason("Samples=%d must be >= 1", d.Samples)
	}
	return nil
}

// FindMin configures parameters for finding the minimum of a function.  The
// algorithm assumes that the function is monotone around the single minimum
// within the interval.
type FindMin struct {
	MinX          float64 `json:"min x" required:"true"`
	MaxX          float64 `json:"max x" required:"true"`
	Epsilon       float64 `json:"epsilon" default:"0.01"` // min size of the search interval
	MaxIterations int     `json:"max iterations" default:"1000"`
}

var _ message.Message = &FindMin{}

func (f *FindMin) InitMessage(js interface{}) error {
	if err := message.Init(f, js); err != nil {
		return errors.Annotate(err, "failed to init FindMin")
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
	return nil
}

// DistributionPlot is a config for a single graph in the Distribution
// experiment.
type DistributionPlot struct {
	Graph          string                  `json:"graph" required:"true"`
	CountsGraph    string                  `json:"counts graph"` // plot buckets' counts
	Buckets        stats.Buckets           `json:"buckets"`
	ChartType      string                  `json:"chart type" choices:"line,bars" default:"line"`
	Normalize      bool                    `json:"normalize"`  // to mean=0, MAD=1
	UseMeans       bool                    `json:"use means"`  // use bucket means rather than middles
	KeepZeros      bool                    `json:"keep zeros"` // by default, skip y==0 points
	LogY           bool                    `json:"log Y"`      // plot log10(y)
	LeftAxis       bool                    `json:"left axis"`
	CountsLeftAxis bool                    `json:"counts left axis"`
	RefDist        *AnalyticalDistribution `json:"reference distribution"`
	AdjustRef      bool                    `json:"adjust reference distribution"`
	DeriveAlpha    *FindMin                `json:"derive alpha"`  // for ref. dist. from data
	IgnoreCounts   int                     `json:"ignore counts"` // when deriving alpha
	PlotMean       bool                    `json:"plot mean"`
	Percentiles    []float64               `json:"percentiles"` // in [0..100]
}

var _ message.Message = &DistributionPlot{}

func (dp *DistributionPlot) InitMessage(js interface{}) error {
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

func (e *Distribution) InitMessage(js interface{}) error {
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

type PowerDist struct {
	ID         string                 `json:"id"` // experiment ID, for multiple instances
	Dist       AnalyticalDistribution `json:"distribution"`
	SamplePlot *DistributionPlot      `json:"sample plot"` // sampled Dist

	// Graphs of statistics as functions of number of samples, up to Samples.
	// Select the number of Points to plot which are spread out logarithmically,
	// and optionally plot min/max values of samples between points.
	MeanGraph  string `json:"mean graph"`
	MADGraph   string `json:"MAD graph"`
	SigmaGraph string `json:"sigma graph"`
	Samples    int    `json:"samples" default:"10000"` // >= 3
	Points     int    `json:"points" default:"200"`    // >= 3
	PlotMinMax bool   `json:"plot min max"`
	// Distribution of derived statistics estimated from Samples, to estimate
	// confidence intervals of the statistics.
	MeanDist  *DistributionPlot `json:"mean distribution"`
	MADDist   *DistributionPlot `json:"MAD distribution"`
	SigmaDist *DistributionPlot `json:"sigma distribution"`
}

var _ message.Message = &PowerDist{}
var _ ExperimentConfig = &PowerDist{}

func (e *PowerDist) InitMessage(js interface{}) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init PowerDist")
	}
	if e.Samples < 3 {
		return errors.Reason("samples=%d must be >= 3", e.Samples)
	}
	if e.Points < 3 {
		return errors.Reason("points=%d must be >= 3", e.Points)
	}
	return nil
}

func (e *PowerDist) Name() string { return "power distribution" }

// ExpMap represents a Message which reads a single-element map {name:
// Experiment} and knows how to populate specific implementations of the
// Experiment interface.
type ExpMap struct {
	Config ExperimentConfig `json:"-"` // populated directly in Init
}

var _ message.Message = &ExpMap{}

func (e *ExpMap) InitMessage(js interface{}) error {
	m, ok := js.(map[string]interface{})
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
		default:
			return errors.Reason("unknown experiment %s", name)
		}
		return errors.Annotate(e.Config.InitMessage(jsConfig),
			"failed to parse experiment config")
	}
	return nil
}

// Graph is a config for a plot graph, a single canvas holding potentially
// multiple plots.
type Graph struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	XLabel    string `json:"x_label"`
	YLogScale bool   `json:"y_log_scale"`
}

var _ message.Message = &Graph{}

// InitMessage implements message.Message.
func (g *Graph) InitMessage(js interface{}) error {
	if err := message.Init(g, js); err != nil {
		return errors.Annotate(err, "cannot parse graph")
	}
	if g.ID == "" {
		return errors.Reason("graph must have a non-empty ID")
	}
	return nil
}

// Group is a config for a group of plots with a common X axis.
type Group struct {
	Timeseries bool     `json:"timeseries"`
	ID         string   `json:"id"`
	Title      string   `json:"title"` // default: same as ID
	XLogScale  bool     `json:"x_log_scale"`
	Graphs     []*Graph `json:"graphs"`
}

var _ message.Message = &Group{}

// InitMessage implements message.Message.
func (g *Group) InitMessage(js interface{}) error {
	if err := message.Init(g, js); err != nil {
		return errors.Annotate(err, "cannot parse group")
	}
	if g.ID == "" {
		return errors.Reason("group must have a non-empty ID")
	}
	if g.Title == "" {
		g.Title = g.ID
	}
	if len(g.Graphs) < 1 {
		return errors.Reason("at least one graph is required in group '%s'",
			g.ID)
	}
	if g.Timeseries && g.XLogScale {
		return errors.Reason("timeseries group '%s' cannot have log-scale X",
			g.ID)
	}
	return nil
}

// Config is the top-level configuration of the app.
type Config struct {
	Groups      []*Group  `json:"groups"`
	Experiments []*ExpMap `json:"experiments"`
}

var _ message.Message = &Config{}

func (c *Config) InitMessage(js interface{}) error {
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
	f, err := os.Open(configPath)
	if err != nil {
		return nil, errors.Annotate(err, "cannot open config file '%s'", configPath)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var jv interface{}
	if err := dec.Decode(&jv); err != nil && err != io.EOF {
		return nil, errors.Annotate(err, "failed to decode JSON in %s", configPath)
	}

	var c Config
	if err := c.InitMessage(jv); err != nil {
		return nil, errors.Annotate(err, "cannot interpret config '%s'", configPath)
	}
	return &c, nil
}
