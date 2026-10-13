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

package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordingStage appends its name to a shared log and to the candidate, so a
// test can see both which stages ran and in what order they saw the state.
type recordingStage struct {
	name string
	log  *[]string
	err  error
}

func (r *recordingStage) Name() string { return r.name }

func (r *recordingStage) Run(_ context.Context, st *State) error {
	*r.log = append(*r.log, r.name)
	if r.err != nil {
		return r.err
	}
	st.Candidate += r.name + ";"
	return nil
}

func TestPipeline_RunsStagesInOrderOnSharedState(t *testing.T) {
	var log []string
	p := NewPipeline(
		&recordingStage{name: "a", log: &log},
		&recordingStage{name: "b", log: &log},
		&recordingStage{name: "c", log: &log},
	)

	st := &State{}
	require.NoError(t, p.Run(context.Background(), st))
	require.Equal(t, []string{"a", "b", "c"}, log)
	require.Equal(t, "a;b;c;", st.Candidate)
	require.Equal(t, []string{"a", "b", "c"}, p.Names())
}

func TestPipeline_StopsAtFirstErrorAndNamesTheStage(t *testing.T) {
	boom := errors.New("boom")
	var log []string
	p := NewPipeline(
		&recordingStage{name: "a", log: &log},
		&recordingStage{name: "b", log: &log, err: boom},
		&recordingStage{name: "c", log: &log},
	)

	st := &State{}
	err := p.Run(context.Background(), st)

	var stageErr *StageError
	require.ErrorAs(t, err, &stageErr)
	require.Equal(t, "b", stageErr.Stage)
	require.ErrorIs(t, err, boom)
	require.Equal(t, "stage b: boom", err.Error())
	require.Equal(t, []string{"a", "b"}, log, "stages after the failing one must not run")
	require.Equal(t, "a;", st.Candidate)
}

func TestPipeline_EmptyRunsNothing(t *testing.T) {
	st := &State{Original: "FROM scratch\n"}
	require.NoError(t, NewPipeline().Run(context.Background(), st))
	require.Empty(t, st.Candidate)
}

func TestParseNames(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		want    []string
		wantErr string
	}{
		{"default", "generate", []string{"generate"}, ""},
		{"whitespace and empty entries", " generate ,, ", []string{"generate"}, ""},
		{"repeated stage is kept", "generate,generate", []string{"generate", "generate"}, ""},
		{"unknown", "generate,nope", nil, `unknown stage "nope" (known: generate)`},
		{"empty", "", nil, "no stages configured"},
		{"only separators", " , ", nil, "no stages configured"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseNames(c.spec)
			if c.wantErr != "" {
				require.EqualError(t, err, c.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestNew(t *testing.T) {
	p, err := New(DefaultNames(), Deps{})
	require.NoError(t, err)
	require.Equal(t, []string{GenerateStageName}, p.Names())

	_, err = New([]string{"nope"}, Deps{})
	require.EqualError(t, err, `unknown stage "nope"`)

	_, err = New(nil, Deps{})
	require.EqualError(t, err, "no stages configured")
}

func TestKnownNames(t *testing.T) {
	require.Equal(t, []string{GenerateStageName}, KnownNames())
}
