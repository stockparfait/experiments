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
	"os"
	"path/filepath"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/logging"
)

type Flags struct {
	DBDir    string // default: ~/.stockparfait/sharadar
	Config   string // required
	LogLevel logging.Level
}

func parseFlags(args []string) (*Flags, error) {
	var flags Flags
	fs := flag.NewFlagSet("sharadar", flag.ExitOnError)
	fs.StringVar(&flags.DBDir, "cache",
		filepath.Join(os.Getenv("HOME"), ".stockparfait", "sharadar"),
		"database path")
	fs.StringVar(&flags.Config, "conf", "", "configuration file (required)")
	flags.LogLevel = logging.Info
	fs.Var(&flags.LogLevel, "log-level", "Log level: debug, info, warning, error")

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}
	if flags.Config == "" {
		return nil, errors.Reason("missing required -conf")
	}
	return &flags, err
}

func run(ctx context.Context, flags *Flags) error {
	logging.Infof(ctx, "nothing to do")
	return nil
}

func main() {
	ctx := context.Background()
	flags, err := parseFlags(os.Args[1:])
	if err != nil {
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		logging.Errorf(ctx, "failed to parse flags:\n%s", err.Error())
		os.Exit(1)
	}
	ctx = logging.Use(ctx, logging.DefaultGoLogger(flags.LogLevel))

	if err := run(ctx, flags); err != nil {
		logging.Errorf(ctx, err.Error())
		os.Exit(1)
	}
}
