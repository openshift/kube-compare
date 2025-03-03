package compare

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsContainer(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"container://my-image:latest:/metadata.yaml", true},
		{"file://local/path/to/file.yaml", false},
		{"http://example.com/data.yaml", false},
		{"randomstringwithoutprefix", false},
	}

	for _, test := range tests {
		result := isContainer(test.input)
		if result != test.expected {
			t.Errorf("For input '%s', expected %v, got %v", test.input, test.expected, result)
		}
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		input         string
		expectedImage string
		expectedPath  string
		expectError   bool
	}{
		{"container://my-image:latest:/metadata.yaml", "my-image:latest", "/metadata.yaml", false},
		{"container://repo/image:tag:/data.yaml", "repo/image:tag", "/data.yaml", false},
		{"container://repo/image:::tag::::/data.yaml", "repo/image:tag", "/data.yaml", false},
		{"container://wrongformat", "", "", true},
		{":missingprefix:/file.yaml", "", "", true},
		{"container://only:two", "", "", true},
	}

	for _, test := range tests {
		image, path, err := parsePath(test.input)
		if test.expectError {
			if err == nil {
				t.Errorf("Expected error for input '%s', but got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input '%s': %v", test.input, err)
			}
			if image != test.expectedImage || path != test.expectedPath {
				t.Errorf("For input '%s', expected ('%s', '%s'), got ('%s', '%s')", test.input, test.expectedImage, test.expectedPath, image, path)
			}
		}
	}
}

// fakeExecCommand() and TestHelperProcess() are derived from https://npf.io/2015/06/testing-exec-command/,
// The MIT License (MIT)

// Copyright (c) 2014 Nate Finch

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	for _, arg := range args {
		if strings.Contains(arg, "invalid") {
			cmd := exec.Command("false")
			return cmd
		}
	}
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func fakeLookPath(command string) (string, error) {
	switch command {
	case "podman":
		return "/usr/bin/podman", nil
	case "docker":
		return "/usr/bin/docker", nil
	default:
		return "", errors.New("not found")
	}
}

const dockerRunResult = "fake output"

func TestRunEngineCommand(t *testing.T) {
	// Override execCommand with fakes
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()
	tests := []struct {
		engine engine
		args   []string
	}{
		{engine{"podman", false, "container123", "/tmp/dir/"}, []string{"run", "hello-world"}},
		{engine{"docker", true, "container123", "/tmp/dir/"}, []string{"run", "hello-world"}},
	}

	for _, test := range tests {
		out, err := test.engine.runEngineCommand(test.args...)

		if err != nil {
			t.Errorf("Expected nil error, got %#v", err)
		}
		if string(out) != dockerRunResult {
			t.Errorf("Expected %q, got %q", dockerRunResult, out)
		}
	}
}

func TestHasPodmanOrDocker(t *testing.T) {
	// Override execCommand and LookPath with fakes
	execCommand = fakeExecCommand
	lookPath = fakeLookPath
	defer func() {
		execCommand = exec.Command
		lookPath = exec.LookPath
	}()

	tests := []struct {
		available    string
		expectedName string
		expectedSudo bool
		expectError  bool
	}{
		{"podman", "podman", false, false},     // Podman is available
		{"docker", "docker", false, false},     // Docker available, no sudo needed
		{"docker-sudo", "docker", true, false}, // Docker requires sudo
		{"none", "", false, true},              // Neither Podman nor Docker is available
	}

	for _, test := range tests {
		// Customize behavior based on test case
		lookPath = func(cmd string) (string, error) {
			if (test.available == "podman" && cmd == "podman") || (test.available == "docker" || test.available == "docker-sudo") && cmd == "docker" {
				return "/usr/bin/" + cmd, nil
			}
			return "", errors.New("not found")
		}

		execCommand = func(command string, args ...string) *exec.Cmd {
			cs := []string{"-test.run=TestHelperProcess", "--", command}
			cs = append(cs, args...)
			cmd := exec.Command(os.Args[0], cs...)
			cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
			if test.available == "docker-sudo" {
				cmd.Env = append(cmd.Env, "MOCK_DOCKER_FAIL=1") // Simulate `docker images` failing
			}
			return cmd
		}

		eng, err := newEngine()

		if test.expectError {
			if err == nil {
				t.Errorf("Expected error but got nil")
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if eng.name != test.expectedName {
				t.Errorf("Expected engine name %q, got %q", test.expectedName, eng.name)
			}
			if eng.requiresSudo != test.expectedSudo {
				t.Errorf("Expected requiresSudo %v, got %v", test.expectedSudo, eng.requiresSudo)
			}
		}
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if os.Getenv("MOCK_DOCKER_FAIL") == "1" {
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, dockerRunResult)
	os.Exit(0)
}

func TestPullAndRunContainer(t *testing.T) {
	// Override execCommand with a fake function
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()

	tests := []struct {
		engine      engine
		image       string
		expectError bool
	}{
		{engine{"podman", false, "", ""}, "hello-world", false},
		{engine{"docker", true, "", ""}, "hello-world", false},
		{engine{"docker", true, "", ""}, "invalid-image", true},
	}

	for _, test := range tests {
		err := test.engine.pullAndRunContainer(test.image)
		if test.expectError {
			if err == nil {
				t.Errorf("Expected error but got nil")
			}
		} else {
			if err != nil {
				t.Errorf("Expected nil error, got %#v", err)
			}
			if test.engine.containerID != dockerRunResult {
				t.Errorf("Expected container ID %q, got %q", dockerRunResult, test.engine.containerID)
			}
		}
	}
}

func TestExtractReferences(t *testing.T) {
	// Override execCommand with a fake function
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()

	tests := []struct {
		engine         engine
		pathToMetadata string
		dname          string
		expectError    bool
	}{
		{engine{"podman", false, "container123", ""}, "/etc/configs", "/tmp/extracted", false},
		{engine{"docker", true, "container456", ""}, "/var/lib/data", "/tmp/extracted", false},
		{engine{"docker", true, "container456", ""}, "/invalid_path", "/tmp/extracted", true},
	}

	for _, test := range tests {
		err := test.engine.extractReferences(test.pathToMetadata, test.dname)
		if test.expectError {
			if err == nil {
				t.Errorf("Expected error but got nil")
			}
		} else {
			if err != nil {
				t.Errorf("Expected nil error, got %#v", err)
			}
			expectedTempDir := filepath.Join(test.dname, filepath.Base(test.pathToMetadata))
			if test.engine.tempDir != expectedTempDir {
				t.Errorf("Expected tempDir %q, got %q", expectedTempDir, test.engine.tempDir)
			}
		}
	}
}

func TestCleanup(t *testing.T) {
	// Override execCommand with a fake function
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()

	tests := []struct {
		engine engine
	}{
		{engine{"podman", false, "container123", ""}},
		{engine{"docker", true, "container456", ""}},
	}

	for _, test := range tests {
		err := test.engine.cleanup()
		if err != nil {
			t.Errorf("Expected nil error, got %#v", err)
		}
	}
}

func TestGetReferencesFromContainer(t *testing.T) {
	// Override execCommand with a fake function
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()

	tests := []struct {
		path                string
		tempContainerRefDir string
		expectError         bool
	}{
		{"container://image:tag:/etc/configs", "/tmp/refdir", false},
		{"invalid-path", "/tmp/refdir", true},
	}

	for _, test := range tests {
		tempDir, err := getReferencesFromContainer(test.path, test.tempContainerRefDir)
		if test.expectError {
			if err == nil {
				t.Errorf("Expected error but got nil")
			}
		} else {
			if err != nil {
				t.Errorf("Expected nil error, got %#v", err)
			}
			if tempDir == "" {
				t.Errorf("Expected a valid tempDir, got an empty string")
			}
		}
	}
}
