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
	"os"
	"path/filepath"
	"testing"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMain(t *testing.T) {
	t.Parallel()
	tmpdir, tmpdirErr := os.MkdirTemp("", "test_exp")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("parseFlags", t, func() {
		flags, err := parseFlags([]string{
			"-conf", "c.json", "-cache", "path/to/cache", "-log-level", "warning"})
		So(err, ShouldBeNil)
		So(flags.DBDir, ShouldEqual, "path/to/cache")
		So(flags.Config, ShouldEqual, "c.json")
		So(flags.LogLevel, ShouldEqual, logging.Warning)
	})

	Convey("run a test experiment end to end", t, func() {
		confJSON := `
{
  "groups": [{"id": "xy", "graphs": [{"id": "r1"}]}],
  "experiments": [{"test": {"graph": "r1"}}]
}`
		confPath := filepath.Join(tmpdir, "config.json")
		So(testutil.WriteFile(confPath, confJSON), ShouldBeNil)

		dataJs := filepath.Join(tmpdir, "data.js")
		dataJSON := filepath.Join(tmpdir, "data.json")

		flags, err := parseFlags([]string{
			"-conf", confPath, "-js", dataJs, "-json", dataJSON})
		So(err, ShouldBeNil)

		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		canvas := plot.NewCanvas()
		values := make(experiments.Values)
		ctx = plot.Use(ctx, canvas)
		ctx = experiments.UseValues(ctx, values)

		So(run(ctx, flags), ShouldBeNil)

		So(values, ShouldResemble, map[string]string{
			"grade": "2",
			"test":  "failed",
		})

		expectedJSON := `{"Groups":[{"Kind":"KindXY","Title":"xy","XLogScale":false,"Graphs":[{"Kind":"KindXY","Title":"","XLabel":"","YLogScale":false,"Plots":[{"Kind":"KindXY","X":[1,2],"Y":[21.5,42],"YLabel":"values","Legend":"Unnamed","ChartType":"ChartLine","LeftAxis":false}]}],"MinX":1,"MaxX":2}]}`

		So(testutil.ReadFile(dataJSON), ShouldContainSubstring, expectedJSON)
		So(testutil.ReadFile(dataJs), ShouldContainSubstring, "var DATA = "+expectedJSON)

	})
}
