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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/autocorr"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/experiments/distribution"
	"github.com/stockparfait/experiments/hold"
	"github.com/stockparfait/experiments/portfolio"
	"github.com/stockparfait/experiments/powerdist"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/plot"
)

type Flags struct {
	DBDir        string // default: ~/.stockparfait/sharadar
	Config       string // required
	LogLevel     logging.Level
	DataJsPath   string // write data.js to this path
	DataJSONPath string // write data.json to this path
}

func parseFlags(args []string) (*Flags, error) {
	var flags Flags
	fs := flag.NewFlagSet("experiments", flag.ExitOnError)
	fs.StringVar(&flags.DBDir, "cache",
		filepath.Join(os.Getenv("HOME"), ".stockparfait", "sharadar"),
		"database path")
	fs.StringVar(&flags.Config, "conf", "", "configuration file (required)")
	flags.LogLevel = logging.Info
	fs.Var(&flags.LogLevel, "log-level", "Log level: debug, info, warning, error")
	fs.StringVar(&flags.DataJsPath, "js", "", "file to write 'data.js' plots")
	fs.StringVar(&flags.DataJSONPath, "json", "", "file to write 'data.json' plots")

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}
	if flags.Config == "" {
		return nil, errors.Reason("missing required -conf")
	}
	return &flags, err
}

func runExperiment(ctx context.Context, ec config.ExperimentConfig) error {
	var e experiments.Experiment
	switch ec.Name() {
	case "test":
		e = &experiments.TestExperiment{}
	case "hold":
		e = &hold.Hold{}
	case "distribution":
		e = &distribution.Distribution{}
	case "power distribution":
		e = &powerdist.PowerDist{}
	case "portfolio":
		e = &portfolio.Portfolio{}
	case "auto-correlation":
		e = &autocorr.AutoCorrelation{}
	default:
		return errors.Reason("unsupported experiment '%s'", ec.Name())
	}
	if err := e.Run(ctx, ec); err != nil {
		return errors.Annotate(err, "failed experiment '%s'", ec.Name())
	}
	return nil
}

func printValues(ctx context.Context) error {
	keys := []string{}
	values := experiments.GetValues(ctx)
	if values == nil {
		return errors.Reason("no values in context")
	}
	for k := range values {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		fmt.Printf("%s: %s\n", k, values[k])
	}
	return nil
}

func writePlots(ctx context.Context, flags *Flags) error {
	if flags.DataJsPath != "" {
		f, err := os.OpenFile(flags.DataJsPath,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return errors.Annotate(err, "cannot open file for writing :'%s'",
				flags.DataJsPath)
		}
		defer f.Close()

		if err := plot.WriteJS(ctx, f); err != nil {
			return errors.Annotate(err, "failed to write '%s'", flags.DataJsPath)
		}
	}
	if flags.DataJSONPath != "" {
		f, err := os.OpenFile(flags.DataJSONPath,
			os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return errors.Annotate(err, "cannot open file for writing :'%s'",
				flags.DataJSONPath)
		}
		defer f.Close()

		if err := plot.WriteJSON(ctx, f); err != nil {
			return errors.Annotate(err, "failed to write '%s'", flags.DataJSONPath)
		}
	}
	return nil
}

func run(ctx context.Context, flags *Flags) error {
	cfg, err := config.Load(flags.Config)
	if err != nil {
		return errors.Annotate(err, "failed to load config")
	}
	if err := plot.ConfigureGroups(ctx, cfg.Groups); err != nil {
		return errors.Annotate(err, "failed to add groups")
	}
	for _, e := range cfg.Experiments {
		if err := runExperiment(ctx, e.Config); err != nil {
			return errors.Annotate(err, "failed to run experiment '%s'",
				e.Config.Name())
		}
	}
	if err := printValues(ctx); err != nil {
		return errors.Annotate(err, "failed to print values")
	}
	if err := writePlots(ctx, flags); err != nil {
		return errors.Annotate(err, "failed to write plots")
	}
	return nil
}

// main should remain minimal, as it is not unit-tested due to os.Exit.
func main() {
	ctx := context.Background()
	flags, err := parseFlags(os.Args[1:])
	if err != nil {
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		logging.Errorf(ctx, "failed to parse flags:\n%s", err.Error())
		os.Exit(1)
	}
	ctx = logging.Use(ctx, logging.DefaultGoLogger(flags.LogLevel))
	canvas := plot.NewCanvas()
	values := make(experiments.Values)
	ctx = plot.Use(ctx, canvas)
	ctx = experiments.UseValues(ctx, values)

	if err := run(ctx, flags); err != nil {
		logging.Errorf(ctx, err.Error())
		os.Exit(1)
	}
}
