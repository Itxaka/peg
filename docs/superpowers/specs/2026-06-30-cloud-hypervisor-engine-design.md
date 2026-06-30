# Cloud-Hypervisor Engine — Design

Date: 2026-06-30
Status: Approved (pending spec review)

## Goal

Add a `cloud-hypervisor` engine to peg alongside the existing QEMU, Docker, and
VirtualBox engines. Cloud-hypervisor is faster and leaner than QEMU and has a
smaller surface to manage. The new engine implements the same `types.Machine`
interface so existing test suites and the matcher helpers work unchanged.

## Background / constraints (verified)

- **Boot:** cloud-hypervisor cannot boot El-Torito CD ISOs the way QEMU does
  (`-drive media=cdrom`). It boots either a direct kernel (`--kernel`) or a
  disk image via firmware (`--firmware`). We use **firmware + disk**.
  CLI: `--firmware <fw> --disk path=<d1> path=<d2> ...`.
- **Networking:** cloud-hypervisor has **no native passt support**. Its
  `--net socket=` is vhost-user, not passt's qemu-stream protocol. `--net`
  accepts only `tap=`, `fd=[...]`, `ip=`, `mask=`, `mac=`. Rootless user-mode
  networking is therefore done with **pasta** (passt's slirp4netns-mode twin):
  pasta builds a netns + tap + host port-forward, and cloud-hypervisor runs
  inside that netns on the tap. This preserves peg's `ssh 127.0.0.1:<port>`
  model exactly like QEMU's `-nic user,hostfwd`, so the SSH controller needs
  no changes.
- **Control surface:** cloud-hypervisor exposes an `--api-socket` (analogous to
  the qemu monitor) but has **no screendump** and **no CD-ROM** concept.

## Scope decisions

| Topic | Decision |
|-------|----------|
| Networking | **pasta** — resembles QEMU, preserves `127.0.0.1:<port>` SSH, no controller change. Requires `pasta` on host/CI. |
| Boot | **firmware + disk**. New `firmware` config field; fall back to common firmware paths, else error. |
| ISO field | **Ignore for boot + warn.** cloud-hypervisor can't boot CD ISOs; log a warning if `iso` is set, require a bootable disk image. |
| Screenshot | Not implemented — return `errors.New("Screenshot is not implemented in cloud-hypervisor machine")` (like Docker). |
| DetachCD | No-op `nil` (no CD concept). |
| Command / SendFile / ReceiveFile | Reuse `controller` SSH/SCP, identical to QEMU. |
| Process mgmt | Reuse `go-processmanager` + `monitor()` for OnFailure, identical to QEMU. |
| Auto drives | Reuse `qemu-img create -f qcow2` (already a host requirement for QEMU). cloud-hypervisor reads qcow2. |
| datasource | Attach as extra `--disk path=<ds>,readonly=on` (cloud-init seed image use case). |

## Components

### New file `pkg/machine/cloudhypervisor.go`

`type CloudHypervisor struct { machineConfig types.MachineConfig; process *process.Process }`

Methods (implementing `types.Machine`):

- `Create(ctx) (context.Context, error)`
  1. Auto-create blank qcow2 drives if `AutoDriveSetup && len(Drives)==0` (reuse `CreateDisk`).
  2. Warn if `ISO != ""` (unsupported boot source for this engine).
  3. Resolve binary: `machineConfig.Process` override, else `findCloudHypervisorBinary()`.
  4. Resolve firmware: `machineConfig.Firmware` override, else search common firmware paths, else error.
  5. Build args via pure `buildArgs()` (see below).
  6. If default networking enabled, wrap launch with pasta: process name = `pasta`,
     args = `--config-net -t <SSH.Port>:22 -- <ch-binary> <ch-args with --net tap=...>`.
     If `DisableDefaultNetworking`, run cloud-hypervisor directly and rely on user `Args`.
  7. `process.New(...)`, `q.process = p`, `newCtx := monitor(ctx, p, OnFailure)`, `return newCtx, p.Run()`.
- `Config()` → returns `machineConfig`.
- `Stop()` → `process.New(WithStateDir).Stop()`.
- `Clean()` → `os.RemoveAll(StateDir)`.
- `Alive()` → `process.New(WithStateDir).IsAlive()`.
- `CreateDisk(diskname, size)` → `qemu-img create -f qcow2` into StateDir (same as QEMU).
- `Command(cmd)` → `controller.SSHCommand(c, cmd)`.
- `Screenshot()` → error, not implemented.
- `DetachCD()` → `nil`.
- `ReceiveFile` / `SendFile` → `controller.ReceiveFile` / `controller.SendFile`.
- helpers: `findCloudHypervisorBinary()` (common paths + `exec.LookPath`),
  `apiSockFile()` → `StateDir/ch-api.sock`, `driveSizes()` (same as QEMU),
  `buildArgs()` (pure, testable).

`buildArgs()` produces, in order:
```
--api-socket <StateDir>/ch-api.sock
--cpus boot=<CPU>
--memory size=<Memory>M
--firmware <fw>
--disk path=<d1> path=<d2> [path=<datasource>,readonly=on]
--serial tty --console off        # headless, equivalent of QEMU -nographic
--net tap=,mac=                    # tap supplied by pasta's netns (only when networking enabled)
<machineConfig.Args...>
```
CPUType (`--cpu`-equivalent) is QEMU-only and omitted here.

### `pkg/machine/types/config.go`

- Add `CloudHypervisor Engine = "cloud-hypervisor"` to the Engine consts.
- Add field `Firmware string \`yaml:"firmware,omitempty"\`` to `MachineConfig`.
- Add `WithFirmware(fw string) MachineOption`.
- Add `var CloudHypervisorEngine MachineOption` (sets `mc.Engine = CloudHypervisor`).

### `pkg/machine/machine.go`

- Add `case types.CloudHypervisor: return &CloudHypervisor{machineConfig: *mc}, nil`
  to the engine switch in `New()`.

### `main.go`

- Add `--cloud-hypervisor` bool flag (env `PEG_CLOUDHYPERVISOR`) → appends
  `types.CloudHypervisorEngine`.
- Add `--firmware` string flag (env `PEG_FIRMWARE`) → `types.WithFirmware(...)`.

## Data flow

Unchanged from existing engines: spec YAML → `MachineConfig` → `machine.New`
selects `CloudHypervisor` → `Create()` launches (pasta →) cloud-hypervisor →
ginkgo specs run commands via `controller` SSH on `127.0.0.1:<SSH.Port>` →
`Stop()`/`Clean()` teardown.

## Error handling

- Missing binary / firmware → descriptive error from `Create()` (wrapped `%w`).
- `monitor()` calls `OnFailure` on non-zero VM exit (same as QEMU).
- pasta missing → `Create()` returns the process start error; documented host
  dependency in README.

## Testing

Repo currently has only ginkgo suite bootstraps (no real specs). Add a focused
unit spec in `pkg/machine` exercising the **pure** `buildArgs()` for: firmware
present, drives present, datasource readonly attach, ISO-set warning path,
networking enabled vs `DisableDefaultNetworking`. Also test
`findCloudHypervisorBinary()` fallback ordering. No live-VM test (matches the
absence of live tests for the other engines).

## Documentation

Update README "Supported engines" to list cloud-hypervisor and note the
`pasta` host requirement and firmware/disk-image (no ISO boot) constraints.

## Out of scope (YAGNI)

- Direct kernel boot (`--kernel`/`--initramfs`/`--cmdline`).
- tap+bridge / guest-IP SSH path (would require controller changes).
- API-socket-driven hot-plug / device management beyond what the interface needs.
- Screenshot support.
