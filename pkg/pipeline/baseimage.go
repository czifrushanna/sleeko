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
	"fmt"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
	"github.com/czifrushanna/sleeko/pkg/dfparse"
)

// BaseImageStageName is the configuration name of the BaseImage stage.
const BaseImageStageName = "baseimage"

// BaseImage measures base image candidates for the original's final stage
// and adds the table to the generate prompt, so the model chooses a base it
// can see measured instead of inventing a tag (W2). It must run before
// generate. Whenever there is no table to offer — no scanner configured, a
// base it cannot resolve, nothing newer in the family — it says why in a
// diagnostic and the prompt stays as it is.
type BaseImage struct {
	proposer *baseimage.Proposer
}

func (b *BaseImage) Name() string { return BaseImageStageName }

func (b *BaseImage) Run(ctx context.Context, st *State) error {
	skip := func(format string, args ...any) error {
		st.Diagnostics = append(st.Diagnostics, Diagnostic{Stage: BaseImageStageName, Message: "no base image table: " + fmt.Sprintf(format, args...)})
		return nil
	}
	if b.proposer == nil {
		return skip("no scanner configured")
	}
	base, err := FinalBase(st.Original)
	if err != nil {
		return skip("%v", err)
	}
	table, err := b.proposer.Propose(ctx, base)
	if err != nil {
		return skip("%v", err)
	}
	for _, d := range table.Dropped {
		st.Diagnostics = append(st.Diagnostics, Diagnostic{Stage: BaseImageStageName, Message: "candidate not measured: " + d})
	}
	if len(table.Candidates) == 0 {
		return skip("%s has no newer image in its family", base)
	}
	st.BaseTable = &table
	st.PromptSections = append(st.PromptSections, table.Render())
	return nil
}

// FinalBase is the image the final stage is built on, following FROM <stage>
// back to an image. The pipeline only gets the Dockerfile, not a build
// target, so the final stage is the one that ships.
func FinalBase(dockerfile string) (string, error) {
	return StageBase(dockerfile, "")
}

// StageBase is the image the named stage is built on (the final stage when
// target is empty), following FROM <stage> back to an image. The harness
// knows each item's build target and uses it to tell which base C ships.
func StageBase(dockerfile, target string) (string, error) {
	_, ref, err := baseRef(dockerfile, target)
	if err != nil {
		return "", err
	}
	return ref.Image, nil
}

// baseRef is the FROM that names the image the named stage (the final stage
// when target is empty) is built on, following FROM <stage> back to an image.
func baseRef(dockerfile, target string) (*dfparse.File, dfparse.FromRef, error) {
	f, err := dfparse.Parse(dockerfile)
	if err != nil {
		return nil, dfparse.FromRef{}, err
	}
	refs, err := f.FromRefs(nil)
	if err != nil {
		return nil, dfparse.FromRef{}, err
	}
	i := len(refs) - 1
	if target != "" {
		i = -1
		for j, s := range f.Stages {
			if s.Name == strings.ToLower(target) {
				i = j
			}
		}
		if i < 0 {
			return nil, dfparse.FromRef{}, fmt.Errorf("no stage named %s", target)
		}
	}
	for refs[i].BaseStage >= 0 {
		i = refs[i].BaseStage
	}
	switch r := refs[i]; {
	case r.Scratch:
		return nil, dfparse.FromRef{}, fmt.Errorf("the final stage is built FROM scratch")
	case len(r.Unresolved) > 0:
		return nil, dfparse.FromRef{}, fmt.Errorf("the final stage's base %s uses unresolved variables", r.Raw)
	default:
		return f, r, nil
	}
}
