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

package score

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
)

func mustParse(t *testing.T, src string) *dfparse.File {
	t.Helper()
	g, f := CheckParse(src)
	require.Equal(t, Pass, g.Status, g.Reason)
	return f
}

func TestCheckParse(t *testing.T) {
	g, f := CheckParse("FROM alpine:3.20\nRUN true\n")
	require.Equal(t, Gate{ID: G1Parse, Status: Pass}, g)
	require.NotNil(t, f)

	g, f = CheckParse("FROM alpine\nCOPYY a b\n")
	require.Equal(t, G1Parse, g.ID)
	require.Equal(t, Fail, g.Status)
	require.Contains(t, g.Reason, "line 2")
	require.Nil(t, f)

	g, _ = CheckParse("")
	require.Equal(t, Fail, g.Status)
}

func TestCheckNoInvention(t *testing.T) {
	const original = "FROM python:3.11\nCOPY requirements.txt .\nCOPY src/ ./src/\nCOPY config/*.yaml /etc/app/\n"
	cases := []struct {
		name       string
		o, c       string
		wantStatus Status
		wantReason string
	}{
		{"identical", original, original, Pass, ""},
		{"same paths written differently", original, "FROM python:3.12\nCOPY ./requirements.txt /app/\nCOPY src /app/src\n", Pass, ""},
		{"a file inside a copied directory", original, "FROM python:3.12\nCOPY src/app.py /app/\n", Pass, ""},
		{"a file matching a copied glob", original, "FROM python:3.12\nCOPY config/prod.yaml /etc/app/\n", Pass, ""},
		{"everything is inside COPY . .", "FROM node:20\nCOPY . .\n", "FROM node:22\nCOPY package.json package-lock.json ./\nCOPY . .\n", Pass, ""},
		{"COPY --from is allowed", original, "FROM python:3.12 AS build\nCOPY requirements.txt .\nFROM python:3.12-slim\nCOPY --from=build /app /app\nCOPY --from=ghcr.io/astral-sh/uv:latest /uv /bin/uv\n", Pass, ""},
		{"dropping copies is not inventing", original, "FROM python:3.12\n", Pass, ""},
		{"an invented file", original, "FROM python:3.12\nCOPY requirements.txt .\nCOPY pyproject.toml .\n", Fail, "pyproject.toml (line 3)"},
		{"a sibling of a copied directory", original, "FROM python:3.12\nCOPY srcx/ ./\n", Fail, "srcx/ (line 2)"},
		{"a parent of a copied directory", original, "FROM python:3.12\nCOPY . .\n", Fail, ". (line 2)"},
		{"invented ADD", original, "FROM python:3.12\nADD vendor.tar.gz /opt/\n", Fail, "vendor.tar.gz (line 2)"},
		{"a remote ADD is not a context copy", original, "FROM python:3.12\nADD https://example.com/x.tgz /opt/\n", Pass, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := CheckNoInvention(mustParse(t, c.o), mustParse(t, c.c))
			require.Equal(t, G2NoInvention, g.ID)
			require.Equal(t, c.wantStatus, g.Status, g.Reason)
			require.Contains(t, g.Reason, c.wantReason)
		})
	}
}

func TestCheckVersions(t *testing.T) {
	require.Equal(t, Gate{ID: G3Versions, Status: NotChecked, Reason: "version lookups not available"}, CheckVersions(false, nil))
	require.Equal(t, Gate{ID: G3Versions, Status: Pass}, CheckVersions(true, nil))
	require.Equal(t, Gate{ID: G3Versions, Status: Fail, Reason: "not found: python:3.99, curl=9.9"}, CheckVersions(true, []string{"python:3.99", "curl=9.9"}))
}

func TestCheckBuild(t *testing.T) {
	require.Equal(t, Gate{ID: G4Build, Status: Pass}, CheckBuild(nil))
	require.Equal(t, Gate{ID: G4Build, Status: Fail, Reason: "exit status 1"}, CheckBuild(errors.New("exit status 1")))
}

func TestCheckBehaviour(t *testing.T) {
	base := Contract{
		Entrypoint: []string{"/server"},
		Cmd:        []string{},
		Expose:     []string{"8080"},
		Workdir:    "/app",
		User:       "",
	}
	with := func(f func(*Contract)) Contract {
		c := base
		f(&c)
		return c
	}

	cases := []struct {
		name       string
		smoke      error
		o, c       Contract
		wantStatus Status
		wantReason string
	}{
		{"identical", nil, base, base, Pass, ""},
		{"smoke test fails", errors.New("exit code 1, want 0"), base, base, Fail, "smoke test: exit code 1"},
		{"root to non-root is allowed", nil, base, with(func(c *Contract) { c.User = "app" }), Pass, ""},
		{"explicit root to a UID is allowed", nil, with(func(c *Contract) { c.User = "root" }), with(func(c *Contract) { c.User = "65532:65532" }), Pass, ""},
		{"root UID 0 with a group counts as root", nil, with(func(c *Contract) { c.User = "0:0" }), with(func(c *Contract) { c.User = "nobody" }), Pass, ""},
		{"non-root to root is not", nil, with(func(c *Contract) { c.User = "app" }), base, Fail, `USER "app" -> ""`},
		{"non-root to another non-root is not", nil, with(func(c *Contract) { c.User = "app" }), with(func(c *Contract) { c.User = "web" }), Fail, `USER "app" -> "web"`},
		{"ENTRYPOINT changed", nil, base, with(func(c *Contract) { c.Entrypoint = []string{"/server", "--fast"} }), Fail, "ENTRYPOINT"},
		{"CMD cleared vs unset differ", nil, base, with(func(c *Contract) { c.Cmd = nil }), Fail, "CMD"},
		{"EXPOSE with an explicit /tcp is the same port", nil, base, with(func(c *Contract) { c.Expose = []string{"8080/tcp", "8080"} }), Pass, ""},
		{"EXPOSE changed", nil, base, with(func(c *Contract) { c.Expose = []string{"8080", "9090"} }), Fail, "EXPOSE"},
		{"WORKDIR written differently", nil, base, with(func(c *Contract) { c.Workdir = "/app/" }), Pass, ""},
		{"WORKDIR changed", nil, base, with(func(c *Contract) { c.Workdir = "/srv" }), Fail, "WORKDIR"},
		{
			"several differences reported together", nil, base,
			with(func(c *Contract) { c.Workdir = "/srv"; c.Expose = nil }),
			Fail, `EXPOSE [8080/tcp] -> []; WORKDIR "/app" -> "/srv"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := CheckBehaviour(c.smoke, c.o, c.c)
			require.Equal(t, G5Behaviour, g.ID)
			require.Equal(t, c.wantStatus, g.Status, g.Reason)
			require.Contains(t, g.Reason, c.wantReason)
		})
	}
}

func TestContractFromDockerfile(t *testing.T) {
	contract := func(src string) Contract {
		rc, err := mustParse(t, src).RuntimeContract()
		require.NoError(t, err)
		return ContractFromDockerfile(rc)
	}

	require.Equal(t, Contract{
		Entrypoint: []string{"/server"},
		Cmd:        []string{},
		Expose:     []string{"8080"},
		Workdir:    "/app",
		User:       "app",
	}, contract("FROM alpine\nWORKDIR /app\nEXPOSE 8080\nUSER app\nENTRYPOINT [\"/server\"]\n"))

	// Shell and exec form of the same command compare equal.
	shell := contract("FROM alpine\nCMD python app.py\n")
	exec := contract("FROM alpine\nCMD [\"/bin/sh\", \"-c\", \"python app.py\"]\n")
	require.Equal(t, []string{"/bin/sh", "-c", "python app.py"}, shell.Cmd)
	require.Equal(t, Pass, CheckBehaviour(nil, shell, exec).Status)

	require.Equal(t, Contract{}, contract("FROM alpine\n"))
}

func TestCheckNoNewSevere(t *testing.T) {
	o := loadTrivy(t, "trivy_o.json")
	cases := []struct {
		name       string
		c          ScanResult
		wantStatus Status
		wantReason string
	}{
		{"identical", o, Pass, ""},
		{"fewer findings", loadTrivy(t, "trivy_c.json"), Pass, ""},
		{"new high CVE", NewScanResult(append(o.Vulnerabilities, vuln("CVE-2025-0001", "libxml2", SeverityHigh, ""))), Fail, "CVE-2025-0001"},
		{"new critical CVEs listed sorted", NewScanResult([]Vulnerability{
			vuln("CVE-2025-0002", "a", SeverityCritical, ""), vuln("CVE-2025-0001", "b", SeverityHigh, ""),
		}), Fail, "new critical/high: CVE-2025-0001, CVE-2025-0002"},
		{"new medium CVE is allowed", NewScanResult(append(o.Vulnerabilities, vuln("CVE-2025-0003", "x", SeverityMedium, ""))), Pass, ""},
		{"same CVE under another package after a base swap", NewScanResult([]Vulnerability{vuln("CVE-2024-5535", "libssl3t64", SeverityCritical, "")}), Pass, ""},
		{"same CVE re-rated from medium to high", NewScanResult([]Vulnerability{vuln("CVE-2024-28182", "libnghttp2-14", SeverityHigh, "")}), Pass, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := CheckNoNewSevere(o, c.c)
			require.Equal(t, G6NoNewSevere, g.ID)
			require.Equal(t, c.wantStatus, g.Status, g.Reason)
			require.Contains(t, g.Reason, c.wantReason)
		})
	}

	empty := loadTrivy(t, "trivy_empty.json")
	require.Equal(t, Fail, CheckNoNewSevere(empty, o).Status, "anything severe is new against an empty O′")
	require.Equal(t, Pass, CheckNoNewSevere(o, empty).Status)
}
