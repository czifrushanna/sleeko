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

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFamily(t *testing.T) {
	cases := map[string]string{
		"FROM ruby:2.6.3\n":                              "ruby",
		"FROM --platform=$BUILDPLATFORM php:8-apache\n":  "php",
		"FROM maven:3-openjdk-17 AS build\nFROM nginx\n": "java",
		"FROM mcr.microsoft.com/dotnet/sdk:8.0\n":        "dotnet",
		"from node:16-alpine\n":                          "node",
		"FROM golang:1.14\n":                             "golang",
		"FROM ubuntu:22.04\n":                            "ubuntu",
		"FROM scratch\n":                                 "other",
		"# no FROM at all\n":                             "other",
	}
	for df, want := range cases {
		require.Equal(t, want, family(df), df)
	}
}

func TestNormalizeAndVariants(t *testing.T) {
	require.Equal(t, "FROM a\nRUN b", normalize("FROM a  \r\nRUN b\t\n\n\n"))
	require.Equal(t, normalize("FROM a\nRUN b\n"), normalize("FROM a\nRUN b"))

	// The dataset's copy has one newline more than the committed file.
	committed := "FROM node:16\nCOPY . .\n"
	variants := exactVariants(committed + "\n")
	require.Contains(t, variants, committed)
}

func TestBlobIDMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, content := range []string{"", "FROM alpine\n", "FROM a\r\nRUN b\r\n"} {
		cmd := exec.Command("git", "hash-object", "--stdin")
		cmd.Stdin = strings.NewReader(content)
		out, err := cmd.Output()
		require.NoError(t, err)
		require.Equal(t, strings.TrimSpace(string(out)), blobID(content))
	}
}

func TestParseRawLog(t *testing.T) {
	out := []byte("@c2 200\n\n" +
		":100644 100644 aaa bbb M\tDockerfile\n" +
		":100644 100644 ccc ddd M\tsrc/main.go\n" +
		":000000 100644 000 eee A\tdocker/api.Dockerfile\n" +
		":100644 000000 fff 000 D\told/Dockerfile\n" +
		"@c1 100\n\n" +
		":000000 100644 000 aaa A\tDockerfile\n" +
		":000000 100644 000 ggg A\tdeploy/Dockerfile.prod\n")
	require.Equal(t, []version{
		{commit: "c2", time: 200, path: "Dockerfile", blob: "bbb"},
		{commit: "c2", time: 200, path: "docker/api.Dockerfile", blob: "eee"},
		{commit: "c1", time: 100, path: "Dockerfile", blob: "aaa"},
		{commit: "c1", time: 100, path: "deploy/Dockerfile.prod", blob: "ggg"},
	}, parseRawLog(out))
}

func TestParseBatch(t *testing.T) {
	out := []byte("abc blob 12\nFROM alpine\n\nzzz missing\ndef blob 0\n\n")
	require.Equal(t, map[string]string{"abc": "FROM alpine\n", "def": ""}, parseBatch(out))
}

func TestLanguage(t *testing.T) {
	require.Equal(t, "ruby", language([]string{"app/models/user.rb", "app/x.rb", "config/routes.rb", "public/app.js", "vendor/a.js", "package.json"}))
	require.Equal(t, "typescript", language([]string{"src/a.ts", "src/b.tsx", "node_modules/x/index.js", "dist/bundle.min.js"}))
	require.Equal(t, "", language([]string{"README.md", "Dockerfile"}))
}

func TestDrawSample(t *testing.T) {
	var recs []record
	for i := 0; i < 20; i++ {
		recs = append(recs, record{RepoName: "o@ruby" + string(rune('a'+i)), Dockerfile: "FROM ruby:3\nCOPY . .\n"})
		recs = append(recs, record{RepoName: "o@py" + string(rune('a'+i)), Dockerfile: "FROM python:3\nCOPY . .\n"})
	}
	recs = append(recs, record{RepoName: "o@nocopy", Dockerfile: "FROM ruby:3\nRUN gem install x\n"})

	got := drawSample(recs, map[string]int{"ruby": 5, "python": 2, "php": 3}, 1)
	require.Len(t, got, 7, "php has no records")
	again := drawSample(recs, map[string]int{"ruby": 5, "python": 2, "php": 3}, 1)
	require.Equal(t, got, again, "the same seed draws the same sample")
	for _, r := range got {
		require.NotEqual(t, "o@nocopy", r.RepoName, "Dockerfiles that copy nothing are not drawn")
	}

	_, err := parseSample("ruby=3,php")
	require.Error(t, err)
	counts, err := parseSample(defaultSample)
	require.NoError(t, err)
	require.Equal(t, 30, counts["ruby"])
}

// TestFindVersion matches dataset texts against a real repository history,
// by blob id and by normalised content.
func TestFindVersion(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	commit := func(file, content, msg string, at int) string {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(repo, file)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repo, file), []byte(content), 0o644))
		run("add", ".")
		date := time.Unix(int64(at), 0).Format(time.RFC3339)
		cmd := exec.Command("git", "commit", "--quiet", "-m", msg)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return run("rev-parse", "HEAD")
	}
	run("init", "--quiet")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	v1 := commit("Dockerfile", "FROM golang:1.10\nCOPY . .\n", "one", 1000)
	v2 := commit("Dockerfile", "FROM golang:1.14\nCOPY . .\n", "two", 2000)
	sub := commit("docker/web.Dockerfile", "FROM node:16  \nCOPY . /app\n\n\n", "three", 3000)

	out, err := git(repo, time.Minute, "", true, "log", "--all", "--no-renames", "--raw", "--no-abbrev", "--format=@%H %ct")
	require.NoError(t, err)
	versions := parseRawLog(out)
	require.Len(t, versions, 3)

	cases := []struct {
		name, text, commit, path string
	}{
		{"exact text", "FROM golang:1.14\nCOPY . .\n", v2, "Dockerfile"},
		{"an extra trailing newline, as in the dataset", "FROM golang:1.10\nCOPY . .\n\n", v1, "Dockerfile"},
		{"whitespace differences need the content", "FROM node:16\nCOPY . /app", sub, "docker/web.Dockerfile"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := findVersion(repo, append([]version(nil), versions...), c.text)
			require.NoError(t, err)
			require.Equal(t, c.commit, got.commit)
			require.Equal(t, c.path, got.path)
		})
	}

	_, err = findVersion(repo, append([]version(nil), versions...), "FROM ruby:3\n")
	require.ErrorContains(t, err, "no matching Dockerfile version")
}

func TestSlug(t *testing.T) {
	require.Equal(t, "diveintohacking-docker-compose-rails-6", slug("DiveIntoHacking-docker-compose-rails-6"))
}
