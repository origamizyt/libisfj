package isfj

import (
    "fmt"
    "os"
    "path"
    "slices"
    "sync"
    "time"
)

// Engines are the manager of this library.
type Engine struct {
    // Base of all temporary folders.
    TempDirBase 	string
    judgers			[]SpecialJudger
    compilers		map[string]*Compiler
    counter			uint64
    queue			chan *Task
    stopFlag		chan any
    taskIds			[]uint64
    lock			sync.Mutex
}

// Creates a new engine using given temporary folder.
// Usually, you can specify [path.Join]([os.TempDir](), "program name").
func NewEngine(tempDirBase string) *Engine {
    return &Engine{
        TempDirBase: tempDirBase,
        counter: 0,
        compilers: map[string]*Compiler{},
        queue: make(chan *Task),
        stopFlag: make(chan any),
    }
}

// Associates given compiler with a language.
// A language can only have one compiler.
func (e *Engine) AddCompiler(name string, compiler *Compiler) {
    e.compilers[name] = compiler
}

// Associates given special judger with an unique id.
// Use [MakeSpecialJudgeMode] to make a special [JudgeMode] for the judger.
func (e *Engine) AddJudger(judger SpecialJudger) int {
    id := len(e.judgers)
    e.judgers = append(e.judgers, judger)
    return id
}

// Create a task associated to given job,
// and send the task to workers.
func (e *Engine) Schedule(job Job) *Task {
    e.lock.Lock()
    defer e.lock.Unlock()
    t := &Task{
        id: e.counter,
        lock: sync.Mutex{},
        job: job,
        tempDir: path.Join(e.TempDirBase, randName("job_")),
    }
    e.counter++
    e.taskIds = append(e.taskIds, t.id)
    e.queue <- t
    return t
}

// Check whether given task is running.
func (e *Engine) ContainsTask(id uint64) bool {
    e.lock.Lock()
    defer e.lock.Unlock()
    return slices.Contains(e.taskIds, id)
}

func (e *Engine) removeTask(id uint64) bool {
    e.lock.Lock()
    defer e.lock.Unlock()
    index := slices.Index(e.taskIds, id)
    if index >= 0 {
        e.taskIds = slices.Delete(e.taskIds, index, index+1)
    }
    return index >= 0
}

// Cancels given task.
// Passing a pointer ensures that only
// those owning the task can cancel it.
func (e *Engine) CancelTask(task *Task) {
    if e.removeTask(task.id) {
        task.cancel()
    }       
}

type worker struct {
    judgers	[]SpecialJudger
    engine 	*Engine
}

func (w *worker) runOne(task *Task, executable string, i int) {
    input := RunnerInput{
        Executable: executable,
        Arguments: task.job.Cases[i].Args,
        NeedleLib: task.job.Needle,
        Stdin: task.job.Cases[i].Stdin,
        Limits: task.job.Cases[i].Limits,
    }
    task.update(func() {
        task.job.Results[i+1].Status = ST_RUNNING
    })
    output := Run(input)
    task.update(func() {
        if output.Status != ST_ACCEPTED {
            task.job.Results[i+1].Status = output.Status
        }
        task.job.Results[i+1].Usages = output.Usages
        switch output.Status {
            case ST_RUNTIME_ERROR: {
                task.job.Results[i+1].Extra = fmt.Sprintf("Process terminated by signal %d", output.ExitInfo)
            }
            case ST_HOSTILE_CODE: {
                task.job.Results[i+1].Extra = 
                    fmt.Sprintf("Process killed due to malicious syscall %d", output.ExitInfo)
            }
        }
    })
    if output.Status == ST_ACCEPTED {
        var status Status
        switch task.job.Mode.ModeBits() {
            case J_LAX: {
                if LaxJudge(output.Stdout, task.job.Cases[i].Stdout) {
                    status = ST_ACCEPTED
                } else {
                    status = ST_WRONG_ANSWER
                }
            }
            case J_STRICT: {
                if StrictJudge(output.Stdout, task.job.Cases[i].Stdout) {
                    status = ST_ACCEPTED
                } else {
                    status = ST_WRONG_ANSWER
                }
            }
            case J_SPECIAL: {
                judger, err := w.judgers[task.job.Mode.JudgerId()].Clone()
                if err != nil {
                    status = ST_SYSTEM_ERROR
                } else {
                    status = judger.Judge(output.Stdout, task.job.Cases[i].Stdout, task.tempDir)
                }
            }
        }
        task.update(func() {
            task.job.Results[i+1].Status = status
            if status == ST_ACCEPTED {
                task.job.Results[i+1].Points = max(task.job.Cases[i].Points - output.Deduction, 0)
            }
        })
    }
}

func (w *worker) runUnpacked(task *Task, executable string) {
    wg := sync.WaitGroup{}
    wg.Add(len(task.job.Cases))
    for i := 0; i < len(task.job.Cases); i++ {
        go func(){
            defer wg.Done()
            w.runOne(task, executable, i)
        }()
    }
    wg.Wait()
}

func (w *worker) runPacked(task *Task, executable string) {
    wg := sync.WaitGroup{}
    wg.Add(len(task.job.Groups))
    for _, group := range task.job.Groups {
        go func(){
            defer wg.Done()
            for _, i := range group {
                w.runOne(task, executable, i-1)
            }
        }()
    }
    wg.Wait()
}

func (w *worker) run(task *Task) {
    task.update(func() {
        task.job.Status = ST_RUNNING
    })
    os.MkdirAll(task.tempDir, 0o777)
    defer os.RemoveAll(task.tempDir)
    compiler := w.engine.compilers[task.job.Lang]
    status, output := compiler.Compile(task.job.Code, task.tempDir)
    task.update(func() {
        task.job.Results[0].Status = status
    })
    if status != ST_COMPILATION_SUCCESS {
        task.update(func() {
            task.job.Results[0].Extra = output
            task.job.Status = status
            for i := 0; i < len(task.job.Results); i++ {
                task.job.Results[i].Status = ST_SKIPPED
            }
        })
        return
    }
    if task.job.Groups != nil {
        w.runPacked(task, output)	
    } else {
        w.runUnpacked(task, output)
    }
    task.update(func() {
        broke := false
        for _, result := range task.job.Results[1:] {
            if result.Status != ST_ACCEPTED {
                task.job.Status = result.Status
                broke = true
                break
            }
        }
        if !broke {
            task.job.Status = ST_ACCEPTED
        }
    })
}

func (w *worker) poll() {
    for {
        select {
            case task := <-w.engine.queue: {
                if w.engine.ContainsTask(task.id) {
                    w.engine.removeTask(task.id)
                    w.run(task)
                }
            }
            case <-w.engine.stopFlag: {
                return
            }
        }
    }
}

func (e *Engine) newWorker() (*worker, error) {
    judgers := make([]SpecialJudger, 0, len(e.judgers))
    for _, judger := range e.judgers {
        j, err := judger.Clone()
        if err != nil {
            return nil, err
        }
        judgers = append(judgers, j)
    }
    return &worker{
        judgers: judgers,
        engine: e,
    }, nil
}

// Spawns specific amount of workers.
// Workers will start consuming jobs immediately.
func (e *Engine) SpawnWorkers(n int) error {
    for i := 0; i < n; i++ {
        w, err := e.newWorker()
        if err != nil {
            return err
        }
        go w.poll()
    }
    return nil
}

// Shuts down all workers, after they have finished their current jobs.
func (e *Engine) Shutdown() {
    close(e.stopFlag)
    for _, judger := range e.judgers {
        judger.Dispose()
    }
    e.judgers = nil
    e.stopFlag = make(chan any)
}

// A task wraps around a [Job] that is assigned to a worker.
type Task struct {
    id			uint64
    lock		sync.Mutex
    job			Job
    tempDir		string
    listener	func(Job)
}

// Id of the task, usually incremented in each task.
func (t *Task) Id() uint64 {
    return t.id
}

// A snapshot of the current job.
func (t *Task) SnapJob() Job {
    t.lock.Lock()
    defer t.lock.Unlock()
    return t.job
}

func (t *Task) update(f func()) {
    t.lock.Lock()
    defer t.lock.Unlock()
    f()
    t.job.Updated = time.Now()
    if t.listener != nil {
        go t.listener(t.job)
    }
}

func (t *Task) cancel() {
    t.update(func() {
        t.job.Status = ST_CANCELLED
        for i := 0; i < len(t.job.Results); i++ {
            t.job.Results[i].Status = ST_CANCELLED
        }
    })
}

// Adds a listeners that is called
// every time the job updates.
func (t *Task) Subscribe(listener func(Job)) {
    t.listener = listener
}