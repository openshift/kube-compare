package compare

import (
	"errors"
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

const containerScheme = "container://"

// isContainer reports whether the given path is a reference to a file in a container by verifying if it starts with "container://".
func isContainer(path string) bool {
	return strings.HasPrefix(path, containerScheme)
}

type parsedPath struct {
	image string
	path  string
}

// parsePath returns the image and referencePath (path to the directory for metadata.yaml), given a path
// of the form container://<IMAGE>:<TAG>:/path_to_metadata.yaml
func parsePath(path string) (parsedPath, error) {
	path = strings.TrimPrefix(path, containerScheme)

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
		klog.V(1).Infof("Running sudo %v", args)
		out, err = execCommand("sudo", args...).Output()
	} else {
		klog.V(1).Infof("Running %s %v", engine.name, args)
		out, err = execCommand(engine.name, args...).Output()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, fmt.Errorf("%s :: %s :: %w", out, exitErr.Stderr, exitErr)
		}
		return out, fmt.Errorf("%s :: %w", out, err)
	}

	return out, nil
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
func (engine *engine) pullContainer(image string) error {
	// create the container so we can cp out of it
	out, err := engine.runEngineCommand("create", image)
	if err != nil {
		return fmt.Errorf("could not create container: %w", err)
	}
	engine.containerID = strings.TrimSpace(string(out)) // Convert bytes to string and trim new line
	klog.V(1).Infof("Created container %s", engine.containerID)
	return nil
}

// extractReferences copies the directory in the container that contains the reference configs into a temporary directory,
// and stores the path to the new directory in the engine struct.
func (engine *engine) extractReferences(pathToMetadata, dname string) error {
	_, err := engine.runEngineCommand("cp", engine.containerID+":"+pathToMetadata, dname)
	if err != nil {
		return fmt.Errorf("could not copy templates from container: %w", err)
	}
	engine.tempDir = filepath.Join(dname, filepath.Base(pathToMetadata))
	return nil
}

// cleanup removes the container used to extract the reference configs.
func (engine *engine) cleanup() {
	_, err := engine.runEngineCommand("rm", engine.containerID)
	if err != nil {
		klog.Warningf("Warning: Could not remove container: %s", err)
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

	err = engine.pullContainer(parsedPath.image)
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
