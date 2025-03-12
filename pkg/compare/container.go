package compare

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"
)

type engine struct {
	name         string
	requiresSudo bool
	containerID  string
	tempDir      string
}

// isContainer reports whether the given path is a reference to a file in a container by verifying if it starts with "container://".
func isContainer(path string) bool {
	return strings.HasPrefix(path, "container://")
}

type parsedPath struct {
	image string
	path  string
}

// parsePath returns the image and referencePath (path to the directory for metadata.yaml), given a path
// of the form container://<IMAGE>:<TAG>:/path_to_metadata.yaml
func parsePath(path string) (parsedPath, error) {
	path = strings.TrimPrefix(path, "container://")

	// Split on ':', removing empty strings from slice. Removes errant colons from string,
	// so container://<IMAGE>:::<TAG>::::::/path/to/metadata.yaml will still work, but
	// paths with leading and trailing colons won't.
	f := func(c rune) bool {
		return c == ':'
	}
	sections := strings.FieldsFunc(path, f)

	if len(sections) == 3 {
		image := sections[0] + ":" + sections[1]
		referencePath := sections[2]
		return parsedPath{image: image, path: referencePath}, nil
	}
	return parsedPath{image: "", path: ""}, fmt.Errorf("incorrect path passed to -r, it must follow this format: container://<IMAGE>:<TAG>:/path/to/metadata.yaml")
}

// Use var's so that we can mock functions in tests.
var execCommand = exec.Command
var lookPath = exec.LookPath

// runEngineCommand runs a podman/docker command with sudo if necessary.
// Returns the stdout (out) and stderr (err) of the command.
func (engine *engine) runEngineCommand(args ...string) ([]byte, error) {
	var out []byte
	var err error
	if engine.requiresSudo {
		args = append([]string{engine.name}, args...) // Prepend engine name to args
		out, err = execCommand("sudo", args...).CombinedOutput()
	} else {
		out, err = execCommand(engine.name, args...).CombinedOutput()
	}

	return out, err //nolint:wrapcheck  // We want to return unaltered errors, this is a wrapper around exec.Command()
}

// newEngine checks if Podman or Docker are in the system's PATH, and returns an engine with a name and a boolean
// that indicates if sudo is needed for future commands. Prefers Podman. Returns an error if neither engine is found.
var newEngine = func() (*engine, error) {
	if _, err := lookPath("podman"); err == nil {
		return &engine{name: "podman", requiresSudo: false}, nil
	}

	if _, err := lookPath("docker"); err == nil {
		_, err = execCommand("docker", "images").Output() // If this errors out, we need to use sudo, return true.
		return &engine{name: "docker", requiresSudo: err != nil}, nil
	}
	return &engine{name: "", requiresSudo: false}, fmt.Errorf("you do not have Podman or Docker on your PATH")
}

// pullContainer pulls an image, runs it using the provided engine, and stores the corresponding containerID in the engine struct
func (engine *engine) pullAndRunContainer(image string) error {
	// run because copy requires a running or stopped container
	// -d to output container ID
	out, err := engine.runEngineCommand("run", "-d", image)
	if err != nil {
		return fmt.Errorf("could not pull/run container: %s", out)
	}
	engine.containerID = strings.TrimSpace(string(out)) // Convert bytes to string and trim new line
	return nil
}

// extractReferences copies the directory in the container that contains the reference configs into a temporary directory,
// and stores the path to the new directory in the engine struct.
func (engine *engine) extractReferences(pathToMetadata, dname string) error {

	out, err := engine.runEngineCommand("cp", engine.containerID+":"+pathToMetadata, dname)
	if err != nil {
		return fmt.Errorf("could not copy templates from container: %s", out)
	}
	engine.tempDir = filepath.Join(dname, filepath.Base(pathToMetadata))
	return nil
}

// cleanup stops and removes the container used to extract the reference configs.
func (engine *engine) cleanup() {
	out, err := engine.runEngineCommand("stop", engine.containerID)
	if err != nil {
		// Print errors as warnings rather than returning, since stopping and removing the container is not vital.
		klog.Warningf("Warning: Could not stop container: %s", out)
	}
	out, err = engine.runEngineCommand("rm", engine.containerID)
	if err != nil {
		klog.Warningf("Warning: Could not remove container: %s", out)
	}
}

// getReferencesFromContainer uses a path to an image and a metadata.yaml within that image, and extracts the reference configs
// to a local temporary directory. Returns the path to this directory.
func getReferencesFromContainer(path, tempContainerRefDir string) (string, error) {
	engine, err := newEngine()
	if err != nil {
		return "", err
	}

	parsedPath, err := parsePath(path)
	if err != nil {
		return "", err
	}

	err = engine.pullAndRunContainer(parsedPath.image)
	if err != nil {
		return "", err
	}

	defer engine.cleanup()

	err = engine.extractReferences(parsedPath.path, tempContainerRefDir)
	if err != nil {
		return "", err
	}

	return engine.tempDir, nil
}
