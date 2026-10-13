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
	"regexp"
	"strings"
)

// Package managers PackageInstalls recognises.
const (
	ManagerApt = "apt"
	ManagerPip = "pip"
)

// PackageInstall is one package named on an apt or pip install command line.
type PackageInstall struct {
	// Manager is ManagerApt or ManagerPip.
	Manager string
	// Stage is the index of the stage the RUN is in.
	Stage int
	// Line is the RUN instruction's first line.
	Line int
	// Name is the package name: for apt without ":arch" or "/release", for pip
	// without extras, both as written otherwise (no case or PEP 503
	// normalisation).
	Name string
	// Version is the pinned version: apt's "name=version", pip's "==" or "===".
	// Empty when the package is not pinned to one exact version.
	Version string
	// Spec is the argument as written, quotes removed.
	Spec string
}

// PackageInstalls returns the packages named by `apt-get install`,
// `apt install`, `pip install` and `python -m pip install` in RUN commands,
// in source order. Arguments that are not package names — local .deb and
// wheel files, paths, URLs, shell variables, requirements files — are
// skipped. Packages installed some other way (from a requirements file, a
// variable, a script) are not visible here.
func (f *File) PackageInstalls() []PackageInstall {
	var out []PackageInstall
	for _, rc := range f.RunCommands() {
		for _, script := range rc.scripts() {
			for _, words := range splitCommands(script) {
				for _, p := range installsIn(words) {
					p.Stage, p.Line = rc.Stage, rc.StartLine
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// installsIn recognises one simple command as an install and returns its
// packages.
func installsIn(words []string) []PackageInstall {
	words = dropRedirections(stripCommandPrefixes(words))
	if len(words) == 0 {
		return nil
	}

	prog := path.Base(words[0])
	switch {
	case prog == "apt-get" || prog == "apt":
		return aptInstalls(words[1:])
	case pipProgram.MatchString(prog):
		return pipInstalls(words[1:])
	case pythonProgram.MatchString(prog):
		for i := 1; i+1 < len(words); i++ {
			if words[i] == "-m" {
				if words[i+1] == "pip" {
					return pipInstalls(words[i+2:])
				}
				return nil
			}
		}
	}
	return nil
}

var (
	pipProgram    = regexp.MustCompile(`^pip(\d+(\.\d+)?)?$`)
	pythonProgram = regexp.MustCompile(`^python(\d+(\.\d+)?)?$`)
	assignment    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	// redirection matches a redirection word: the operator, optionally with a
	// file descriptor in front and the target attached ("2>/dev/null").
	redirection = regexp.MustCompile(`^(\d*|&)(>>?|<<?)(&?)(.*)$`)
)

// dropRedirections removes redirections, and the target word after an
// operator that stands alone ("> out.log"), so neither reads as an argument.
func dropRedirections(words []string) []string {
	out := words[:0:0]
	for i := 0; i < len(words); i++ {
		m := redirection.FindStringSubmatch(words[i])
		if m == nil {
			out = append(out, words[i])
			continue
		}
		if m[4] == "" {
			i++
		}
	}
	return out
}

// shellKeywords can precede a command in the same simple-command word list
// once the script is split on separators ("if apt-get install ...; then").
var shellKeywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "do": true,
	"while": true, "until": true, "!": true, "{": true,
}

// commandWrappers run the command that follows them.
var commandWrappers = map[string]bool{
	"sudo": true, "env": true, "nice": true, "time": true, "command": true, "exec": true, "nohup": true,
}

// stripCommandPrefixes drops what can stand before the program name: shell
// keywords, variable assignments, and wrappers with their options.
func stripCommandPrefixes(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		switch {
		case shellKeywords[w], assignment.MatchString(w):
			words = words[1:]
		case commandWrappers[path.Base(w)]:
			words = words[1:]
			for len(words) > 0 && strings.HasPrefix(words[0], "-") {
				words = words[1:]
			}
		default:
			return words
		}
	}
	return words
}

// aptValueOptions take a separate value word.
var aptValueOptions = map[string]bool{
	"-o": true, "--option": true, "-t": true, "--target-release": true, "--default-release": true,
	"-c": true, "--config-file": true, "-a": true, "--host-architecture": true,
}

func aptInstalls(args []string) []PackageInstall {
	pkgs, ok := subcommandArgs(args, aptValueOptions)
	if !ok {
		return nil
	}

	var out []PackageInstall
	for _, spec := range pkgs {
		// "*" is an apt glob, never part of a package name.
		if !looksLikePackage(spec) || strings.HasSuffix(spec, ".deb") || strings.Contains(spec, "*") {
			continue
		}
		name, version, _ := strings.Cut(spec, "=")
		name, _, _ = strings.Cut(name, "/")
		name, _, _ = strings.Cut(name, ":")
		if name == "" {
			continue
		}
		out = append(out, PackageInstall{Manager: ManagerApt, Name: name, Version: version, Spec: spec})
	}
	return out
}

// pipValueOptions take a separate value word.
var pipValueOptions = map[string]bool{
	"-r": true, "--requirement": true, "-c": true, "--constraint": true, "-e": true, "--editable": true,
	"-i": true, "--index-url": true, "--extra-index-url": true, "-f": true, "--find-links": true,
	"-t": true, "--target": true, "--prefix": true, "--root": true, "--src": true,
	"--platform": true, "--python-version": true, "--implementation": true, "--abi": true,
	"--upgrade-strategy": true, "--progress-bar": true, "--log": true, "--log-file": true,
	"--cache-dir": true, "--trusted-host": true, "--proxy": true, "--timeout": true, "--retries": true,
	"--no-binary": true, "--only-binary": true, "-C": true, "--config-settings": true,
	"--global-option": true, "--report": true, "--python": true, "--exists-action": true,
	"--cert": true, "--client-cert": true, "--keyring-provider": true, "--use-feature": true,
	"--use-deprecated": true, "--root-user-action": true, "--group": true,
}

// pipRequirement splits a PEP 508 requirement into name, extras and the rest.
var pipRequirement = regexp.MustCompile(`^([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)(\[[^\]]*\])?(.*)$`)

func pipInstalls(args []string) []PackageInstall {
	pkgs, ok := subcommandArgs(args, pipValueOptions)
	if !ok {
		return nil
	}

	var out []PackageInstall
	for _, spec := range pkgs {
		if !looksLikePackage(spec) || strings.Contains(spec, "/") || isArchive(spec) {
			continue
		}
		req := strings.ReplaceAll(spec, " ", "")
		req, _, _ = strings.Cut(req, ";") // environment marker
		m := pipRequirement.FindStringSubmatch(req)
		if m == nil {
			continue
		}
		out = append(out, PackageInstall{Manager: ManagerPip, Name: m[1], Version: exactPipVersion(m[3]), Spec: spec})
	}
	return out
}

// isArchive reports a local distribution file passed to pip.
func isArchive(spec string) bool {
	for _, ext := range []string{".whl", ".tar.gz", ".tgz", ".zip"} {
		if strings.HasSuffix(spec, ext) {
			return true
		}
	}
	return false
}

// exactPipVersion returns the version a specifier pins exactly, or "".
func exactPipVersion(specifier string) string {
	v, ok := strings.CutPrefix(specifier, "===")
	if !ok {
		v, ok = strings.CutPrefix(specifier, "==")
	}
	if !ok || v == "" || strings.ContainsAny(v, ",*<>=!~") {
		return ""
	}
	return v
}

// subcommandArgs finds the subcommand in args and, when it is "install",
// returns the non-option words after it. Options are skipped wherever they
// appear, along with the value word of those in valueOpts.
func subcommandArgs(args []string, valueOpts map[string]bool) ([]string, bool) {
	var sub string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			if valueOpts[a] {
				i++
			}
			continue
		}
		if sub == "" {
			sub = a
			continue
		}
		rest = append(rest, a)
	}
	return rest, sub == "install"
}

// looksLikePackage rejects words that are not package names: paths, URLs and
// shell variables. "*" is left to the caller: a glob for apt, a version
// wildcard for pip ("==1.*").
func looksLikePackage(w string) bool {
	if w == "" || strings.ContainsAny(w, "$`") || strings.Contains(w, "://") {
		return false
	}
	return !strings.HasPrefix(w, ".") && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, "~")
}

// splitCommands splits shell text into simple commands, each a list of words
// with quotes removed. It understands quoting, backslash escapes and line
// continuations, comments, and the separators && || ; | & ( ) and newline —
// enough to find install commands, not a shell parser.
func splitCommands(script string) [][]string {
	var (
		cmds    [][]string
		words   []string
		word    strings.Builder
		inWord  bool
		single  bool
		double  bool
		runes   = []rune(script)
		endWord = func() {
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		}
		endCmd = func() {
			endWord()
			if len(words) > 0 {
				cmds = append(cmds, words)
				words = nil
			}
		}
	)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case single:
			if r == '\'' {
				single = false
			} else {
				word.WriteRune(r)
			}
		case double:
			switch {
			case r == '"':
				double = false
			case r == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`\n", runes[i+1]):
				if runes[i+1] != '\n' {
					word.WriteRune(runes[i+1])
				}
				i++
			default:
				word.WriteRune(r)
			}
		case r == '\\':
			if i+1 < len(runes) {
				if runes[i+1] != '\n' {
					word.WriteRune(runes[i+1])
					inWord = true
				}
				i++
			}
		case r == '\'':
			single, inWord = true, true
		case r == '"':
			double, inWord = true, true
		case r == '#' && !inWord:
			for i+1 < len(runes) && runes[i+1] != '\n' {
				i++
			}
		case r == '\n' || r == ';' || r == '&' || r == '|' || r == '(' || r == ')':
			endCmd()
		case r == ' ' || r == '\t' || r == '\r':
			endWord()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endCmd()
	return cmds
}
