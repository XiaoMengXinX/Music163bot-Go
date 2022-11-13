package ps

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const (
	StatusStart  = "start"
	StartsUpdate = "update"
)

func GetProcessStatus() string {
	return os.Getenv("STATUS")
}

func SetProcessStatus(status string) {
	_ = os.Setenv("STATUS", status)
}

func SetDaemonPidEnv() {
	_ = os.Setenv("DAEMON_PID", fmt.Sprintf("%d", os.Getpid()))
}

func ForkProcess(status string) (pid int, err error) {
	path := os.Args[0]
	_ = os.Setenv("STATUS", status)

	environList := []string{}
	for _, value := range os.Environ() {
		environList = append(environList, value)
	}

	execSpec := &syscall.ProcAttr{
		Env:   environList,
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
	}

	return syscall.ForkExec(path, os.Args, execSpec)
}

func CheckProcessAlive(pid int) bool {
	if pid != 0 {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return false
		}
		s := strings.Split(string(stat), " ")
		if len(s) > 3 {
			return !(s[2] == "Z" || s[2] == "X")
		}
	}
	return false
}

func KillProcess(pid int) error {
	if pid != 0 {
		return syscall.Kill(pid, syscall.SIGTERM)
	}
	return nil
}

func KillDaemonProcess() {
	pid, _ := strconv.Atoi(os.Getenv("DAEMON_PID"))
	_ = KillProcess(pid)
}
