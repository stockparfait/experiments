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
	"testing"

	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/plot"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExperiments(t *testing.T) {
	t.Parallel()

	Convey("Experiments API works", t, func() {
		ctx := context.Background()
		canvas := plot.NewCanvas()
		values := make(Values)
		ctx = plot.Use(ctx, canvas)
		ctx = UseValues(ctx, values)

		_, err := plot.EnsureGraph(ctx, plot.KindXY, "main", "top")
		So(err, ShouldBeNil)

		Convey("for TestExperiment", func() {
			conf := config.TestExperimentConfig{
				Grade:  3.5,
				Passed: true,
				Graph:  "main",
			}
			testExp := TestExperiment{}
			So(testExp.Run(ctx, &conf), ShouldBeNil)
			So(canvas.Groups[0].Graphs[0].Plots[0].X, ShouldResemble, []float64{1.0, 2.0})
			So(values, ShouldResemble, map[string]string{"grade": "3.5", "test": "passed"})
		})
	})

}
