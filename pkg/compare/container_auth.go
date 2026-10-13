package compare

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	dockercredentials "github.com/docker/cli/cli/config/credentials"
	"github.com/docker/cli/cli/config/types"
	helpercredentials "github.com/docker/docker-credential-helpers/credentials"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

const credentialHelperWaitDelay = 2 * time.Second

var (
	errCredentialConfiguration = errors.New("loading registry credential configuration failed")
	errCredentialHelper        = errors.New("registry credential helper failed")
	errCredentialHelperOutput  = errors.New("registry credential helper output exceeds limit")
)

type helperCredential struct {
	serverURL string
	username  string
	secret    string
}

type credentialHelperRunner interface {
	get(context.Context, string, string) (helperCredential, bool, error)
}

type execCredentialHelperRunner struct {
	maxOutput int
}

type boundedCapture struct {
	buf      []byte
	limit    int
	overflow bool
}

func (capture *boundedCapture) Write(data []byte) (int, error) {
	available := capture.limit - len(capture.buf)
	if available > 0 {
		keep := min(available, len(data))
		capture.buf = append(capture.buf, data[:keep]...)
	}
	if len(data) > available {
		capture.overflow = true
	}
	return len(data), nil
}

func (runner execCredentialHelperRunner) get(
	ctx context.Context,
	helperSuffix string,
	serverURL string,
) (helperCredential, bool, error) {
	if err := ctx.Err(); err != nil {
		return helperCredential{}, false, fmt.Errorf("credential helper context: %w", err)
	}

	output := &boundedCapture{limit: runner.maxOutput}
	defer clear(output.buf)

	// The command name is selected by the user's Docker-compatible auth file.
	// No shell is involved and output is contained below.
	cmd := exec.CommandContext(ctx, "docker-credential-"+helperSuffix, "get") //nolint:gosec
	cmd.Stdin = strings.NewReader(serverURL)
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = credentialHelperWaitDelay

	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return helperCredential{}, false, fmt.Errorf("credential helper context: %w", ctxErr)
	}
	if output.overflow {
		return helperCredential{}, false, errCredentialHelperOutput
	}
	if err != nil {
		if helpercredentials.IsErrCredentialsNotFoundMessage(string(output.buf)) {
			return helperCredential{}, true, nil
		}
		return helperCredential{}, false, errCredentialHelper
	}

	var response struct {
		ServerURL string
		Username  string
		Secret    string
	}
	if err := json.Unmarshal(output.buf, &response); err != nil {
		return helperCredential{}, false, errCredentialHelper
	}
	return helperCredential{
		serverURL: response.ServerURL,
		username:  response.Username,
		secret:    response.Secret,
	}, false, nil
}

// safeDefaultKeychain preserves the pinned DefaultKeychain's Docker-first,
// Podman-fallback lookup behavior while keeping helper execution contextual
// and preventing helper output from reaching logs or returned errors.
type safeDefaultKeychain struct {
	mu     sync.Mutex
	runner credentialHelperRunner
}

var _ authn.ContextKeychain = (*safeDefaultKeychain)(nil)
var _ authn.Keychain = (*safeDefaultKeychain)(nil)

func (keychain *safeDefaultKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	return keychain.ResolveContext(context.Background(), target)
}

func (keychain *safeDefaultKeychain) ResolveContext(
	ctx context.Context,
	target authn.Resource,
) (authn.Authenticator, error) {
	keychain.mu.Lock()
	defer keychain.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("registry credential context: %w", err)
	}

	configFile, err := loadRegistryAuthConfig()
	if err != nil {
		return nil, err
	}
	if configFile == nil {
		if os.Getenv(configfile.DockerEnvConfigKey) == "" {
			return authn.Anonymous, nil
		}
		configFile = configfile.New("")
	}

	environmentAuth, environmentAuthValid := parseDockerAuthEnvironment(os.Getenv(configfile.DockerEnvConfigKey))
	var empty types.AuthConfig
	for _, lookupKey := range []string{target.String(), target.RegistryStr()} {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("registry credential context: %w", err)
		}
		if lookupKey == name.DefaultRegistry {
			lookupKey = authn.DefaultAuthKey
		}

		authConfig, err := keychain.resolveAuthConfig(
			ctx,
			configFile,
			environmentAuth,
			environmentAuthValid,
			lookupKey,
		)
		if err != nil {
			return nil, err
		}
		authConfig.ServerAddress = ""
		if authConfig != empty {
			return authn.FromConfig(authn.AuthConfig{
				Username:      authConfig.Username,
				Password:      authConfig.Password,
				Auth:          authConfig.Auth,
				IdentityToken: authConfig.IdentityToken,
				RegistryToken: authConfig.RegistryToken,
			}), nil
		}
	}

	return authn.Anonymous, nil
}

func (keychain *safeDefaultKeychain) resolveAuthConfig(
	ctx context.Context,
	configFile *configfile.ConfigFile,
	environmentAuth map[string]types.AuthConfig,
	environmentAuthValid bool,
	lookupKey string,
) (types.AuthConfig, error) {
	if environmentAuthValid {
		if authConfig, found := environmentAuth[lookupKey]; found {
			return authConfig, nil
		}
	}

	helperSuffix := configFile.CredentialsStore
	if helper, found := configFile.CredentialHelpers[lookupKey]; found {
		helperSuffix = helper
	}
	if helperSuffix != "" {
		credential, notFound, err := keychain.runner.get(ctx, helperSuffix, lookupKey)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return types.AuthConfig{}, err
			}
			return types.AuthConfig{}, errCredentialHelper
		}
		if notFound {
			return types.AuthConfig{}, nil
		}
		authConfig := types.AuthConfig{ServerAddress: lookupKey}
		if credential.username == "<token>" {
			authConfig.IdentityToken = credential.secret
		} else {
			authConfig.Username = credential.username
			authConfig.Password = credential.secret
		}
		return authConfig, nil
	}

	authConfig, err := dockercredentials.NewFileStore(configFile).Get(lookupKey)
	if err != nil {
		return types.AuthConfig{}, errCredentialConfiguration
	}
	return authConfig, nil
}

func loadRegistryAuthConfig() (*configfile.ConfigFile, error) {
	home, homeErr := os.UserHomeDir()
	dockerConfigDir := os.Getenv(config.EnvOverrideConfigDir)
	foundDockerConfig := homeErr == nil && fileExists(filepath.Join(home, ".docker", config.ConfigFileName))
	if !foundDockerConfig && dockerConfigDir != "" {
		foundDockerConfig = fileExists(filepath.Join(dockerConfigDir, config.ConfigFileName))
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" && home != "" {
		configDir = filepath.Join(home, ".config")
	}
	podmanAuth := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "auth.json")
	if (os.Getenv("XDG_RUNTIME_DIR") == "" || !fileExists(podmanAuth)) && configDir != "" {
		podmanAuth = filepath.Join(configDir, "containers", "auth.json")
	}

	if foundDockerConfig {
		if dockerConfigDir == "" {
			dockerConfigDir = filepath.Join(home, ".docker")
		}
		configFile, err := config.Load(dockerConfigDir)
		if err != nil {
			return nil, errCredentialConfiguration
		}
		return configFile, nil
	}

	for _, authFile := range []string{os.Getenv("REGISTRY_AUTH_FILE"), podmanAuth} {
		if !fileExists(filepath.Clean(authFile)) {
			continue
		}
		file, err := os.Open(filepath.Clean(authFile))
		if err != nil {
			return nil, errCredentialConfiguration
		}
		configFile, loadErr := config.LoadFromReader(file)
		closeErr := file.Close()
		if loadErr != nil || closeErr != nil {
			return nil, errCredentialConfiguration
		}
		return configFile, nil
	}

	return nil, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type dockerEnvironmentConfig struct {
	AuthConfigs map[string]struct {
		Auth string `json:"auth"`
	} `json:"auths"`
}

func parseDockerAuthEnvironment(value string) (map[string]types.AuthConfig, bool) {
	if value == "" {
		return nil, false
	}

	var environment dockerEnvironmentConfig
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&environment); err != nil {
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, false
	}

	authConfigs := make(map[string]types.AuthConfig, len(environment.AuthConfigs))
	for address, encoded := range environment.AuthConfigs {
		if encoded.Auth == "" {
			return nil, false
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded.Auth)
		if err != nil {
			return nil, false
		}
		username, password, found := strings.Cut(string(decoded), ":")
		clear(decoded)
		if !found || username == "" {
			return nil, false
		}
		authConfigs[address] = types.AuthConfig{
			Username:      username,
			Password:      strings.Trim(password, "\x00"),
			ServerAddress: address,
		}
	}
	return authConfigs, true
}
