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

package powerdist

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDistribution(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := ioutil.TempDir("", "test_powerdist")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("Power distribution experiment works", t, func() {
		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		canvas := plot.NewCanvas()
		values := make(experiments.Values)
		ctx = plot.Use(ctx, canvas)
		ctx = experiments.UseValues(ctx, values)

		Convey("minimal config", func() {
			var cfg config.PowerDist
			JSConfig := `
{
  "distribution": {"name": "t"}
}
`
			So(cfg.InitMessage(testutil.JSON(JSConfig)), ShouldBeNil)
			var pd PowerDist
			So(pd.Run(ctx, &cfg), ShouldBeNil)
		})

		Convey("with all plots", func() {
			var cfg config.PowerDist
			JSConfig := `
{
  "distribution": {
    "name": "t",
    "buckets": {"n": 5},
    "samples": 10
  },
  "sample plot": {
    "graph": "dist"
  },
  "cumulative mean": {
    "graph": "samples",
    "percentiles": [5, 95],
    "plot expected": true
  },
  "cumulative MAD": {
    "graph": "samples",
    "percentiles": [5, 95],
    "plot expected": true
  },
  "cumulative sigma": {
    "graph": "samples",
    "percentiles": [5, 95],
    "plot expected": true
  },
  "cumulative samples": 10,
  "mean distribution": {
    "graph": "means"
  },
  "MAD distribution": {
    "graph": "mads"
  },
  "sigma distribution": {
    "graph": "sigmas"
  },
  "alpha distribution": {
    "graph": "alphas"
  },
  "statistic samples": 10
}
`
			distGraph, err := canvas.EnsureGraph(plot.KindXY, "dist", "group")
			So(err, ShouldBeNil)

			samplesGraph, err := canvas.EnsureGraph(plot.KindXY, "samples", "group")
			So(err, ShouldBeNil)

			meansGraph, err := canvas.EnsureGraph(plot.KindXY, "means", "group")
			So(err, ShouldBeNil)

			madsGraph, err := canvas.EnsureGraph(plot.KindXY, "mads", "group")
			So(err, ShouldBeNil)

			sigmasGraph, err := canvas.EnsureGraph(plot.KindXY, "sigmas", "group")
			So(err, ShouldBeNil)

			alphasGraph, err := canvas.EnsureGraph(plot.KindXY, "alphas", "group")
			So(err, ShouldBeNil)

			So(cfg.InitMessage(testutil.JSON(JSConfig)), ShouldBeNil)
			var pd PowerDist
			So(pd.Run(ctx, &cfg), ShouldBeNil)
			So(len(distGraph.Plots), ShouldEqual, 1)
			So(len(samplesGraph.Plots), ShouldEqual, 12) // 4 for each statistic
			So(len(meansGraph.Plots), ShouldEqual, 1)
			So(len(madsGraph.Plots), ShouldEqual, 1)
			So(len(sigmasGraph.Plots), ShouldEqual, 1)
			So(len(alphasGraph.Plots), ShouldEqual, 1)
		})
	})
}
