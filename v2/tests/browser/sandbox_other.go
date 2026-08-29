//go:build !windows

package browserqualification

const SandboxProbeHelperCommand = "__mindweaver_internal_job_probe"

func processSandboxAvailable() bool { return false }

func RunSandboxProbeHelper([]string) int { return 81 }
