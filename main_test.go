package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPullNonHelmArgs(t *testing.T) {
	testCases := []struct{
		name         string
		args         []string
		expectedOpts options
		expectedArgs []string
	}{
		{
			name: "keep temp values file at end",
			args: []string{
				"--namespace",
				"test",
				"--dry-run",
				"--keep-temp-values-file",
			},
			expectedOpts: options{keepTempValuesFile: true},
			expectedArgs: []string{
				"--namespace",
				"test",
				"--dry-run",
			},
		},
		{
			name: "keep temp values file in middle of args",
			args: []string{
				"--namespace",
				"test",
				"--keep-temp-values-file",
				"--dry-run",
				"-f",
				"values.yaml",
			},
			expectedOpts: options{keepTempValuesFile: true},
			expectedArgs: []string{
				"--namespace",
				"test",
				"--dry-run",
				"-f",
				"values.yaml",
			},
		},
		{
			name: "no keep temp values file",
			args: []string{
				"--namespace",
				"test",
				"--dry-run",
				"-f",
				"values.yaml",
			},
			expectedOpts: options{keepTempValuesFile: false},
			expectedArgs: []string{
				"--namespace",
				"test",
				"--dry-run",
				"-f",
				"values.yaml",
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(subT *testing.T) {
			c := &controller{
				opts: options{keepTempValuesFile: false},
			}
			result := c.pullNonHelmArgs(tc.args)
			if c.opts.keepTempValuesFile != tc.expectedOpts.keepTempValuesFile {
				subT.Errorf("expected opts.keepTempValuesFile to be %v, got %v\n", tc.expectedOpts.keepTempValuesFile, c.opts.keepTempValuesFile)
			}
			if len(result) != len(tc.expectedArgs) {
				subT.Errorf("expected %d returned args, got %d\n", len(tc.expectedArgs), len(result))
			}
		})
	}
}

func TestPullValueFiles(t *testing.T) {
	testCases := []struct {
		name                   string
		args                   []string
		expectedValueFiles     []string
		expectedNewArgs        []string
	}{
		{
			name: "one values file with -f",
			args: []string{"-n", "test", "upgrade", "release", "my/chart", "-f", "override-values.yaml", "--dry-run"},
			expectedValueFiles: []string{"override-values.yaml"},
			expectedNewArgs: []string{"-n", "test", "upgrade", "release", "my/chart", "--dry-run"},
		},
		{
			name: "lots of values files",
			args: []string{"install", "release", "my/chart", "--values", "values-1.yaml", "-f", "values-2.yaml", "--dry-run", "--values", "values-3.yaml"},
			expectedValueFiles: []string{"values-1.yaml", "values-2.yaml", "values-3.yaml"},
			expectedNewArgs: []string{"install", "release", "my/chart", "--dry-run"},
		},
		{
			name: "no value files",
			args: []string{"install", "-n", "namespace", "release", "my/chart"},
			expectedValueFiles: []string{},
			expectedNewArgs: []string{"install", "-n", "namespace", "release", "my/chart"},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(subT *testing.T) {
			resultValueFiles, resultNewArgs := pullValueFiles(tc.args)
			if len(resultValueFiles) != len(tc.expectedValueFiles) {
				subT.Errorf("expected %d value files, got %d\n", len(tc.expectedValueFiles), len(resultValueFiles))
			}
			for i, f := range resultValueFiles {
				if f != tc.expectedValueFiles[i] {
					subT.Errorf("expected value file %s, got %s\n", tc.expectedValueFiles[i], f)
				}
			}
			if len(resultNewArgs) != len(tc.expectedNewArgs) {
				subT.Errorf("expected %d new args, got %d\n", len(tc.expectedNewArgs), len(resultNewArgs))
			}
			for i, a := range resultNewArgs {
				if a != tc.expectedNewArgs[i] {
					subT.Errorf("expected new arg %s, got %s\n", tc.expectedNewArgs[i], a)
				}
			}
		})
	}
}

func TestResolveValueFilesNoSSM(t *testing.T) {
	// Multiple value files WITHOUT ssm directives must be handed to helm as
	// separate `-f` flags in the original order (so helm does its native
	// multi-file merge), with no temp files created. This is the core of the
	// fix: previously they were concatenated into one temp file. Files with no
	// ssm directives take the changed==false branch, so this exercises the new
	// wiring without needing AWS or a real helm binary.
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	local := filepath.Join(dir, "local.yaml")
	if err := os.WriteFile(base, []byte("chassis:\n  utilDeployments: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("chassis:\n  extra: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &controller{opts: options{}}
	baseArgs := []string{"upgrade", "release", "my/chart", "-n", "ns"}
	helmArgs, tempFiles, err := c.resolveValueFiles([]string{base, local}, baseArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tempFiles) != 0 {
		t.Errorf("expected no temp files for non-ssm inputs, got %v", tempFiles)
	}
	expected := []string{"upgrade", "release", "my/chart", "-n", "ns", "-f", base, "-f", local}
	if len(helmArgs) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(helmArgs), helmArgs)
	}
	for i := range expected {
		if helmArgs[i] != expected[i] {
			t.Errorf("arg %d: expected %q, got %q", i, expected[i], helmArgs[i])
		}
	}
}

func TestConstructReplacementLine(t *testing.T) {
	testCases := []struct {
		name          string
		oldLine       string
		location		  []int
		paramValue    string
		expected      string
	}{
		{
			name: "Top-level standalone parameter",
			oldLine: "test-values-key: {{ssm /configmgmt/my/parameter}}",
			location: []int{17, 49},
			paramValue: "test-param-value",
			expected:    "test-values-key: test-param-value\n",
		},
		{
			name: "Nested standalone parameter",
			oldLine: "    test-values-key: {{ssm /configmgmt/my/parameter}}",
			location: []int{21, 53},
			paramValue: "test-param-value-2",
			expected:    "    test-values-key: test-param-value-2\n",
		},
		{
			name: "List item path parameter",
			oldLine: "- {{ssm-path /configmgmt/path/to/my/parameter}}",
			location: []int{2, 47},
			paramValue: "{param1: testtttt, param2: hello}",
			expected:    "- {param1: testtttt, param2: hello}\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(subT *testing.T) {
			result := constructReplacementLine(tc.oldLine, tc.location, tc.paramValue)
			if result != tc.expected {
				subT.Errorf("expected line %s, got %s\n", tc.expected, result)
			}
		})
	}
}
