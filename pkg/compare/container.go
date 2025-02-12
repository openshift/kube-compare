package compare

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type engine struct {
	name         string
	requiresSudo bool
}

// isContainer reports whether the given path is a reference to a file in a container by verifying if it starts with "container://".
func isContainer(path string) bool {
	return strings.HasPrefix(path, "container://")
}

// parsePath returns the image and referencePath (path to the directory for metadata.yaml), given a path
// of the form container://<IMAGE>:<TAG>:/path_to_metadata.yaml
func parsePath(path string) (string, string, error) {
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
		return image, referencePath, nil
	}
	return "", "", fmt.Errorf("Incorrect path passed into -r, it should follow this format: container://<IMAGE>:<TAG>:/path_to_metadata.yaml")
}

// runEngineCommand sets up the podman/docker command with sudo if necessary.
// Returns the stdout (out) and stderr (err) of the command.
func runEngineCommand(engine engine, args ...string) ([]byte, error) {
	var out []byte
	var err error
	if engine.requiresSudo {
		args = append([]string{engine.name}, args...) // Prepend engine name to args
		out, err = exec.Command("sudo", args...).Output()
	} else {
		out, err = exec.Command(engine.name, args...).Output()
	}

	return out, err
}

// hasPodmanOrDocker checks if Podman or Docker are in the system's PATH,
// and returns the name of the engine it finds, with a preference for Podman, and a boolean
// that indicates if sudo is needed for future commands. Returns an error if neither engine is found.
func hasPodmanOrDocker() (engine, error) {
	if _, err := exec.LookPath("podman"); err == nil {
		return engine{name: "podman", requiresSudo: false}, nil
	}

	if _, err := exec.LookPath("docker"); err == nil {
		_, err = exec.Command("docker", "images").Output() // If this errors out, we need to use sudo, return true.
		return engine{name: "docker", requiresSudo: err != nil}, nil
	}
	return engine{name: "", requiresSudo: false}, fmt.Errorf("You do not have Podman or Docker on your PATH")
}

// pullContainer pulls an image and runs it using the provided engine, and returns the corresponding containerID
func pullAndRunContainer(engine engine, image string) (string, error) {
	// run because copy requires a running or stopped container
	// -d to output container ID
	out, err := runEngineCommand(engine, "run", "-d", image)
	if err != nil {
		return "", fmt.Errorf("Could not pull/run container: %s", err)
	}
	containerID := strings.TrimSpace(string(out)) // Convert bytes to string and trim new line
	return containerID, nil
}

// extractReferences copies the directory in the container that contains the reference configs into a temporary directory,
// and returns the path to the new directory.
func extractReferences(engine engine, containerID string, pathToMetadata string, dname string) (string, error) {

	_, err := runEngineCommand(engine, "cp", containerID+":"+pathToMetadata, dname)
	if err != nil {
		return "", fmt.Errorf("Could not copy templates from container: %s", err)
	}
	return filepath.Join(dname, filepath.Base(pathToMetadata)), nil
}

// cleanup stops and removes the container used to extract the reference configs.
func cleanup(engine engine, containerID string) error {
	_, err := runEngineCommand(engine, "stop", containerID)
	if err != nil {
		// Print errors rather than returning, since stopping and removing the container is not vital.
		fmt.Printf("Warning: Could not stop container: %s", err)
	}
	_, err = runEngineCommand(engine, "rm", containerID)
	if err != nil {
		fmt.Printf("Warning: Could not remove container: %s", err)
	}
	return nil
}

// getReferencesFromContainer uses a path to an image and a metadata.yaml within that image, and extracts the reference configs
// to a local temporary directory. Returns the path (referencesPath) to this directory.
func getReferencesFromContainer(path string) (string, error) {
	engine, err := hasPodmanOrDocker()
	if err != nil {
		return "", err
	}

	dname, err := os.MkdirTemp("", "kube-compare")
	if err != nil {
		return "", err
	}

	image, metadataPath, err := parsePath(path)
	if err != nil {
		return "", err
	}

	containerID, err := pullAndRunContainer(engine, image)
	if err != nil {
		return "", err
	}

	defer cleanup(engine, containerID)

	referencesPath, err := extractReferences(engine, containerID, metadataPath, dname)
	if err != nil {
		return "", err
	}
	return referencesPath, nil
}
