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

package dfparse

import (
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
)

// RunCommand is one RUN instruction.
type RunCommand struct {
	// Stage is the index of the stage the instruction is in.
	Stage int
	// StartLine and EndLine span the instruction, heredoc bodies included.
	StartLine int
	EndLine   int
	// Exec reports the JSON (exec) form, which runs without a shell.
	Exec bool
	// Command is the command line with line continuations joined. For the exec
	// form it is the arguments joined with spaces. Flags such as --mount are
	// not part of it.
	Command string
	// Heredocs are the heredoc bodies the instruction carries, in order.
	Heredocs []Heredoc
}

// Heredoc is an inline file attached to an instruction.
type Heredoc struct {
	Name    string
	Content string
}

// Normalized is the command in a form for comparing RUN instructions across
// two Dockerfiles: every run of whitespace collapsed to one space, heredoc
// bodies appended one trimmed line at a time, blank lines dropped. It is for
// comparison only; quoting is not respected, so it is not runnable.
func (r RunCommand) Normalized() string {
	parts := []string{collapseSpace(r.Command)}
	for _, h := range r.Heredocs {
		for _, line := range strings.Split(h.Content, "\n") {
			if line = collapseSpace(line); line != "" {
				parts = append(parts, line)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// scripts is the text scanned for package installs: the command line and
// every heredoc body. A heredoc can also feed a non-shell interpreter
// (RUN python3 <<EOF), but install commands inside one are rare enough that
// telling them apart is not worth a guess.
func (r RunCommand) scripts() []string {
	out := []string{r.Command}
	for _, h := range r.Heredocs {
		out = append(out, h.Content)
	}
	return out
}

// RunCommands returns every RUN instruction, in source order. RUNs inside
// ONBUILD are not included: they do not run in this build.
func (f *File) RunCommands() []RunCommand {
	var out []RunCommand
	for _, st := range f.Stages {
		for _, in := range st.Instructions {
			c, ok := in.cmd.(*instructions.RunCommand)
			if !ok {
				continue
			}

			rc := RunCommand{
				Stage:     st.Index,
				StartLine: in.StartLine,
				EndLine:   in.EndLine,
				Exec:      !c.PrependShell,
				Command:   strings.TrimSpace(strings.Join(c.CmdLine, " ")),
			}
			for _, file := range c.Files {
				rc.Heredocs = append(rc.Heredocs, Heredoc{Name: file.Name, Content: file.Data})
			}
			out = append(out, rc)
		}
	}
	return out
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
