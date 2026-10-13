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
	"path"
	"slices"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
)

// RuntimeContract is how the final image is meant to be run, as far as the
// Dockerfile decides it. Anything the Dockerfile leaves to the base image is
// reported as unset, since the base image is not visible here.
type RuntimeContract struct {
	// Entrypoint and Cmd are nil when inherited from the base image. A non-nil
	// Cmd with no Args is a CMD that ENTRYPOINT cleared: Docker drops an
	// inherited CMD when a stage sets ENTRYPOINT without setting CMD.
	Entrypoint *Command
	Cmd        *Command
	// Expose lists the ports from every EXPOSE, each once, in the order they
	// first appear. BuildKit sorts the ports within a single EXPOSE.
	Expose []string
	// Workdir is the effective working directory, relative WORKDIRs resolved
	// against earlier ones; empty when inherited.
	Workdir string
	// User is the effective USER as written; empty when inherited.
	User string
}

// Command is an ENTRYPOINT or CMD.
type Command struct {
	// Args are the exec-form arguments, or the single shell-form command line.
	Args []string
	// Shell reports the shell form, which Docker runs through /bin/sh -c.
	Shell bool
}

// RuntimeContract returns the final stage's runtime contract. Settings
// carried over from earlier stages that the final stage builds on (FROM
// <stage>) are included, as Docker includes them.
func (f *File) RuntimeContract() (RuntimeContract, error) {
	refs, err := f.FromRefs(nil)
	if err != nil {
		return RuntimeContract{}, err
	}

	// The final stage and the stages it builds on, oldest first.
	var chain []int
	for i := len(f.Stages) - 1; i >= 0; i = refs[i].BaseStage {
		chain = append([]int{i}, chain...)
	}

	var rc RuntimeContract
	for _, si := range chain {
		cmdSetHere := false
		for _, in := range f.Stages[si].Instructions {
			switch c := in.cmd.(type) {
			case *instructions.EntrypointCommand:
				rc.Entrypoint = newCommand(c.ShellDependantCmdLine)
				if !cmdSetHere {
					rc.Cmd = &Command{}
				}
			case *instructions.CmdCommand:
				rc.Cmd = newCommand(c.ShellDependantCmdLine)
				cmdSetHere = true
			case *instructions.ExposeCommand:
				for _, p := range c.Ports {
					if !slices.Contains(rc.Expose, p) {
						rc.Expose = append(rc.Expose, p)
					}
				}
			case *instructions.WorkdirCommand:
				if path.IsAbs(c.Path) || rc.Workdir == "" {
					rc.Workdir = path.Clean(c.Path)
				} else {
					rc.Workdir = path.Join(rc.Workdir, c.Path)
				}
			case *instructions.UserCommand:
				rc.User = c.User
			}
		}
	}
	return rc, nil
}

func newCommand(c instructions.ShellDependantCmdLine) *Command {
	return &Command{Args: slices.Clone(c.CmdLine), Shell: c.PrependShell}
}
