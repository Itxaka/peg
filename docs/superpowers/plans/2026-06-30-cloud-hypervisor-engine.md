# Cloud-Hypervisor Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `cloud-hypervisor` engine to peg that implements `types.Machine` so existing specs/matchers run unchanged.

**Architecture:** New `pkg/machine/cloudhypervisor.go` mirrors `qemu.go` (process via `go-processmanager`, `monitor()` for OnFailure, SSH via `controller`). Boots firmware+disk (`--firmware`/`--disk`). Rootless networking via `pasta` wrapping the cloud-hypervisor process, forwarding host `127.0.0.1:<SSH.Port>` → guest `:22` (preserves the existing SSH model). Arg construction is split into pure, unit-tested helpers.

**Tech Stack:** Go 1.24+, ginkgo/gomega (suites), `github.com/mudler/go-processmanager`, cloud-hypervisor + pasta (host binaries), `qemu-img` (host, for blank disks).

## Global Constraints

- Module path: `github.com/spectrocloud/peg`. Go directive `go 1.24.0`.
- New engine string value: exactly `cloud-hypervisor`.
- Default networking flag reuses existing `MachineConfig.DisableDefaultNetworking`.
- ISO is NOT a boot source for this engine: log a warning if `iso` is set, do not attach it.
- Screenshot returns an error (not implemented); DetachCD is a no-op returning `nil`.
- Reuse existing helpers: `controller.SSHCommand/SendFile/ReceiveFile`, `monitor()`, `utils.SH`.
- Tests are plain Go `testing` table tests (the pure helpers are unexported; internal `package machine` / `package types` test files). Run with `go test ./...`.
- Commit message footer on every commit:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- All work on branch `cloud-hypervisor-engine`.

---

## File Structure

- Create `pkg/machine/cloudhypervisor.go` — `CloudHypervisor` type + pure arg/firmware/binary helpers.
- Create `pkg/machine/cloudhypervisor_test.go` (`package machine`) — table tests for the pure helpers + `machine.New` engine selection.
- Modify `pkg/machine/types/config.go` — `CloudHypervisor` engine const, `Firmware` field, `WithFirmware`, `CloudHypervisorEngine`.
- Create `pkg/machine/types/config_test.go` (`package types`) — option/parse tests.
- Modify `pkg/machine/machine.go` — `New()` switch case.
- Modify `main.go` — `--cloud-hypervisor` and `--firmware` CLI flags.
- Modify `README.md` — engine list + constraints.

---

### Task 1: Config wiring — engine const, Firmware field, options

**Files:**
- Modify: `pkg/machine/types/config.go`
- Test: `pkg/machine/types/config_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `const CloudHypervisor Engine = "cloud-hypervisor"`
  - field `MachineConfig.Firmware string` (yaml `firmware`)
  - `func WithFirmware(fw string) MachineOption`
  - `var CloudHypervisorEngine MachineOption` (sets `mc.Engine = CloudHypervisor`)

- [ ] **Step 1: Write the failing test**

Create `pkg/machine/types/config_test.go`:

```go
package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWithFirmware(t *testing.T) {
	mc := DefaultMachineConfig()
	if err := mc.Apply(WithFirmware("/fw/hypervisor-fw")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if mc.Firmware != "/fw/hypervisor-fw" {
		t.Fatalf("got %q", mc.Firmware)
	}
	// empty value must not overwrite
	if err := mc.Apply(WithFirmware("")); err != nil {
		t.Fatalf("apply empty: %v", err)
	}
	if mc.Firmware != "/fw/hypervisor-fw" {
		t.Fatalf("empty overwrote firmware: %q", mc.Firmware)
	}
}

func TestCloudHypervisorEngineOption(t *testing.T) {
	mc := DefaultMachineConfig()
	if err := mc.Apply(CloudHypervisorEngine); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if mc.Engine != CloudHypervisor {
		t.Fatalf("got engine %q", mc.Engine)
	}
}

func TestEngineYAMLParse(t *testing.T) {
	mc := DefaultMachineConfig()
	in := []byte("engine: cloud-hypervisor\nfirmware: /fw/CLOUDHV.fd\n")
	if err := yaml.Unmarshal(in, mc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mc.Engine != CloudHypervisor {
		t.Fatalf("engine %q", mc.Engine)
	}
	if mc.Firmware != "/fw/CLOUDHV.fd" {
		t.Fatalf("firmware %q", mc.Firmware)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/machine/types/ -run 'Firmware|CloudHypervisor|EngineYAML' -v`
Expected: FAIL — `undefined: WithFirmware`, `undefined: CloudHypervisorEngine`, `Firmware` field missing.

- [ ] **Step 3: Add the engine const**

In `pkg/machine/types/config.go`, extend the engine consts block:

```go
const (
	VBox             Engine = "vbox"
	QEMU             Engine = "qemu"
	Docker           Engine = "docker"
	CloudHypervisor  Engine = "cloud-hypervisor"
)
```

- [ ] **Step 4: Add the Firmware field**

In `MachineConfig`, add below the `Display` field:

```go
	// Firmware path for engines that boot via firmware (cloud-hypervisor).
	Firmware string `yaml:"firmware,omitempty"`
```

- [ ] **Step 5: Add WithFirmware and CloudHypervisorEngine**

Add `WithFirmware` next to the other `WithXxx` options:

```go
func WithFirmware(fw string) MachineOption {
	return func(mc *MachineConfig) error {
		if fw != "" {
			mc.Firmware = fw
		}
		return nil
	}
}
```

Add `CloudHypervisorEngine` next to `QEMUEngine`/`VBoxEngine`:

```go
// CloudHypervisorEngine sets the machine engine to cloud-hypervisor.
var CloudHypervisorEngine MachineOption = func(mc *MachineConfig) error {
	mc.Engine = CloudHypervisor
	return nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./pkg/machine/types/ -run 'Firmware|CloudHypervisor|EngineYAML' -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Commit**

```bash
git add pkg/machine/types/config.go pkg/machine/types/config_test.go
git commit -m "feat(types): add cloud-hypervisor engine const, Firmware field, options

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Pure helpers — arg builder, launch wrapper, path resolver

**Files:**
- Create: `pkg/machine/cloudhypervisor.go`
- Test: `pkg/machine/cloudhypervisor_test.go` (create)

**Interfaces:**
- Consumes: `types.MachineConfig`, `types.CloudHypervisor` (Task 1).
- Produces:
  - `func buildCloudHypervisorArgs(mc types.MachineConfig, firmware string, drives []string) []string`
  - `func buildLaunchCommand(mc types.MachineConfig, chBinary string, chArgs []string) (string, []string)`
  - `func firstExisting(paths []string) (string, error)`

- [ ] **Step 1: Write the failing test**

Create `pkg/machine/cloudhypervisor_test.go`:

```go
package machine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spectrocloud/peg/pkg/machine/types"
)

func cfg() types.MachineConfig {
	return types.MachineConfig{
		StateDir: "/state",
		CPU:      "2",
		Memory:   "2048",
		SSH:      &types.SSH{Port: "2222"},
	}
}

func joined(args []string) string { return strings.Join(args, " ") }

func TestBuildArgsFirmwareDrives(t *testing.T) {
	args := buildCloudHypervisorArgs(cfg(), "/fw/hypervisor-fw", []string{"/state/d0.img"})
	got := joined(args)
	for _, want := range []string{
		"--api-socket /state/ch-api.sock",
		"--cpus boot=2",
		"--memory size=2048M",
		"--firmware /fw/hypervisor-fw",
		"--disk path=/state/d0.img",
		"--net tap=,mac=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in: %s", want, got)
		}
	}
}

func TestBuildArgsDatasourceReadonly(t *testing.T) {
	c := cfg()
	c.DataSource = "/state/seed.img"
	args := buildCloudHypervisorArgs(c, "/fw", []string{"/state/d0.img"})
	if !strings.Contains(joined(args), "path=/state/seed.img,readonly=on") {
		t.Fatalf("datasource not attached readonly: %s", joined(args))
	}
}

func TestBuildArgsNetworkingDisabled(t *testing.T) {
	c := cfg()
	c.DisableDefaultNetworking = true
	args := buildCloudHypervisorArgs(c, "/fw", []string{"/state/d0.img"})
	if strings.Contains(joined(args), "--net") {
		t.Fatalf("--net present despite disabled networking: %s", joined(args))
	}
}

func TestBuildArgsAppendsUserArgs(t *testing.T) {
	c := cfg()
	c.Args = []string{"--rng", "src=/dev/urandom"}
	args := buildCloudHypervisorArgs(c, "/fw", nil)
	if !strings.HasSuffix(joined(args), "--rng src=/dev/urandom") {
		t.Fatalf("user args not appended last: %s", joined(args))
	}
}

func TestBuildLaunchCommandPasta(t *testing.T) {
	name, args := buildLaunchCommand(cfg(), "/bin/cloud-hypervisor", []string{"--cpus", "boot=2"})
	if name != "pasta" {
		t.Fatalf("expected pasta, got %s", name)
	}
	got := joined(args)
	for _, want := range []string{"--config-net", "-t 2222:22", "-- /bin/cloud-hypervisor", "--cpus boot=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in: %s", want, got)
		}
	}
}

func TestBuildLaunchCommandNoNet(t *testing.T) {
	c := cfg()
	c.DisableDefaultNetworking = true
	name, args := buildLaunchCommand(c, "/bin/cloud-hypervisor", []string{"--cpus", "boot=2"})
	if name != "/bin/cloud-hypervisor" {
		t.Fatalf("expected direct binary, got %s", name)
	}
	if joined(args) != "--cpus boot=2" {
		t.Fatalf("args altered: %s", joined(args))
	}
}

func TestFirstExisting(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "fw")
	if err := os.WriteFile(real, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := firstExisting([]string{filepath.Join(dir, "missing"), real})
	if err != nil || got != real {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := firstExisting([]string{filepath.Join(dir, "none")}); err == nil {
		t.Fatal("expected error when no path exists")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/machine/ -run 'BuildArgs|BuildLaunch|FirstExisting' -v`
Expected: FAIL — undefined `buildCloudHypervisorArgs`, `buildLaunchCommand`, `firstExisting`.

- [ ] **Step 3: Write the pure helpers**

Create `pkg/machine/cloudhypervisor.go`:

```go
package machine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	process "github.com/mudler/go-processmanager"
	"github.com/spectrocloud/peg/internal/utils"
	"github.com/spectrocloud/peg/pkg/controller"
	"github.com/spectrocloud/peg/pkg/machine/types"
)

type CloudHypervisor struct {
	machineConfig types.MachineConfig
	process       *process.Process
}

// commonCHBinaryPaths are searched (in order) when no Process override is set.
var commonCHBinaryPaths = []string{
	"/usr/bin/cloud-hypervisor",
	"/usr/local/bin/cloud-hypervisor",
	"/home/linuxbrew/.linuxbrew/bin/cloud-hypervisor",
}

// commonFirmwarePaths are searched (in order) when no Firmware is configured.
var commonFirmwarePaths = []string{
	"/usr/share/cloud-hypervisor/hypervisor-fw",
	"/usr/lib/cloud-hypervisor/hypervisor-fw",
	"/usr/share/cloud-hypervisor/CLOUDHV.fd",
	"/usr/share/edk2/x64/CLOUDHV.fd",
}

func firstExisting(paths []string) (string, error) {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("none of the candidate paths exist: %v", paths)
}

// buildCloudHypervisorArgs builds the cloud-hypervisor CLI args (excluding the
// binary name and any pasta wrapper). Pure function for testability.
func buildCloudHypervisorArgs(mc types.MachineConfig, firmware string, drives []string) []string {
	args := []string{
		"--api-socket", filepath.Join(mc.StateDir, "ch-api.sock"),
		"--cpus", fmt.Sprintf("boot=%s", mc.CPU),
		"--memory", fmt.Sprintf("size=%sM", mc.Memory),
		"--firmware", firmware,
		"--serial", "tty",
		"--console", "off",
	}

	disks := []string{}
	for _, d := range drives {
		disks = append(disks, fmt.Sprintf("path=%s", d))
	}
	if mc.DataSource != "" {
		disks = append(disks, fmt.Sprintf("path=%s,readonly=on", mc.DataSource))
	}
	if len(disks) > 0 {
		args = append(args, "--disk")
		args = append(args, disks...)
	}

	if !mc.DisableDefaultNetworking {
		args = append(args, "--net", "tap=,mac=")
	}

	args = append(args, mc.Args...)
	return args
}

// buildLaunchCommand wraps the cloud-hypervisor invocation with pasta when
// default networking is enabled. Pasta builds a rootless netns + tap and
// forwards host <SSH.Port> -> guest 22, preserving 127.0.0.1:<port> SSH.
func buildLaunchCommand(mc types.MachineConfig, chBinary string, chArgs []string) (string, []string) {
	if mc.DisableDefaultNetworking {
		return chBinary, chArgs
	}
	port := ""
	if mc.SSH != nil {
		port = mc.SSH.Port
	}
	pastaArgs := []string{"--config-net", "-t", fmt.Sprintf("%s:22", port), "--", chBinary}
	return "pasta", append(pastaArgs, chArgs...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/machine/ -run 'BuildArgs|BuildLaunch|FirstExisting' -v`
Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/machine/cloudhypervisor.go pkg/machine/cloudhypervisor_test.go
git commit -m "feat(machine): pure arg/launch/path helpers for cloud-hypervisor

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: CloudHypervisor methods + engine selection in machine.New

**Files:**
- Modify: `pkg/machine/cloudhypervisor.go`
- Modify: `pkg/machine/machine.go:161-168` (the `New()` engine switch)
- Test: `pkg/machine/cloudhypervisor_test.go`

**Interfaces:**
- Consumes: helpers from Task 2; `monitor()`, `controller.*`, `utils.SH`.
- Produces: `*CloudHypervisor` satisfying `types.Machine`; `machine.New` returns it for engine `cloud-hypervisor`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/machine/cloudhypervisor_test.go`:

```go
func TestNewSelectsCloudHypervisor(t *testing.T) {
	m, err := New(types.WithStateDir(t.TempDir()), types.CloudHypervisorEngine,
		types.WithSSHPort("2222"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := m.(*CloudHypervisor); !ok {
		t.Fatalf("expected *CloudHypervisor, got %T", m)
	}
}

func TestCloudHypervisorImplementsMachine(t *testing.T) {
	var _ types.Machine = (*CloudHypervisor)(nil)
}

func TestScreenshotNotImplemented(t *testing.T) {
	c := &CloudHypervisor{machineConfig: cfg()}
	if _, err := c.Screenshot(); err == nil {
		t.Fatal("expected screenshot error")
	}
}

func TestDetachCDNoop(t *testing.T) {
	c := &CloudHypervisor{machineConfig: cfg()}
	if err := c.DetachCD(); err != nil {
		t.Fatalf("DetachCD should be nil, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/machine/ -run 'NewSelects|ImplementsMachine|ScreenshotNot|DetachCDNoop' -v`
Expected: FAIL — `*CloudHypervisor` does not implement `types.Machine` (missing methods); `New` returns nil engine error.

- [ ] **Step 3: Add the interface methods**

Append to `pkg/machine/cloudhypervisor.go`:

```go
func (q *CloudHypervisor) resolveBinary() (string, error) {
	if q.machineConfig.Process != "" {
		return q.machineConfig.Process, nil
	}
	if p, err := firstExisting(commonCHBinaryPaths); err == nil {
		return p, nil
	}
	p, err := exec.LookPath("cloud-hypervisor")
	if err != nil {
		return "", fmt.Errorf("cloud-hypervisor not found in common paths or PATH: %w", err)
	}
	return p, nil
}

func (q *CloudHypervisor) resolveFirmware() (string, error) {
	if q.machineConfig.Firmware != "" {
		return q.machineConfig.Firmware, nil
	}
	fw, err := firstExisting(commonFirmwarePaths)
	if err != nil {
		return "", fmt.Errorf("no firmware configured and none found in common paths: %w", err)
	}
	return fw, nil
}

func (q *CloudHypervisor) driveSizes() []string {
	sizes := []string{}
	for _, s := range q.machineConfig.DriveSizes {
		sizes = append(sizes, fmt.Sprintf("%sM", s))
	}
	if len(sizes) == 0 {
		sizes = append(sizes, fmt.Sprintf("%sM", types.DefaultDriveSize))
	}
	return sizes
}

func (q *CloudHypervisor) CreateDisk(diskname, size string) error {
	if err := os.MkdirAll(q.machineConfig.StateDir, os.ModePerm); err != nil {
		return err
	}
	out, err := utils.SH(fmt.Sprintf("qemu-img create -f qcow2 %s %s",
		filepath.Join(q.machineConfig.StateDir, diskname), size))
	if err != nil {
		return fmt.Errorf("%s : %w", out, err)
	}
	return nil
}

func (q *CloudHypervisor) Create(ctx context.Context) (context.Context, error) {
	log.Info("Create cloud-hypervisor machine")

	if q.machineConfig.ISO != "" {
		log.Warn("cloud-hypervisor cannot boot ISO images; 'iso' is ignored. Provide a bootable disk image via 'image'/'drives'.")
	}

	userDrives := q.machineConfig.Drives
	if q.machineConfig.AutoDriveSetup && len(userDrives) == 0 {
		for i, s := range q.driveSizes() {
			filename := fmt.Sprintf("%s-%d.img", q.machineConfig.ID, i)
			if err := q.CreateDisk(filename, s); err != nil {
				return ctx, fmt.Errorf("creating disk with size %s: %w", s, err)
			}
			userDrives = append(userDrives, filepath.Join(q.machineConfig.StateDir, filename))
		}
	}

	binary, err := q.resolveBinary()
	if err != nil {
		return ctx, fmt.Errorf("failed to find cloud-hypervisor binary: %w", err)
	}
	firmware, err := q.resolveFirmware()
	if err != nil {
		return ctx, fmt.Errorf("failed to resolve firmware: %w", err)
	}

	chArgs := buildCloudHypervisorArgs(q.machineConfig, firmware, userDrives)
	name, args := buildLaunchCommand(q.machineConfig, binary, chArgs)

	log.Infof("Starting VM with %s [ Memory: %s, CPU: %s ]", binary, q.machineConfig.Memory, q.machineConfig.CPU)

	p := process.New(
		process.WithName(name),
		process.WithArgs(args...),
		process.WithStateDir(q.machineConfig.StateDir),
	)
	q.process = p

	newCtx := monitor(ctx, p, q.machineConfig.OnFailure)
	return newCtx, p.Run()
}

func (q *CloudHypervisor) Config() types.MachineConfig {
	return q.machineConfig
}

func (q *CloudHypervisor) Stop() error {
	return process.New(process.WithStateDir(q.machineConfig.StateDir)).Stop()
}

func (q *CloudHypervisor) Clean() error {
	if q.machineConfig.StateDir != "" {
		return os.RemoveAll(q.machineConfig.StateDir)
	}
	return nil
}

func (q *CloudHypervisor) Alive() bool {
	return process.New(process.WithStateDir(q.machineConfig.StateDir)).IsAlive()
}

func (q *CloudHypervisor) Command(cmd string) (string, error) {
	return controller.SSHCommand(q, cmd)
}

func (q *CloudHypervisor) Screenshot() (string, error) {
	return "", errors.New("Screenshot is not implemented in cloud-hypervisor machine")
}

func (q *CloudHypervisor) DetachCD() error {
	return nil // Not applicable: cloud-hypervisor has no CD-ROM concept.
}

func (q *CloudHypervisor) ReceiveFile(src, dst string) error {
	return controller.ReceiveFile(q, src, dst)
}

func (q *CloudHypervisor) SendFile(src, dst, permissions string) error {
	return controller.SendFile(q, src, dst, permissions)
}
```

- [ ] **Step 4: Add the engine switch case**

In `pkg/machine/machine.go`, in `New()`'s engine switch, add the case after `types.VBox`:

```go
	case types.VBox:
		return &VBox{machineConfig: *mc}, nil
	case types.CloudHypervisor:
		return &CloudHypervisor{machineConfig: *mc}, nil
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/machine/ -run 'NewSelects|ImplementsMachine|ScreenshotNot|DetachCDNoop' -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Run the full package + build**

Run: `go build ./... && go test ./pkg/... -v`
Expected: build OK; all `pkg/...` tests PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/machine/cloudhypervisor.go pkg/machine/cloudhypervisor_test.go pkg/machine/machine.go
git commit -m "feat(machine): implement cloud-hypervisor engine and wire into New

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: CLI flags in main.go

**Files:**
- Modify: `main.go` (flags block ~line 164-168; opts assembly ~line 185-201)

**Interfaces:**
- Consumes: `types.CloudHypervisorEngine`, `types.WithFirmware` (Task 1).
- Produces: `--cloud-hypervisor` bool flag, `--firmware` string flag.

- [ ] **Step 1: Add the flags**

In the `Flags` slice, after the `vbox` BoolFlag, add:

```go
			cli.BoolFlag{
				Name:   "cloud-hypervisor",
				Usage:  "forces cloud-hypervisor engine",
				EnvVar: "PEG_CLOUDHYPERVISOR",
			},
			cli.StringFlag{
				Name:   "firmware",
				Usage:  "firmware path for cloud-hypervisor (rust-hypervisor-firmware or CLOUDHV.fd)",
				EnvVar: "PEG_FIRMWARE",
			},
```

- [ ] **Step 2: Wire firmware into machineOpts**

In the `machineOpts := []types.MachineOption{...}` literal, add a line:

```go
				types.WithFirmware(c.String("firmware")),
```

- [ ] **Step 3: Wire the engine flag**

After the existing `if c.Bool("qemu") { ... }` block, add:

```go
			if c.Bool("cloud-hypervisor") {
				machineOpts = append(machineOpts, types.CloudHypervisorEngine)
			}
```

- [ ] **Step 4: Verify build + flag presence**

Run: `go build -o /tmp/peg . && /tmp/peg --help 2>&1 | grep -E 'cloud-hypervisor|firmware'`
Expected: build OK; both `--cloud-hypervisor` and `--firmware` lines printed.

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat(cli): add --cloud-hypervisor and --firmware flags

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Documentation

**Files:**
- Modify: `README.md` (Supported engines section, ~line 14-25)

**Interfaces:** none.

- [ ] **Step 1: Update the engines list and design notes**

In `README.md`, change the "Supported engines" list to include cloud-hypervisor and add a constraints note:

```markdown
## Supported engines

- QEMU (no KVM)
- Docker
- Virtualbox
- Cloud Hypervisor

They share the same common apis, so you can control machine created with the engines in the same way from a testing perspective.
```

Then add, after the existing QEMU design-notes paragraph:

```markdown
Cloud Hypervisor notes: it boots from a disk image via firmware (`--firmware` +
`--disk`), not from CD ISOs — the `iso` field is ignored for this engine, so
provide a bootable disk image via `image`/`drives` and a firmware via the
`firmware` field or `--firmware`. Rootless networking uses `pasta` (install it
on the host/CI) to forward the local SSH port to the guest, mirroring the QEMU
user-networking model. Set `disable_default_networking` to provide your own
`args` networking instead.
```

- [ ] **Step 2: Verify**

Run: `grep -n 'Cloud Hypervisor' README.md`
Expected: matches in both the list and the notes.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document cloud-hypervisor engine and its constraints

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Final verification (after all tasks)

- [ ] Run `go build ./... && go vet ./... && go test ./...` — all PASS.
- [ ] Run `gofmt -l .` — no files listed.
- [ ] Confirm `git log --oneline` shows one commit per task on `cloud-hypervisor-engine`.
