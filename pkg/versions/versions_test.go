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

package versions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDeb uses cases from dpkg's own test suite and Debian Policy, plus the
// shapes security fixes take in practice.
func TestDeb(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.0-0", 0},
		{"1.0-1", "1.0-2", -1},
		{"1.0~rc1", "1.0", -1},
		{"1.0~~", "1.0~", -1},
		{"1.0~", "1.0", -1},
		{"1.0", "1.0+b1", -1},
		{"1.0", "1.0a", -1},
		{"1.0a", "1.0+", -1},
		{"1:1.0", "2.0", 1},
		{"0:1.0", "1.0", 0},
		{"1.10", "1.9", 1},
		{"1.010", "1.10", 0},
		{"2.36-9+deb12u1", "2.36-9+deb12u3", -1},
		{"7.88.1-10+deb12u5", "7.88.1-10+deb12u6", -1},
		{"3.0.11-1~deb12u2", "3.0.13-1~deb12u1", -1},
		{"3.0.13-1~deb12u1", "3.0.13-1", -1},
		{"1:1.2.13.dfsg-1", "1:1.2.13.dfsg-1+deb12u1", -1},
		{"2.30.2-1+deb11u2", "1:2.30.2-1", -1},
		{"5.2.15-2+b2", "5.2.15-2", 1},
		{"1.2.3ubuntu0.1", "1.2.3", 1},
		{"2.35-0ubuntu3.8", "2.35-0ubuntu3.10", -1},
	}
	for _, c := range cases {
		got, err := Deb(c.a, c.b)
		require.NoError(t, err, "%s vs %s", c.a, c.b)
		require.Equal(t, c.want, got, "%s vs %s", c.a, c.b)
		back, err := Deb(c.b, c.a)
		require.NoError(t, err)
		require.Equal(t, -c.want, back, "antisymmetric: %s vs %s", c.b, c.a)
	}

	for _, bad := range []string{"", "a1.0", "x:1.0", "1:"} {
		_, err := Deb(bad, "1.0")
		require.Error(t, err, bad)
	}
}

func TestPEP440Order(t *testing.T) {
	// Strictly increasing, from PEP 440's own examples.
	ordered := []string{
		"1.0.dev456", "1.0a1", "1.0a2.dev456", "1.0a12.dev456", "1.0a12",
		"1.0b1.dev456", "1.0b2", "1.0b2.post345.dev456", "1.0b2.post345",
		"1.0rc1.dev456", "1.0rc1", "1.0", "1.0+abc.5", "1.0+abc.7", "1.0+5",
		"1.0.post456.dev34", "1.0.post456", "1.1.dev1", "65.5.1", "70.0.0", "1!0.1",
	}
	for i := 0; i+1 < len(ordered); i++ {
		got, err := PEP440(ordered[i], ordered[i+1])
		require.NoError(t, err)
		require.Equal(t, -1, got, "%s < %s", ordered[i], ordered[i+1])
	}
}

func TestPEP440Equivalent(t *testing.T) {
	// Alternative spellings PEP 440 normalises.
	pairs := [][2]string{
		{"1.0", "1.0.0"},
		{"v1.0", "1.0"},
		{"1.0-1", "1.0.post1"},
		{"1.0.post", "1.0.post0"},
		{"1.0-alpha.1", "1.0a1"},
		{"1.0c1", "1.0rc1"},
		{"1.0-preview-2", "1.0rc2"},
		{"1.0.DEV", "1.0.dev0"},
		{"1.0+ubuntu-1", "1.0+ubuntu.1"},
		{"0!1.0", "1.0"},
	}
	for _, p := range pairs {
		got, err := PEP440(p[0], p[1])
		require.NoError(t, err)
		require.Zero(t, got, "%s == %s", p[0], p[1])
	}
}

func TestParsePEP440(t *testing.T) {
	v, err := ParsePEP440("2!1.4rc3.post2.dev1+local.7")
	require.NoError(t, err)
	require.Equal(t, 2, v.Epoch)
	require.Equal(t, []int{1, 4}, v.Release)
	require.Equal(t, &[2]int{2, 3}, v.Pre)
	require.Equal(t, 2, *v.Post)
	require.Equal(t, 1, *v.Dev)
	require.Equal(t, []string{"local", "7"}, v.Local)

	for _, bad := range []string{"", "latest", "1.0-beta-x", "==1.0", "1.0.*"} {
		_, err := ParsePEP440(bad)
		require.Error(t, err, bad)
	}
}
