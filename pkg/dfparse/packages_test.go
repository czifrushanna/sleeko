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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageInstalls_Fixtures(t *testing.T) {
	cases := []struct {
		name string
		file func(t *testing.T) *File
		want []PackageInstall
	}{
		{
			name: "single stage: env prefix, option values, quoted pip specs, requirements file skipped",
			file: func(t *testing.T) *File { return parseFixture(t, "single.Dockerfile") },
			want: []PackageInstall{
				{Manager: ManagerApt, Stage: 0, Line: 7, Name: "curl", Version: "7.88.1-10+deb12u6", Spec: "curl=7.88.1-10+deb12u6"},
				{Manager: ManagerApt, Stage: 0, Line: 7, Name: "ca-certificates", Spec: "ca-certificates"},
				{Manager: ManagerPip, Stage: 0, Line: 15, Name: "gunicorn", Version: "22.0.0", Spec: "gunicorn==22.0.0"},
				{Manager: ManagerPip, Stage: 0, Line: 15, Name: "requests", Spec: "requests[socks]>=2.31"},
				{Manager: ManagerPip, Stage: 0, Line: 15, Name: "uvicorn", Version: "0.30.1", Spec: "uvicorn===0.30.1"},
			},
		},
		{
			name: "multi-stage: stage index recorded",
			file: func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			want: []PackageInstall{
				{Manager: ManagerApt, Stage: 2, Line: 17, Name: "tzdata", Spec: "tzdata"},
			},
		},
		{
			name: "heredoc script with continuations, python -m pip with redirections",
			file: func(t *testing.T) *File { return parseFixture(t, "heredoc.Dockerfile") },
			want: []PackageInstall{
				{Manager: ManagerApt, Stage: 0, Line: 4, Name: "python3-pip", Spec: "python3-pip"},
				{Manager: ManagerApt, Stage: 0, Line: 4, Name: "git", Version: "1:2.43.0-1ubuntu7", Spec: "git=1:2.43.0-1ubuntu7"},
				{Manager: ManagerPip, Stage: 0, Line: 13, Name: "flask", Version: "3.0.3", Spec: "flask==3.0.3"},
			},
		},
		{
			name: "the repo's own Dockerfile installs nothing",
			file: parseRepoDockerfile,
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, c.file(t).PackageInstalls())
		})
	}
}

// pkg is shorthand for an expected install in TestPackageInstalls_CommandLines,
// where stage and line are always the same.
func pkg(manager, name, version, spec string) PackageInstall {
	return PackageInstall{Manager: manager, Stage: 0, Line: 2, Name: name, Version: version, Spec: spec}
}

func TestPackageInstalls_CommandLines(t *testing.T) {
	cases := []struct {
		name string
		run  string
		want []PackageInstall
	}{
		// apt
		{"apt-get with options before the subcommand", "apt-get -qq -y install curl", []PackageInstall{pkg(ManagerApt, "curl", "", "curl")}},
		{"apt rather than apt-get", "apt install -y vim=2:9.0.1378-2", []PackageInstall{pkg(ManagerApt, "vim", "2:9.0.1378-2", "vim=2:9.0.1378-2")}},
		{"sudo and its options", "sudo -E apt-get install -y git", []PackageInstall{pkg(ManagerApt, "git", "", "git")}},
		{"env wrapper with an assignment", "env DEBIAN_FRONTEND=noninteractive apt-get install -y tzdata", []PackageInstall{pkg(ManagerApt, "tzdata", "", "tzdata")}},
		{"absolute program path", "/usr/bin/apt-get install -y make", []PackageInstall{pkg(ManagerApt, "make", "", "make")}},
		{"target release option and a /release suffix", "apt-get install -t bookworm-backports -y foo/bookworm-backports", []PackageInstall{pkg(ManagerApt, "foo", "", "foo/bookworm-backports")}},
		{"architecture qualifier", "apt-get install -y libc6:i386=2.36-9", []PackageInstall{pkg(ManagerApt, "libc6", "2.36-9", "libc6:i386=2.36-9")}},
		{"--only-upgrade is still an install", "apt-get install -y --only-upgrade openssl=3.0.15-1~deb12u1", []PackageInstall{pkg(ManagerApt, "openssl", "3.0.15-1~deb12u1", "openssl=3.0.15-1~deb12u1")}},
		{"inside an if", "if apt-get install -y jq; then echo ok; fi", []PackageInstall{pkg(ManagerApt, "jq", "", "jq")}},
		{"output redirected", "apt-get install -y curl > /dev/null 2>&1", []PackageInstall{pkg(ManagerApt, "curl", "", "curl")}},
		{"redirection target attached", "apt-get install -y curl >/tmp/log", []PackageInstall{pkg(ManagerApt, "curl", "", "curl")}},
		{"local .deb, variables and globs skipped", "apt-get install -y ./pkg.deb /tmp/x.deb $EXTRA \"${PKGS}\" 'libfoo*' wget", []PackageInstall{pkg(ManagerApt, "wget", "", "wget")}},
		{"other subcommands install nothing", "apt-get update && apt-get remove -y curl && apt-get upgrade -y", nil},
		{"commented out", "true # apt-get install -y curl", nil},
		{"mentioned, not run", "echo 'apt-get install -y curl'", nil},

		// pip
		{"pip3 with an absolute path", "/opt/venv/bin/pip3 install numpy==1.26.4", []PackageInstall{pkg(ManagerPip, "numpy", "1.26.4", "numpy==1.26.4")}},
		{"versioned pip", "pip3.11 install six", []PackageInstall{pkg(ManagerPip, "six", "", "six")}},
		{"python -m pip with interpreter options", "python3.12 -I -m pip install -U pip==24.2 setuptools", []PackageInstall{pkg(ManagerPip, "pip", "24.2", "pip==24.2"), pkg(ManagerPip, "setuptools", "", "setuptools")}},
		{"python -m something else", "python -m venv /venv", nil},
		{"spaces inside a quoted spec", `pip install "Django == 5.0.6"`, []PackageInstall{pkg(ManagerPip, "Django", "5.0.6", "Django == 5.0.6")}},
		{"environment marker", `pip install "tomli==2.0.1; python_version < '3.11'"`, []PackageInstall{pkg(ManagerPip, "tomli", "2.0.1", "tomli==2.0.1; python_version < '3.11'")}},
		{"ranges and wildcards are not exact pins", "pip install 'a>=1,<2' b==1.* c~=1.4 d!=2", []PackageInstall{
			pkg(ManagerPip, "a", "", "a>=1,<2"), pkg(ManagerPip, "b", "", "b==1.*"),
			pkg(ManagerPip, "c", "", "c~=1.4"), pkg(ManagerPip, "d", "", "d!=2"),
		}},
		{"options with values skipped", "pip install -r req.txt -c constraints.txt -i https://pypi.example/simple --target /deps flask", []PackageInstall{pkg(ManagerPip, "flask", "", "flask")}},
		{"options with = values", "pip install --index-url=https://pypi.example/simple --no-deps attrs==23.2.0", []PackageInstall{pkg(ManagerPip, "attrs", "23.2.0", "attrs==23.2.0")}},
		{"paths, archives, URLs and editables skipped", "pip install . ./pkg dir/pkg x-1.0-py3-none-any.whl x.tar.gz git+https://github.com/o/r.git -e ./local", nil},
		{"pip download is not an install", "pip download requests==2.0", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse("FROM debian\nRUN " + c.run + "\n")
			require.NoError(t, err)
			require.Equal(t, c.want, f.PackageInstalls())
		})
	}
}

func TestPackageInstalls_ExecForm(t *testing.T) {
	f, err := Parse("FROM python:3.12\nRUN [\"pip\", \"install\", \"httpx==0.27.0\"]\n")
	require.NoError(t, err)
	require.Equal(t, []PackageInstall{pkg(ManagerPip, "httpx", "0.27.0", "httpx==0.27.0")}, f.PackageInstalls())
}

func TestSplitCommands(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   [][]string
	}{
		{"separators", "a 1 && b 2 || c; d | e & f", [][]string{{"a", "1"}, {"b", "2"}, {"c"}, {"d"}, {"e"}, {"f"}}},
		{"quotes kept together and removed", `x "a b" 'c d' e"f g"h`, [][]string{{"x", "a b", "c d", "ef gh"}}},
		{"separators inside quotes are literal", `echo "a && b" 'c; d'`, [][]string{{"echo", "a && b", "c; d"}}},
		{"line continuation", "a \\\n  b\nc", [][]string{{"a", "b"}, {"c"}}},
		{"escaped space", `a b\ c`, [][]string{{"a", "b c"}}},
		{"escapes inside double quotes", `a "x\"y\\z\$"`, [][]string{{"a", `x"y\z$`}}},
		{"comment to end of line", "a # b && c\nd", [][]string{{"a"}, {"d"}}},
		{"# inside a word is not a comment", "a b#c", [][]string{{"a", "b#c"}}},
		{"subshell", "(cd /x && make)", [][]string{{"cd", "/x"}, {"make"}}},
		{"empty quoted word", `a "" b`, [][]string{{"a", "", "b"}}},
		{"empty", "  \n ; ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, splitCommands(c.script))
		})
	}
}
