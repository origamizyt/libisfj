package isfj

import (
	"bytes"
	_ "embed"
	"os/exec"
	"text/template"

	"github.com/google/shlex"
)

//go:embed needle.c.tpl
var needleTemplate string

// Blacklist or whitelist?
type RuleMode bool

const (
    // Blacklist.
    // Unruled syscalls will be allowed.
    RM_BLACKLIST RuleMode = false
    // Whitelist.
    // Unruled syscalls will cause the program to terminate.
    RM_WHITELIST RuleMode = true
)

// What to do when encountering syscalls.
// If Deduction == 0, program will be directly killed.
type SyscallAction struct {
    // Syscall to filter.
    Syscall		int
    // Deduction of points.
    Deduction	int
}

// Rules to apply to seccomp.
type SyscallRules struct {
    // Mode of filtering.
    Mode	RuleMode
    // Action when encountering syscalls.
    Actions	[]SyscallAction
}

type compileNeedleTemplateData struct {
    Output	string
}

// Compiles a .so, which will be injected into the program.
// Code will be fed via stdin.
//
// Example command:
// gcc -o {{ .Output }} -fPIC -shared -x c -
func CompileNeedleLibrary(rules SyscallRules, command, output string) error {
    templ, _ := template.New("").Parse(needleTemplate)
    buf := bytes.Buffer{}
    templ.Execute(&buf, rules)
    code := buf.String()
    buf = bytes.Buffer{}
    templ, _ = template.New("").Parse(command)
    err := templ.Execute(&buf, compileNeedleTemplateData{ Output: output })
    if err != nil {
        return err
    }
    args, _ := shlex.Split(buf.String())
    cmd := exec.Command(args[0], args[1:]...)
    pipe, _ := cmd.StdinPipe()
    cmd.Start()
    pipe.Write([]byte(code))
    pipe.Close()
    return cmd.Wait()
}