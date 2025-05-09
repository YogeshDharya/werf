package ssh_agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/werf/common-go/pkg/util"
	"github.com/werf/logboek"
	secret_common "github.com/werf/nelm/pkg/secret"
	"github.com/werf/werf/v2/pkg/werf"
)

const (
	SSHAuthSockEnv = "SSH_AUTH_SOCK"
)

var (
	SSHAuthSock string
	tmpSockPath string
)

var (
	ErrSocketPathTooLong = errors.New("system ssh-agent socket path length exceeds the limit")
	ErrSocketPathEmpty   = errors.New("system ssh-agent socket path is empty")
)

func setupProcessSSHAgent(sshAuthSock string) error {
	SSHAuthSock = sshAuthSock
	return os.Setenv(SSHAuthSockEnv, SSHAuthSock)
}

type sshKeyConfig struct {
	FilePath   string
	Passphrase []byte
}

type sshKey struct {
	Config     sshKeyConfig
	PrivateKey interface{}
}

func getSshKeyConfig(path string) (sshKeyConfig, error) {
	var filePath string
	var passphrase []byte

	switch {
	case strings.HasPrefix(path, "file://"):
		userinfoWithPath := strings.TrimPrefix(path, "file://")

		parts := strings.SplitN(userinfoWithPath, "@", 2)
		passphrase = []byte(parts[0])

		var err error
		filePath, err = util.ExpandPath(parts[1])
		if err != nil {
			return sshKeyConfig{}, fmt.Errorf("error expanding path %q: %w", parts[1], err)
		}
	default:
		var err error
		filePath, err = util.ExpandPath(path)
		if err != nil {
			return sshKeyConfig{}, fmt.Errorf("error expanding path %q: %w", path, err)
		}
	}

	if keyExists, err := util.FileExists(filePath); !keyExists {
		return sshKeyConfig{}, fmt.Errorf("specified ssh key does not exist")
	} else if err != nil {
		return sshKeyConfig{}, fmt.Errorf("specified ssh key does not exist: %w", err)
	}

	return sshKeyConfig{FilePath: filePath, Passphrase: passphrase}, nil
}

type loadSshKeysOptions struct {
	WarnInvalidKeys bool
}

func loadSshKeys(ctx context.Context, configs []sshKeyConfig, opts loadSshKeysOptions) ([]sshKey, error) {
	var res []sshKey

	for _, cfg := range configs {
		sshKey, err := parsePrivateSSHKey(cfg)
		if err != nil {
			if opts.WarnInvalidKeys {
				logboek.Context(ctx).Warn().LogF("WARNING: unable to parse ssh key %s: %s\n", cfg.FilePath, err)
				continue
			} else {
				return nil, fmt.Errorf("unable to parse ssh key %s: %w", cfg.FilePath, err)
			}
		}

		res = append(res, sshKey)
	}

	return res, nil
}

func Init(ctx context.Context, userKeys []string) error {
	var configs []sshKeyConfig

	for _, key := range userKeys {
		cfg, err := getSshKeyConfig(key)
		if err != nil {
			return fmt.Errorf("unable to get ssh key %s config: %w", key, err)
		}

		configs = append(configs, cfg)
	}

	if len(configs) > 0 {
		keys, err := loadSshKeys(ctx, configs, loadSshKeysOptions{})
		if err != nil {
			return fmt.Errorf("unable to load ssh keys: %w", err)
		}

		if len(keys) > 0 {
			agentSock, err := runSSHAgentWithKeys(ctx, keys)
			if err != nil {
				return fmt.Errorf("unable to run ssh agent with specified keys: %w", err)
			}
			if err := setupProcessSSHAgent(agentSock); err != nil {
				return fmt.Errorf("unable to init ssh auth socket to %q: %w", agentSock, err)
			}
			return nil
		}
	}

	systemAgentSock := os.Getenv(SSHAuthSockEnv)
	validSystemAgentSock, err := validateAgentSock(systemAgentSock)
	if err != nil {
		if errors.Is(err, ErrSocketPathEmpty) {
			logboek.Context(ctx).Debug().LogF("System ssh agent not found\n")
		} else {
			return err
		}
	}
	if systemAgentSock != "" && validSystemAgentSock {
		SSHAuthSock = systemAgentSock
		logboek.Context(ctx).Info().LogF("Using system ssh-agent: %s\n", systemAgentSock)
		return nil
	}

	var defaultConfigs []sshKeyConfig
	for _, defaultFileName := range []string{"id_rsa", "id_dsa"} {
		path := filepath.Join(os.Getenv("HOME"), ".ssh", defaultFileName)

		if keyExists, _ := util.FileExists(path); keyExists {
			defaultConfigs = append(defaultConfigs, sshKeyConfig{FilePath: path})
		}
	}

	if len(defaultConfigs) > 0 {
		keys, err := loadSshKeys(ctx, defaultConfigs, loadSshKeysOptions{WarnInvalidKeys: true})
		if err != nil {
			return fmt.Errorf("unable to load ssh keys: %w", err)
		}

		if len(keys) > 0 {
			agentSock, err := runSSHAgentWithKeys(ctx, keys)
			if err != nil {
				return fmt.Errorf("unable to run ssh agent with specified keys: %w", err)
			}
			if err := setupProcessSSHAgent(agentSock); err != nil {
				return fmt.Errorf("unable to init ssh auth socket to %q: %w", agentSock, err)
			}
		}
	}

	return nil
}

func Terminate() error {
	if tmpSockPath != "" {
		err := os.RemoveAll(tmpSockPath)
		if err != nil {
			return fmt.Errorf("unable to remove tmp ssh agent sock %s: %w", tmpSockPath, err)
		}
	}

	return nil
}

func runSSHAgentWithKeys(ctx context.Context, keys []sshKey) (string, error) {
	agentSock, err := runSSHAgent(ctx)
	if err != nil {
		return "", fmt.Errorf("error running ssh agent: %w", err)
	}

	for _, key := range keys {
		err := addSSHKey(ctx, agentSock, key)
		if err != nil {
			return "", fmt.Errorf("error adding ssh key: %w", err)
		}
	}

	return agentSock, nil
}

func runSSHAgent(ctx context.Context) (string, error) {
	sockPath := filepath.Join(werf.GetTmpDir(), "werf-ssh-agent", uuid.NewString())
	err := validateSockPathLength(sockPath)
	if err != nil {
		logboek.Context(ctx).Warn().LogF("WARNING: unable to use unix sock path %s: %s\n", sockPath, err)
		sockPath = fallbackToDefaultUnix()
		logboek.Context(ctx).Warn().LogF("WARNING: fallback to %s\n", sockPath)
	}

	tmpSockPath = sockPath

	err = os.MkdirAll(filepath.Dir(sockPath), os.ModePerm)
	if err != nil {
		return "", err
	}

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("error listen unix sock %s: %w", sockPath, err)
	}

	logboek.Context(ctx).Info().LogF("Running ssh agent on unix sock: %s\n", sockPath)

	go func() {
		agnt := agent.NewKeyring()

		for {
			conn, err := ln.Accept()
			if err != nil {
				logboek.Context(ctx).Warn().LogF("WARNING: failed to accept ssh-agent connection: %s\n", err)
				continue
			}

			go func() {
				var err error

				err = agent.ServeAgent(agnt, conn)
				if err != nil && err != io.EOF {
					logboek.Context(ctx).Warn().LogF("WARNING: ssh-agent server error: %s\n", err)
					return
				}

				err = conn.Close()
				if err != nil {
					logboek.Context(ctx).Warn().LogF("WARNING: ssh-agent server connection close error: %s\n", err)
					return
				}
			}()
		}
	}()

	return sockPath, nil
}

func addSSHKey(ctx context.Context, authSock string, key sshKey) error {
	conn, err := net.Dial("unix", authSock)
	if err != nil {
		return fmt.Errorf("error dialing with ssh agent %s: %w", authSock, err)
	}
	defer conn.Close()

	agentClient := agent.NewClient(conn)

	err = agentClient.Add(agent.AddedKey{PrivateKey: key.PrivateKey})
	if err != nil {
		return err
	}

	logboek.Context(ctx).Info().LogF("Added private key %s to ssh agent %s\n", key.Config.FilePath, authSock)

	return nil
}

func parsePrivateSSHKey(cfg sshKeyConfig) (sshKey, error) {
	keyData, err := ioutil.ReadFile(cfg.FilePath)
	if err != nil {
		return sshKey{}, fmt.Errorf("error reading key file %q: %w", cfg.FilePath, err)
	}

	var privateKey interface{}

	privateKey, err = ssh.ParseRawPrivateKey(keyData)
	if err != nil {
		switch err.(type) {
		case *ssh.PassphraseMissingError:
			var passphrase []byte
			if len(cfg.Passphrase) == 0 {
				if terminal.IsTerminal(int(os.Stdin.Fd())) {
					if data, err := secret_common.InputFromInteractiveStdin(fmt.Sprintf("Enter passphrase for ssh key %s: ", cfg.FilePath)); err != nil {
						return sshKey{}, fmt.Errorf("error getting passphrase for ssh key %s: %w", cfg.FilePath, err)
					} else {
						passphrase = data
					}
				} else {
					return sshKey{}, fmt.Errorf(`%w: please provide passphrase using --ssh-add="file://PASSPHRASE@FILEPATH" format`, err)
				}
			} else {
				passphrase = cfg.Passphrase
			}

			privateKey, err = ssh.ParseRawPrivateKeyWithPassphrase(keyData, passphrase)
			if err != nil {
				return sshKey{}, fmt.Errorf("error parsing private key %s: %w", cfg.FilePath, err)
			}

		default:
			return sshKey{}, fmt.Errorf("error parsing private key %s: %w", cfg.FilePath, err)
		}
	}

	return sshKey{Config: cfg, PrivateKey: privateKey}, nil
}

func validateAgentSock(sock string) (bool, error) {
	if sock == "" {
		return false, ErrSocketPathEmpty
	}

	err := validateSockPathLength(sock)
	if err != nil {
		return false, fmt.Errorf("unable to use system ssh sock '%s': %w", sock, err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(sock)
		if err != nil {
			return false, err
		}

		if info.Mode()&os.ModeSocket == 0 {
			return false, fmt.Errorf("system ssh-agent socket `%s` is not a socket", sock)
		}
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return false, fmt.Errorf("unable to connect to system ssh-agent socket %s: %w", sock, err)
	}
	defer conn.Close()

	return true, nil
}

func getUnixSocketPathLimit() int {
	switch runtime.GOOS {
	case "darwin", "freebsd", "openbsd", "netbsd":
		return 104
	default:
		return 108
	}
}

func validateSockPathLength(sockPath string) error {
	if len(sockPath) > getUnixSocketPathLimit() {
		return ErrSocketPathTooLong
	}
	return nil
}

func fallbackToDefaultUnix() string {
	// since user can change tmpDir we want to prevent it from setting path more than 104/108 chars
	return filepath.Join(os.TempDir(), "werf-ssh-agent", uuid.NewString())
}
