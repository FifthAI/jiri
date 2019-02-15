// Copyright 2018 The Fuchsia Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cipd

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	// Some random valid cipd version tags from infra/tools/cipd
	cipdVersionForTestA = "git_revision:00e2d8b49a4e7505d1c71f19d15c9e7c5b9245a5"
	cipdVersionForTestB = "git_revision:8fac632847b1ce0de3b57d16d0f2193625f4a4f0"
	// package path and versions for ACL tests
	cipdPkgPathA    = "gn/gn/${platform}"
	cipdPkgVersionA = "git_revision:bdb0fd02324b120cacde634a9235405061c8ea06"
	cipdPkgPathB    = "notexist/notexist"
	cipdPkgVersionB = "git_revision:bdb0fd02324b120cacde634a9235405061c8ea06"
)

var (
	// Digests generated by cipd selfupdate-roll ...
	digestMapA = map[string]string{
		"linux-amd64":  "df37ffc2588e345a31ca790d773b6136fedbd2efbf9a34cb735dd34b6891c16c",
		"linux-arm64":  "650f2a045f8587062a16299a650aa24ba5c5c0652585a2d9bd56594369d5f99e",
		"linux-armv6l": "61b657c860ddc39d3286ced073c843852b1dafc0222af0bdc22ad988b289d733",
		"mac-amd64":    "4d015791ed6f03f305cf6a5a673a447e5c47ff5fdb701f43f99fba9ca73e61f8",
	}
	digestMapB = map[string]string{
		"linux-amd64":  "bdc971fd2895c3771e0709d2a3ec5fcace69c59a3a9f9dc33ab76fbc2f777d40",
		"linux-arm64":  "e1d6aadc9bfc155e9088aa3de39b9d3311c7359f398f372b5ad1c308e25edfeb",
		"linux-armv6l": "3ad97b47ecc1b358c8ebd1b0307087d354433d88f24bf8ece096fb05452837f9",
		"mac-amd64":    "167edadf7c7c019a40b9f7869a4c05b2d9834427dad68e295442ef9ebce88dba",
	}
	instanceIDMap = map[string]string{
		"gn/gn/linux-amd64": "0uGjKAZkJXPZjtYktgEwHiNbwsut_qRsk7ZCGGxi82IC",
		"gn/gn/mac-amd64":   "rN2F641yR4Bj-H1q8OwC_RiqRpUYxy3hryzRfPER9wcC",
	}
)

// TestFetchBinary tests fetchiBinary method by fetching a set of
// cipd binaries. This test requires network access
func TestFetchBinary(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "jiri-test")
	if err != nil {
		t.Error("failed to create temp dir for testing")
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		version string
		digest  map[string]string
	}{
		{cipdVersionForTestA, digestMapA},
		{cipdVersionForTestB, digestMapB},
	}

	for i, test := range tests {
		for platform, digest := range test.digest {
			cipdPath := path.Join(tmpDir, "cipd"+platform+test.version)
			if err := fetchBinary(cipdPath, platform, test.version, digest); err != nil {
				t.Errorf("test %d failed while retrieving cipd binary for platform %q on version %q with digest %q: %v", i, platform, test.version, digest, err)
			}
		}
	}
}

func TestCipdVersion(t *testing.T) {
	// Assume cipd version is always a git commit hash for now
	versionStr := string(cipdVersion)
	if len(versionStr) != len("git_revision:00e2d8b49a4e7505d1c71f19d15c9e7c5b9245a5") ||
		!strings.HasPrefix(versionStr, "git_revision:") {
		t.Errorf("unsupported cipd version tag: %q", versionStr)
	}
	versionHash := versionStr[len("git_revision:"):]
	if _, err := hex.DecodeString(versionHash); err != nil {
		t.Errorf("unsupported cipd version tag: %q", versionStr)
	}
}

func TestFetchDigest(t *testing.T) {
	tests := []string{
		"linux-amd64",
		"linux-arm64",
		"linux-armv6l",
		"mac-amd64",
	}

	for _, platform := range tests {
		digest, _, err := fetchDigest(platform)
		if err != nil {
			t.Errorf("failed to retrieve cipd digest for platform %q due to error: %v", platform, err)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			t.Errorf("digest %q is not a valid hex string for platform %q", digest, platform)
		}
	}
}

func TestSelfUpdate(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "jiri-test")
	if err != nil {
		t.Error("failed to create temp dir for testing")
	}
	defer os.RemoveAll(tmpDir)
	// Bootstrap cipd to version A
	cipdPath := path.Join(tmpDir, "cipd")
	if err := fetchBinary(cipdPath, CipdPlatform.String(), cipdVersionForTestA, digestMapA[CipdPlatform.String()]); err != nil {
		t.Errorf("failed to bootstrap cipd with version %q: %v", cipdVersionForTestA, err)
	}
	// Perform cipd self update to version B
	if err := selfUpdate(cipdPath, cipdVersionForTestB); err != nil {
		t.Errorf("failed to perform cipd self update: %v", err)
	}
	// Verify self updated cipd
	cipdData, err := ioutil.ReadFile(cipdPath)
	if err != nil {
		t.Errorf("failed to read self-updated cipd binary: %v", err)
	}
	verified, err := verifyDigest(cipdData, digestMapB[CipdPlatform.String()])
	if err != nil {
		t.Errorf("digest failed verification for platform %q on version %q", CipdPlatform.String(), cipdVersionForTestB)
	}
	if !verified {
		t.Errorf("self-updated cipd failed integrity test")
	}
}

func TestBootsrap(t *testing.T) {
	cipdPath, err := Bootstrap()
	if cipdPath == "" {
		t.Errorf("bootstrap returned an empty path")
	}
	fileInfo, err := os.Stat(cipdPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Errorf("bootstrap failed, cipd binary was not found at %q", cipdPath)
		}
		t.Errorf("bootstrap failed, could not access cipd binary at %q due to error %v", cipdPath, err)
	}
	if fileInfo.Mode()&0111 == 0 {
		t.Errorf("bootstrap failed, cipd binary at %q is not executable", cipdBinary)
	}
}

func TestEnsure(t *testing.T) {
	cipdPath, err := Bootstrap()
	if err != nil {
		t.Errorf("bootstrap failed due to error: %v", err)
	}
	defer os.Remove(cipdPath)
	// Write test ensure file
	testEnsureFile, err := ioutil.TempFile("", "test_jiri*.ensure")
	if err != nil {
		t.Errorf("failed to create test ensure file: %v", err)
	}
	defer testEnsureFile.Close()
	defer os.Remove(testEnsureFile.Name())
	_, err = testEnsureFile.Write([]byte(`
$ParanoidMode CheckPresence

# GN
gn/gn/${platform} git_revision:bdb0fd02324b120cacde634a9235405061c8ea06
`))
	if err != nil {
		t.Errorf("failed to write test ensure file: %v", err)
	}
	testEnsureFile.Sync()
	tmpDir, err := ioutil.TempDir("", "jiri-test")
	if err != nil {
		t.Error("failed to creat temp dir for testing")
	}
	defer os.RemoveAll(tmpDir)
	// Invoke Ensure on test ensure file
	if err := Ensure(nil, testEnsureFile.Name(), tmpDir, 30); err != nil {
		t.Errorf("ensure failed due to error: %v", err)
	}
	// Check the existence downloaded package
	gnPath := path.Join(tmpDir, "gn")
	if _, err := os.Stat(gnPath); err != nil {
		if os.IsNotExist(err) {
			t.Errorf("fetched cipd package is not found at %q", gnPath)
		}
		t.Errorf("failed to execute os.Stat() on fetched cipd package due to error: %v", err)
	}
}

func TestCheckACL(t *testing.T) {
	cipdPath, err := Bootstrap()
	if err != nil {
		t.Errorf("bootstrap failed due to error: %v", err)
	}
	defer os.Remove(cipdPath)

	pkgMap := make(map[string]bool)
	pkgMap[cipdPkgPathA] = false
	pkgMap[cipdPkgPathB] = false
	versionMap := make(map[string]string)
	versionMap[cipdPkgPathA] = cipdPkgVersionA
	versionMap[cipdPkgPathB] = cipdPkgVersionB
	if err := CheckPackageACL(nil, pkgMap, versionMap); err != nil {
		t.Errorf("CheckPackageACL failed due to error: %v", err)
	}

	if !pkgMap[cipdPkgPathA] {
		t.Errorf("pkg %q should be accessible, but it is not accessible by cipd", cipdPkgPathA)
	}

	if pkgMap[cipdPkgPathB] {
		t.Errorf("pkg %q should not be accessible, but it is accessible by cipd", cipdPkgPathB)
	}

}

func TestResolve(t *testing.T) {
	cipdPath, err := Bootstrap()
	if err != nil {
		t.Errorf("bootstrap failed due to error: %v", err)
	}
	defer os.Remove(cipdPath)

	// Write test ensure file
	testEnsureFile, err := ioutil.TempFile("", "test_jiri*.ensure")
	if err != nil {
		t.Errorf("failed to create test ensure file: %v", err)
	}
	defer testEnsureFile.Close()
	ensureFileName := testEnsureFile.Name()
	defer os.Remove(ensureFileName)
	versionFileName := ensureFileName[:len(ensureFileName)-len(".ensure")] + ".version"
	var ensureBuf bytes.Buffer
	ensureBuf.WriteString("$ResolvedVersions " + versionFileName + "\n")
	ensureBuf.WriteString(`
$ParanoidMode CheckPresence
$VerifiedPlatform linux-amd64
$VerifiedPlatform mac-amd64

# GN
gn/gn/${platform} git_revision:bdb0fd02324b120cacde634a9235405061c8ea06
`)
	_, err = testEnsureFile.Write(ensureBuf.Bytes())
	if err != nil {
		t.Errorf("failed to write test ensure file: %v", err)
	}

	testEnsureFile.Sync()
	instances, err := Resolve(nil, testEnsureFile.Name())
	if err != nil {
		t.Errorf("resolve failed due to error: %v", err)
	}
	for _, instance := range instances {
		if val, ok := instanceIDMap[instance.PackageName]; ok {
			if val != instance.InstanceID {
				t.Errorf("instance id %q for package %q does not match the record %q",
					instance.InstanceID, instance.PackageName, val)
			}
		} else {
			t.Errorf("package %q is not found in record", instance.PackageName)
		}
	}
}

func TestExpand(t *testing.T) {
	platforms := []Platform{
		Platform{"linux", "amd64"},
		Platform{"linux", "arm64"},
		Platform{"mac", "amd64"},
	}

	tests := map[string][]string{
		"gn/gn/${platform}":                   []string{"gn/gn/linux-amd64", "gn/gn/linux-arm64", "gn/gn/mac-amd64"},
		"fuchsia/sysroot/${os=linux}-${arch}": []string{"fuchsia/sysroot/linux-amd64", "fuchsia/sysroot/linux-arm64"},
		"infra/ninja/linux-amd64":             []string{"infra/ninja/linux-amd64"},
	}

	for k, p := range tests {
		pkgs, err := Expand(k, platforms)
		if err != nil {
			t.Errorf("Expand faild on path %q due to error: %v", p, err)
		}
		sort.Strings(p)
		sort.Strings(pkgs)
		if !reflect.DeepEqual(p, pkgs) {
			t.Errorf("test on %q failed: expecting %v, got %v", k, p, pkgs)
		}
	}
}
