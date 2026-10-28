// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
)

func TestRecordedTables(t *testing.T) {
	table := func(cur string, v int) *BaseChoice {
		return &BaseChoice{Table: baseimage.Table{
			Current:    baseimage.Measurement{Ref: cur, V: 50},
			Candidates: []baseimage.Measurement{{Ref: cur + "-new", V: v}},
		}}
	}
	recs := []Record{
		{Item: "a", Repetition: 1, BaseImage: table("python:3.12-slim", 10)},
		{Item: "a", Repetition: 2, BaseImage: table("python:3.12-slim", 10)},
		{Item: "b", Repetition: 1},
	}
	tables, err := LoadRecordedTables(recs)
	require.NoError(t, err)

	got, err := tables.Propose(context.Background(), "python:3.12-slim")
	require.NoError(t, err)
	require.Equal(t, table("python:3.12-slim", 10).Table, got)

	_, err = tables.Propose(context.Background(), "node:20")
	require.EqualError(t, err, "the replayed run offered no table for node:20")

	_, err = LoadRecordedTables(append(recs, Record{Item: "c", BaseImage: table("python:3.12-slim", 20)}))
	require.EqualError(t, err, "the records hold different tables for python:3.12-slim")
}
