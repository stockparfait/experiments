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

package assets

import (
	"testing"

	"github.com/stockparfait/experiments/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigs(t *testing.T) {
	t.Parallel()

	configs := []string{
		"Distribution-all-stocks-with-samples.json",
		"GOOG-exp-buckets.json",
		"GOOG-linear-buckets.json",
		"GOOG-vs-TSLA-normalized.json",
		"GOOG-vs-TSLA.json",
		"Vol-1M-stocks.json",
		"all-stocks-normal.json",
		"by-sectors.json",
		"by-volume.json",
		"by-years.json",
	}

	Convey("Configs parse successfully", t, func() {
		for _, conf := range configs {
			_, err := config.Load(conf)
			So(err, ShouldBeNil)
		}
	})
}
