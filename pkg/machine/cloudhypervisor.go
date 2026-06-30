package machine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spectrocloud/peg/pkg/machine/types"
)

type CloudHypervisor struct {
	machineConfig types.MachineConfig
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
