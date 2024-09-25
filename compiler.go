package isfj

import (
    "bytes"
    "os"
    "os/exec"
    "path"
    "text/template"

    "github.com/google/shlex"
)

/*
A compiler that invokes external programs.
Commands are represented by a text/template template.

Example:
gcc -o "{{ .Output }}" -x c "{{ .Source }}"
*/
type Compiler struct {
    command	*template.Template
}

type compilerTemplateData struct {
    Source	string
    Output	string
}

// Creates a new compiler with given command template.
// This function will fail only if the template is invalid.
func NewCompiler(templ string) (*Compiler, error) {
    command, err := template.New("").Parse(templ)
    if err != nil {
        return nil, err
    }
    return &Compiler{
        command: command,
    }, nil
}

// Compiles given code with this compiler in given temporary folder.
// If compilation succeeds, will return ([ST_COMPILATION_SUCCESS], executable path).
// Otherwise, return (status, compiler stdout & stderr).
func (c *Compiler) Compile(code string, tempDir string) (Status, string) {
    sourceName := path.Join(tempDir, randName("src_"))
    err := os.WriteFile(sourceName, []byte(code), 0o666)
    if err != nil {
        return ST_SYSTEM_ERROR, ""
    }
    outputName := path.Join(tempDir, randName("exe_"))
    buf := bytes.Buffer{}
    err = c.command.Execute(&buf, compilerTemplateData{
        Source: sourceName,
        Output: outputName,
    })
    if err != nil {
        return ST_SYSTEM_ERROR, ""
    }
    args, _ := shlex.Split(buf.String())
    cmd := exec.Command(args[0], args[1:]...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        if _, ok := err.(*exec.ExitError); ok {
            return ST_COMPILATION_ERROR, string(output)
        }
        return ST_SYSTEM_ERROR, ""
    }
    return ST_COMPILATION_SUCCESS, outputName
}