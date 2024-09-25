package isfj

import (
	"bytes"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"text/template"

	"github.com/google/shlex"
	lua "github.com/yuin/gopher-lua"
)

func randName(prefix string) string {
    const candidates = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    s := make([]byte, 0, 8)
    for i := 0; i < 8; i++ {
        s = append(s, candidates[rand.Intn(len(candidates))])
    }
    return prefix + string(s)
}

func splitLinesAndTrim(s string) []string {
    lines := strings.Split(s, "\n")
    for i := 0; i < len(lines); i++ {
        lines[i] = strings.TrimSpace(lines[i])
    }
    lines = slices.DeleteFunc(lines, func (line string) bool { return len(line) == 0 })
    return lines
}

// Judger for [J_LAX].
// Compares only non-empty trimmed lines.
func LaxJudge(got, expected string) bool {
    gotLines := splitLinesAndTrim(got)
    expectedLines := splitLinesAndTrim(expected)
    n := len(gotLines)
    if n != len(expectedLines) {
        return false
    }
    for i := 0; i < n; i++ {
        if gotLines[i] != expectedLines[i] {
            return false
        }
    }
    return true
}

// Judger for [J_STRICT].
// Two strings must be exactly the same.
func StrictJudge(got, expected string) bool {
    return got == expected
}

// Judger for [J_SPECIAL].
type SpecialJudger interface {
    // Compares two strings, with an additional temporary folder.
    Judge(got, expected, tempDir string) Status
    // Clones this judger to avoid concurrency issues.
    Clone() (SpecialJudger, error)
    // Dispose of this judger.
    Dispose()
}

// An implementation of [SpecialJudger] which
// calls an external program to compare.
type ExternalJudger struct {
    command	*template.Template
}

type judgerTemplateData struct {
    Got string
    Expected string
}

// Creates a new [ExternalJudger] with given command template.
//
// Example:
// python3 compare.py "{{ .Got }}" "{{ .Expected }}"
func NewExternalJudger(templ string) (*ExternalJudger, error) {
    command, err := template.New("").Parse(templ)
    if err != nil {
        return nil, err
    }
    return &ExternalJudger{
        command: command,
    }, nil
}

// Implements [SpecialJudger].
func (s *ExternalJudger) Judge(got, expected, tempDir string) Status {
    gotFile := path.Join(tempDir, randName("spj_got_"))
    err := os.WriteFile(gotFile, []byte(got), 0o666)
    if err != nil {
        return ST_SYSTEM_ERROR
    }
    expectedFile := path.Join(tempDir, randName("spj_exp_"))
    err = os.WriteFile(expectedFile, []byte(expected), 0o666)
    if err != nil {
        return ST_SYSTEM_ERROR
    }
    buf := bytes.Buffer{}
    err = s.command.Execute(&buf, judgerTemplateData{
        Got: gotFile,
        Expected: expectedFile,
    })
    if err != nil {
        return ST_SYSTEM_ERROR
    }
    args, _ := shlex.Split(buf.String())
    cmd := exec.Command(args[0], args[1:]...)
    cmd.Run()
    if cmd.ProcessState.ExitCode() == 0 {
        return ST_ACCEPTED
    } else {
        return ST_WRONG_ANSWER
    }
}

// Implements [SpecialJudger].
func (s *ExternalJudger) Clone() (SpecialJudger, error) {
    commandClone, err := s.command.Clone()
    return &ExternalJudger{
        command: commandClone,
    }, err
}

// Implements [SpecialJudger].
func (s *ExternalJudger) Dispose() {}

// An implementation of [SpecialJudger] which
// uses a embedded Lua engine to execute scripts.
//
// The code must define a function named "judge",
// which takes two strings and returns a status.
// Status names are predefined in the global table.
type LuaJudger struct {
    Code    string
    state   *lua.LState
}

// Creates a [LuaJudger] using given script.
func NewLuaJudger(code string) (*LuaJudger, error) {
    j := &LuaJudger{
        Code: code,
        state: lua.NewState(lua.Options{ SkipOpenLibs: true }),
    }
    for _, pair := range []struct {
        n string
        f lua.LGFunction
    }{
        {lua.LoadLibName, lua.OpenPackage}, // Must be first
        {lua.BaseLibName, lua.OpenBase},
        {lua.TabLibName, lua.OpenTable},
    } {
        if err := j.state.CallByParam(lua.P{
            Fn:      j.state.NewFunction(pair.f),
            NRet:    0,
            Protect: true,
        }, lua.LString(pair.n)); err != nil {
            return nil, err
        }
    }
    for i := Status(0); i <= ST_MAX; i++ {
        j.state.SetGlobal(i.Ident(), lua.LNumber(i))
    }
    return j, nil
}

// Implements [SpecialJudger].
func (l *LuaJudger) Judge(got, expected, tempDir string) Status {
    l.state.SetGlobal("tempdir", lua.LString(tempDir))
    err := l.state.DoString(l.Code)
    if err != nil {
        return ST_SYSTEM_ERROR
    }
    judgeFunc, ok := l.state.GetGlobal("judge").(*lua.LFunction)
    if !ok {
        return ST_SYSTEM_ERROR
    }
    if err := l.state.CallByParam(lua.P{
        Fn:      judgeFunc,
        NRet:    1,
        Protect: true,
    }, lua.LString(got), lua.LString(expected)); err != nil {
        return ST_SYSTEM_ERROR
    }
    code := lua.LVAsNumber(l.state.Get(-1))
    l.state.Pop(-1)
    return Status(code)
}

// Implements [SpecialJudger].
func (l *LuaJudger) Clone() (SpecialJudger, error) {
    return NewLuaJudger(l.Code)
}

// Implements [SpecialJudger].
func (l *LuaJudger) Dispose() {
    l.state.Close()
}