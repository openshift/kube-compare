package compare

import "testing"

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
