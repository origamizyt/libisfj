package isfj

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Limits on resources.
// 0 means no limit.
type Limits struct {
    // Time limit, in microseconds.
    Time	        uint64
    // Stack memory limit, in bytes.
    StackMemory	    uint64
    // Heap memory limit, in bytes.
    HeapMemory   uint64
}

// Resource usages.
type Usages struct {
    // Time of execution, in microseconds.
    Time    uint64
    // Stack + heap memory, in bytes.
    Memory  uint64
}

// Checks if every limit is 0.
func (u Limits) IsAllUnlimited() bool {
    return u.Time == 0 && u.StackMemory == 0 && u.HeapMemory == 0
}

// Input to [Run].
type RunnerInput struct {
    // Executable path.
    Executable	string
    // Arguments to executable, without argv[0].
    Arguments	[]string
    // Needle library to inject.
    NeedleLib	string
    // Content to write to child's stdin.
    Stdin		string
    // Resource limits.
    Limits		Limits
}

// Output from [Run].
type RunnerOutput struct {
    // Status of execution.
    // [ST_ACCEPTED] means success.
    Status 		Status
    // Stdout read from child.
    Stdout		string
    // Memory usages.
    Usages		Usages
    // Point deduction caused by syscalls.
    Deduction	int
    // Exit information.
    //
    // If Status == [ST_ACCEPTED], this is the exit code.
    // If Status == [ST_RUNTIME_ERROR], this is the terminating signal.
    // If Status == [ST_HOSTILE_CODE], this is the resulting syscall.
    ExitInfo	int
}

type syscallInfo struct {
    Op 					uint8
    Arch 				uint32
    InstructionPointer 	uint64
    StackPointer 		uint64
    Seccomp 			struct {
        Nr 		uint64
        Args 	[6]uint64
        RetData	uint32
    }
}

func ptraceGetSyscallInfo(pid int) (syscallInfo, error) {
    info := syscallInfo{}
    _, _, err := unix.Syscall6(
        unix.SYS_PTRACE, 
        unix.PTRACE_GET_SYSCALL_INFO, 
        uintptr(pid), 
        unsafe.Sizeof(info),
        uintptr(unsafe.Pointer(&info)),
        0,
        0,
    )
    return info, err
}

func vforkExec(executable string, args []string, env []string, stdin *os.File, stdout *os.File) (int, error) {
    process, err := os.StartProcess(executable, args[1:], &os.ProcAttr{
        Dir: path.Dir(executable),
        Env: env,
        Files: []*os.File { stdin, stdout, stdout },
        Sys: &unix.SysProcAttr{
            Ptrace: true,
        },
    })
    return process.Pid, err
}

func getMemoryUsages(pid int) (stack uint64, heap uint64, err error) {
    statFile, err := os.Open(path.Join("/proc", strconv.Itoa(pid), "status"))
    if err != nil {
        return
    }
    scanner := bufio.NewScanner(statFile)
    for scanner.Scan() {
        line := scanner.Text()
        if line, ok := strings.CutPrefix(line, "VmData:"); ok {
            line = strings.TrimSpace(line)
            num, _, _ := strings.Cut(line, " ") // unit should be kB
            n, _ := strconv.Atoi(num)
            heap = uint64(n * 1024)
        } else if line, ok := strings.CutPrefix(line, "VmStk:"); ok {
            line = strings.TrimSpace(line)
            num, _, _ := strings.Cut(line, " ") // unit should be kB
            n, _ := strconv.Atoi(num)
            stack = uint64(n * 1024)
        }
    }
    err = scanner.Err()
    return
}

// Runs given program.
func Run(input RunnerInput) RunnerOutput {
    stdinR, stdinW, err := os.Pipe()
    if err != nil {
        return RunnerOutput{
            Status: ST_SYSTEM_ERROR,
            Stdout: "",
            Deduction: 0,
            ExitInfo: 0,
        }
    }
    defer stdinR.Close()
    stdinW.WriteString(input.Stdin)
    stdinW.Close()
    stdoutR, stdoutW, err := os.Pipe()
    if err != nil {
        return RunnerOutput{
            Status: ST_SYSTEM_ERROR,
            Stdout: "",
            Deduction: 0,
            ExitInfo: 0,
        }
    }
    defer stdoutR.Close()
    defer stdoutW.Close()

    args := make([]string, 0, len(input.Arguments) + 1);
    args = append(args, input.Executable)
    args = append(args, input.Arguments...)

    runtime.LockOSThread()
    defer runtime.UnlockOSThread()
    pid, err := vforkExec(
        input.Executable, args, 
        []string { fmt.Sprintf("LD_PRELOAD=%s", input.NeedleLib) },
        stdinR, stdoutW,
    )
    if err != nil {
        return RunnerOutput{
            Status: ST_SYSTEM_ERROR,
            Stdout: "",
            Deduction: 0,
            ExitInfo: 0,
        }
    }
    var status unix.WaitStatus
    var usages Usages
    skipUsages := false
    rusage := unix.Rusage{}
    deduction := uint32(0)
    startTime := time.Now()
    unix.Wait4(pid, nil, unix.WUNTRACED, nil)
    unix.PtraceSetOptions(pid, unix.PTRACE_O_TRACESECCOMP | unix.PTRACE_O_TRACEEXIT)
    updateUsages := func() (uint64, uint64) {
        stack, heap, err := getMemoryUsages(pid)
        if err == nil {
            usages = Usages{
                Time: uint64(time.Since(startTime).Microseconds()),
                Memory: stack + heap,
            }
        }
        return stack, heap
    }
    for {
        unix.PtraceCont(pid, 0);
        if input.Limits.IsAllUnlimited() {
            unix.Wait4(pid, &status, unix.WUNTRACED, &rusage)
        } else {
            for {
                time.Sleep(time.Millisecond * 10) // wait every 0.01s
                wpid, _ := unix.Wait4(pid, &status, unix.WUNTRACED | unix.WNOHANG, &rusage)
                if !skipUsages {
                    stack, heap := updateUsages()
                    if input.Limits.Time > 0 && usages.Time > input.Limits.Time {
                        unix.Kill(pid, unix.SIGKILL)
                        return RunnerOutput{
                            Status: ST_TIME_LIMIT_EXCEEDED,
                            Stdout: "",
                            Usages: usages,
                            Deduction: 0,
                            ExitInfo: 0,
                        }
                    } else if (
                        input.Limits.StackMemory > 0 && stack > input.Limits.StackMemory || 
                        input.Limits.HeapMemory > 0 && heap > input.Limits.HeapMemory) {
                        unix.Kill(pid, unix.SIGKILL)
                        return RunnerOutput{
                            Status: ST_MEMORY_LIMIT_EXCEEDED,
                            Stdout: "",
                            Usages: usages,
                            Deduction: 0,
                            ExitInfo: 0,
                        }
                    }
                }
                if wpid > 0 { break }
            }
        }
        if int(status >> 8) == int(unix.SIGTRAP | (unix.PTRACE_EVENT_SECCOMP << 8)) {
            updateUsages()
            info, _ := ptraceGetSyscallInfo(pid)
            if info.Seccomp.RetData == 0 {
                unix.Kill(pid, unix.SIGKILL)
                return RunnerOutput{
                    Status: ST_HOSTILE_CODE,
                    Stdout: "",
                    Usages: usages,
                    Deduction: 0,
                    ExitInfo: int(info.Seccomp.Nr),
                }
            } else {
                deduction += info.Seccomp.RetData
            }
        } else if int(status >> 8) == int(unix.SIGTRAP | (unix.PTRACE_EVENT_EXIT << 8)) {
            // last snapshot of the child
            updateUsages()
            skipUsages = true
        } else if status.Exited() {
            stdoutW.Close()
            stdout, err := io.ReadAll(stdoutR)
            if err != nil {
                return RunnerOutput{
                    Status: ST_SYSTEM_ERROR,
                    Stdout: "",
                    Deduction: 0,
                    ExitInfo: 0,
                }
            }
            return RunnerOutput{
                Status: ST_ACCEPTED,
                Stdout: string(stdout),
                Usages: usages,
                Deduction: int(deduction),
                ExitInfo: status.ExitStatus(),
            }
        } else if status.Signaled() {
            signal := status.Signal()
            status := ST_RUNTIME_ERROR
            return RunnerOutput{
                Status: status,
                Stdout: "",
                Usages: usages,
                Deduction: 0,
                ExitInfo: int(signal),
            }
        }
    }
}