<div align="center">
    <h1>libisfj: Injected Sandbox For Judgers</h1>
    <p>❗Only available on syscall-based (Unix) systems❗</p>
</div>

`libisfj` was originally designed to be just a sandbox for general purposes. However, for unexplainable reasons I added some layers on the top and made it a embeddable judging system. The API is goroutine-safe and can be used by modern judgers as a low-level implementation.

## API

### Compiling Needle

In order to prevent malicious system calls, we need the child to enable `seccomp` mode (Secure Computing), which will notify the parent when any evil attempt was made. Therefore we need a shared object to be injected into the process space, which we called the "needle".

To compile a "needle", you need to know which system calls to block, and have a compiler in your machine. Here's an example using `gcc`:
```go
const SYS_clone3 = 435

func CompileNeedle() {
    rules := isfj.SyscallRules{
        Mode: isfj.R_BLACKLIST,
        Actions: []isfj.SyscallAction{
            {
                Syscall: SYS_clone3,
                Deduction: 0
            },
        },
    }
    isfj.CompileNeedleLibrary(
        rules, 
        "gcc -o {{ .Output }} -fPIC -shared -x c -",
        "path/to/needle.so"
    )
}
```

If your policy doesn't change, you will only need to compile this once. Its all up to you to compile for every problem or use precompiled ones.

### Judging

To start judging, first thing you need is an `Engine`, which manages all resources used:
```go
import (
    "os"
    "path"
)

engine := isfj.NewEngine(path.Join(os.TempDir(), "isfj"))
```

Or you can use your own temporary folder.

Associate compilers with languages:
```go
// if your template cannot be syntactically incorrect,
// you can ignore the error
gcc, _ := isfj.NewCompiler(
    `gcc -o "{{ .Output }}" -x c "{{ .Source }}"`,
)
engine.AddCompiler("c", gcc)
```

Spawn workers to handle tasks:
```go
err := engine.SpawnWorkers(4)
// if you don't use special judgers
// you can ignore the error
if err != nil { ... }
```

And off we go.
```go
import "slices"

code := `
#include <stdio.h>
int main() {
    int a, b;
    scanf("%d%d", &a, &b);
    printf("%d", a+b);
    return 0;
}
`

// pretend we have 10 cases, 10 points each
cases := slices.Repeat([]isfj.Case{{
    Stdin: "1 2",
    Stdout: "3",
    // command line args
    Args: []string{},
    // empty means no limits
    Limits: isfj.Limits{},
    Points: 10,
}})

init := isfj.JobInit{
    Code: code,
    Lang: "c",
    // absolute path please!
    Needle: "/path/to/needle.so",
    // lax mode, accepts empty lines & extra whitespace
    Mode: isfj.J_LAX,
    Cases: cases,
}

job := isfj.NewJob(init)
task := engine.Schedule(job)

fmt.Println("task started:", task.Id())
```

And the task will start asynchronously without blocking the main goroutine. The engine doesn't store the task itself, only its id. You should store the task elsewhere, e.g. in a database, so you could query its status.

To check the job's status, e.g. in a web interface, use `SnapJob` on the task:
```go
job := task.SnapJob()

SendSomeJson(map[string]any {
    "status": job.Status,
})
```

`SnapJob` will return the snapshot of the job. Further updates on the job will not be reflected on the snapshot.

In order to do something when the task finishes executing, set a custom listener using `Subscribe`:
```go
task.Subscribe(func (job isfj.Job) {
    if job.Finished() {
        StoreJobInDatabase(job)
    }
})
```

The subscriber will be executed concurrently, so you don't need to worry about blocking the worker. It is guaranteed that if `job.Finished` returns `true`, the job will not be modified after.

To cancel a scheduled task, use `CancelTask`:
```go
engine.CancelTask(task)
```

The task pointer must be passed in, to ensure that only those owning the task can cancel it. Only tasks that are scheduled and is not executed or has been executed can be cancelled. Cancelling the task sets the job status and all case results to `ST_CANCELLED`.