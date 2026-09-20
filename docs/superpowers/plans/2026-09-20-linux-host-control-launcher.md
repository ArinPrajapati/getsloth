# Linux Host Control Launcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically open the host control console in a supported Linux terminal emulator without blocking the host session.

**Architecture:** Keep `launchHostControlConsole` as the platform boundary. macOS continues using AppleScript unchanged; Linux resolves a terminal emulator from an explicit, testable priority list and starts it asynchronously with an argument array that launches the existing `getsloth control --socket <path> --watch` command. If resolution or process start fails, return an error so `main.go` preserves its current keyboard-shortcut fallback.

**Tech Stack:** Go standard library (`os/exec`, `runtime`), existing Bubble Tea control console, Go `testing`.

**Spec:** `docs/host-control-console-checklist.md` (Terminal-launch checklist; issue #1)

## Global Constraints

- Do not add suppression comments, unimplemented stubs, or secrets.
- The control socket remains local-only and is passed only as a process argument to a locally launched terminal.
- The relay must not receive password-comparison or authentication-decision logic.
- Linux launch must not invoke a shell or concatenate executable/socket values into shell syntax.
- When no supported emulator exists, keep the session alive and use the existing `Ctrl-] i` / `Ctrl-] r` fallback message.
- Preserve macOS behavior unchanged.

---

## File map

- `cmd/getsloth/host_control_launcher.go`: platform dispatch, Linux emulator detection, direct command arguments, asynchronous child-process launch.
- `cmd/getsloth/host_control_launcher_test.go`: deterministic tests for emulator priority, argument shape, and unsupported-platform/error fallback.
- `docs/host-control-console-checklist.md`: mark Linux terminal launch support complete while leaving unrelated checklist items unchanged.

### Task 1: Specify Linux terminal resolution and command arguments

**Files:**
- Modify: `cmd/getsloth/host_control_launcher_test.go`
- Modify: `cmd/getsloth/host_control_launcher.go`

**Interfaces:**
- Produces `findLinuxTerminal(lookPath func(string) (string, error)) (string, error)`.
- Produces `linuxHostControlLaunchArgs(terminalPath, executablePath, socketPath string) []string`.
- `findLinuxTerminal` checks this order: `gnome-terminal`, `konsole`, `xterm`, `x-terminal-emulator`.
- `linuxHostControlLaunchArgs` returns `-- <executable> control --socket <socket> --watch` for `gnome-terminal`, and `-e <executable> control --socket <socket> --watch` for the other three emulators.

- [ ] **Step 1: Write failing selection tests**

```go
func TestFindLinuxTerminal_PrefersGnomeTerminal(t *testing.T) {
    lookPath := func(name string) (string, error) {
        if name == "gnome-terminal" {
            return "/usr/bin/gnome-terminal", nil
        }
        return "", exec.ErrNotFound
    }

    got, err := findLinuxTerminal(lookPath)
    if err != nil || got != "/usr/bin/gnome-terminal" {
        t.Fatalf("findLinuxTerminal() = %q, %v", got, err)
    }
}

func TestFindLinuxTerminal_ReturnsErrorWhenNoneAreInstalled(t *testing.T) {
    _, err := findLinuxTerminal(func(string) (string, error) { return "", exec.ErrNotFound })
    if err == nil {
        t.Fatal("findLinuxTerminal() error = nil, want unsupported-terminal error")
    }
}
```

- [ ] **Step 2: Run the selection tests to verify failure**

Run: `go test ./cmd/getsloth -run '^TestFindLinuxTerminal_' -count=1`

Expected: compile failure because `findLinuxTerminal` does not exist.

- [ ] **Step 3: Write failing direct-argument tests**

```go
func TestLinuxHostControlLaunchArgs_UsesDirectCommandArguments(t *testing.T) {
    got := linuxHostControlLaunchArgs("/usr/bin/gnome-terminal", "/opt/Get Sloth/getsloth", "/tmp/control.sock")
    want := []string{"--", "/opt/Get Sloth/getsloth", "control", "--socket", "/tmp/control.sock", "--watch"}
    if !reflect.DeepEqual(got, want) {
        t.Fatalf("args = %#v, want %#v", got, want)
    }
}
```

- [ ] **Step 4: Implement the minimal pure helpers**

```go
var linuxTerminalCandidates = []string{"gnome-terminal", "konsole", "xterm", "x-terminal-emulator"}

func findLinuxTerminal(lookPath func(string) (string, error)) (string, error) {
    for _, name := range linuxTerminalCandidates {
        if path, err := lookPath(name); err == nil {
            return path, nil
        }
    }
    return "", errors.New("no supported Linux terminal emulator found")
}

func linuxHostControlLaunchArgs(terminalPath, executablePath, socketPath string) []string {
    command := []string{executablePath, "control", "--socket", socketPath, "--watch"}
    if strings.HasSuffix(terminalPath, "gnome-terminal") {
        return append([]string{"--"}, command...)
    }
    return append([]string{"-e"}, command...)
}
```

- [ ] **Step 5: Run focused helper tests**

Run: `go test ./cmd/getsloth -run '^(TestFindLinuxTerminal_|TestLinuxHostControlLaunchArgs_)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the testable Linux command selection**

```bash
git add cmd/getsloth/host_control_launcher.go cmd/getsloth/host_control_launcher_test.go
git commit -m "feat: select Linux terminals for host control"
```

### Task 2: Launch the selected Linux terminal without blocking the host

**Files:**
- Modify: `cmd/getsloth/host_control_launcher.go`
- Modify: `cmd/getsloth/host_control_launcher_test.go`

**Interfaces:**
- Produces `launchLinuxHostControlConsole(executablePath, socketPath string, lookPath func(string) (string, error), start func(string, []string) error) error`.
- `launchHostControlConsole` calls this only when `runtime.GOOS == "linux"`.
- The Linux `start` implementation runs `exec.Command(name, args...).Start()` and arranges `Wait()` in a goroutine to reap the child process. The existing macOS AppleScript path continues to use `Run()` unchanged.

- [ ] **Step 1: Write failing Linux fallback test**

```go
func TestLaunchLinuxHostControlConsole_DoesNotStartWhenNoTerminalExists(t *testing.T) {
    started := false
    err := launchLinuxHostControlConsole(
        "/usr/local/bin/getsloth", "/tmp/control.sock",
        func(string) (string, error) { return "", exec.ErrNotFound },
        func(string, []string) error { started = true; return nil },
    )
    if err == nil || started {
        t.Fatalf("err = %v, started = %t; want fallback error without a process", err, started)
    }
}
```

- [ ] **Step 2: Run the fallback test to verify failure**

Run: `go test ./cmd/getsloth -run '^TestLaunchLinuxHostControlConsole_DoesNotStartWhenNoTerminalExists$' -count=1`

Expected: compile failure because `launchLinuxHostControlConsole` does not exist.

- [ ] **Step 3: Implement Linux launch and platform dispatch**

```go
func launchLinuxHostControlConsole(executablePath, socketPath string, lookPath func(string) (string, error), start func(string, []string) error) error {
    terminalPath, err := findLinuxTerminal(lookPath)
    if err != nil {
        return err
    }
    return start(terminalPath, linuxHostControlLaunchArgs(terminalPath, executablePath, socketPath))
}

func launchHostControlConsole(socketPath string) error {
    executablePath, err := os.Executable()
    if err != nil {
        return fmt.Errorf("find getsloth executable: %w", err)
    }
    switch runtime.GOOS {
    case "darwin":
        name, args := hostControlLaunchArgs(executablePath, socketPath)
        if err := exec.Command(name, args...).Run(); err != nil {
            return fmt.Errorf("open Terminal host control console: %w", err)
        }
        return nil
    case "linux":
        return launchLinuxHostControlConsole(executablePath, socketPath, exec.LookPath, startLinuxHostControlLauncher)
    default:
        return fmt.Errorf("automatic host control console is not supported on %s", runtime.GOOS)
    }
}
```

`startHostControlLauncher` must call `cmd.Start()`, then call `cmd.Wait()` in a goroutine. It must return a contextual error if `Start()` fails. No terminal process is treated as a session failure.

- [ ] **Step 4: Run focused launcher tests**

Run: `go test ./cmd/getsloth -run '^(TestHostControlLaunchArgs_|TestFindLinuxTerminal_|TestLinuxHostControlLaunchArgs_|TestLaunchLinuxHostControlConsole_)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the platform launch behavior**

```bash
git add cmd/getsloth/host_control_launcher.go cmd/getsloth/host_control_launcher_test.go
git commit -m "feat: launch host control console on Linux"
```

### Task 3: Record completed Linux support and verify project gates

**Files:**
- Modify: `docs/host-control-console-checklist.md`

- [ ] **Step 1: Mark only Linux launcher support complete**

Change this checklist line:

```markdown
- [ ] Later: Linux terminal launcher support.
```

to:

```markdown
- [x] Linux terminal launcher support (`gnome-terminal`, `konsole`, `xterm`, and `x-terminal-emulator`; graceful keyboard-shortcut fallback when unavailable).
```

- [ ] **Step 2: Run all required checks**

Run:

```bash
./scripts/check.sh
go test ./... -cover
cd web && npm ci && npm run check && npm test -- --run
cd .. && "$(go env GOPATH)/bin/gosec" ./...
```

Expected: fast checks, test suites, and secrets scan PASS. Record existing coverage or security warnings without suppressing them.

- [ ] **Step 3: Commit documentation**

```bash
git add docs/host-control-console-checklist.md
git commit -m "docs: record Linux host control launcher support"
```
