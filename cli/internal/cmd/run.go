package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/AnchorageLabs/envy/cli/internal/api"
	"github.com/AnchorageLabs/envy/cli/internal/config"
	"github.com/spf13/cobra"
)

type runOptions struct {
	root *rootOptions
	env  string
}

// runExitError signals the CLI to exit with a specific code while still
// surfacing a clear message to stderr (handled by Execute via ExitCode()).
type runExitError struct {
	msg  string
	code int
}

func (e *runExitError) Error() string { return e.msg }
func (e *runExitError) ExitCode() int { return e.code }

func newRunCommand(root *rootOptions) *cobra.Command {
	opts := &runOptions{root: root}

	cmd := &cobra.Command{
		Use:   "run [--env <name>] -- <cmd> [args...]",
		Short: "Run a command with the resolved environment's variables injected",
		Long: "Resolve the target environment, fetch its locked version values via the API, and execute the command after `--` with those KEY=VALUE pairs merged into the process environment (fetched values override on collision). No .env.local is written. stdin/stdout/stderr are forwarded, SIGINT/SIGTERM are relayed to the child, and the child's exit code is propagated.",
		// Disable cobra's flag parsing on the trailing command so flags meant for
		// the child are not interpreted by envy. Flags before `--` are still parsed.
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd, opts, args)
		},
	}

	cmd.Flags().StringVar(&opts.env, "env", "", "environment name (overrides the bound environment)")

	return cmd
}

func runRun(cmd *cobra.Command, opts *runOptions, args []string) error {
	// Split flags from the trailing command using the position of `--`.
	dashIdx := cmd.ArgsLenAtDash()
	if dashIdx < 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: missing command: usage `envy run [--env <name>] -- <cmd> [args...]`")
		return &runExitError{msg: "missing command after `--`", code: 1}
	}

	commandArgs := args[dashIdx:]
	if len(commandArgs) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: missing command after `--`: usage `envy run [--env <name>] -- <cmd> [args...]`")
		return &runExitError{msg: "missing command after `--`", code: 1}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	projectConfig, err := config.LoadProjectConfig(cwd)
	if err != nil {
		if errors.Is(err, config.ErrProjectConfigNotFound) {
			return fmt.Errorf("no ENVY project found: run `envy init` first")
		}
		return err
	}

	lockPath := filepath.Join(cwd, ".envy", "lock.json")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.ErrOrStderr(), "error: .envy/lock.json not found: run `envy pull` first")
			return &runExitError{msg: ".envy/lock.json not found", code: 1}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to read .envy/lock.json: %v\n", err)
		return &runExitError{msg: err.Error(), code: 1}
	}

	var lock lockFile
	if err := json.Unmarshal(lockData, &lock); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to parse .envy/lock.json: %v\n", err)
		return &runExitError{msg: err.Error(), code: 1}
	}

	envName := strings.TrimSpace(opts.env)
	if envName == "" {
		envName = strings.TrimSpace(projectConfig.Environment)
	}
	if envName == "" {
		// Fall back to the sole environment in the lockfile if unambiguous.
		if len(lock.Environments) == 1 {
			for name := range lock.Environments {
				envName = name
			}
		}
	}
	if envName == "" {
		return fmt.Errorf("no environment specified: pass one with --env or set a default environment with `envy init`")
	}

	block, ok := lock.Environments[envName]
	if !ok {
		return fmt.Errorf("environment %q not found in .envy/lock.json: run `envy pull %s` first", envName, envName)
	}

	project := projectConfig.Project
	if project == "" {
		project = lock.Project
	}

	if strings.TrimSpace(opts.root.resolvedAPIURL) == "" {
		return fmt.Errorf("api url is required: configure it in .envy/config.json, set ENVY_API_URL, or pass --api-url")
	}

	client := api.NewClient(opts.root.resolvedAPIURL, opts.root.apiToken, nil)

	values, err := client.GetEnvironmentVersionValues(project, envName, block.Version)
	if err != nil {
		return err
	}

	// Build the child env in-memory: start from the parent env, then overlay
	// fetched values so they win on key collision. Nothing is written to disk.
	childEnv := mergeEnv(os.Environ(), values)

	return spawnAndWait(cmd, commandArgs, childEnv)
}

// mergeEnv overlays the supplied KEY=VALUE map onto the parent environment.
// Fetched values override existing keys.
func mergeEnv(parent []string, overrides map[string]string) []string {
	index := make(map[string]int, len(parent))
	merged := make([]string, len(parent))
	copy(merged, parent)

	for i, kv := range merged {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			index[kv] = i
			continue
		}
		index[kv[:eq]] = i
	}

	for k, v := range overrides {
		entry := k + "=" + v
		if i, ok := index[k]; ok {
			merged[i] = entry
			continue
		}
		index[k] = len(merged)
		merged = append(merged, entry)
	}

	return merged
}

// spawnAndWait executes the command with the supplied env, forwarding stdio and
// relaying SIGINT/SIGTERM to the child, and propagates the child's exit code.
func spawnAndWait(cmd *cobra.Command, commandArgs []string, env []string) error {
	name := commandArgs[0]
	rest := commandArgs[1:]

	child := exec.Command(name, rest...)
	child.Env = env
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	if err := child.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: command not found: %s\n", name)
			return &runExitError{msg: fmt.Sprintf("command not found: %s", name), code: 127}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "error: failed to start command %q: %v\n", name, err)
		return &runExitError{msg: err.Error(), code: 1}
	}

	// Relay signals to the child process.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-sigCh:
				if child.Process != nil {
					_ = child.Process.Signal(sig)
				}
			case <-done:
				return
			}
		}
	}()

	waitErr := child.Wait()
	close(done)
	signal.Stop(sigCh)

	if waitErr == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			// Terminated by a signal; map to the conventional 128+signal code.
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				code = 128 + int(status.Signal())
			} else {
				code = 1
			}
		}
		return &runExitError{msg: waitErr.Error(), code: code}
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "error: command failed: %v\n", waitErr)
	return &runExitError{msg: waitErr.Error(), code: 1}
}
