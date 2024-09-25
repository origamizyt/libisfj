package isfj

import (
	"time"
)

// Job & case status.
type Status uint16;

// Identifier of the status.
func (s Status) Ident() string {
    switch s {
        case ST_WAITING: 
            return "ST_WAITING"
        case ST_RUNNING:
            return "ST_RUNNING"
        case ST_CANCELLED:
            return "ST_CANCELLED"
        case ST_ACCEPTED:
            return "ST_ACCEPTED"
        case ST_COMPILATION_ERROR:
            return "ST_COMPILATION_ERROR"
        case ST_COMPILATION_SUCCESS:
            return "ST_COMPILATION_SUCCESS"
        case ST_WRONG_ANSWER:
            return "ST_WRONG_ANSWER"
        case ST_RUNTIME_ERROR:
            return "ST_RUNTIME_ERROR"
        case ST_HOSTILE_CODE:
            return "ST_HOSTILE_CODE"
        case ST_TIME_LIMIT_EXCEEDED:
            return "ST_TIME_LIMIT_EXCEEDED"
        case ST_MEMORY_LIMIT_EXCEEDED:
            return "ST_MEMORY_LIMIT_EXCEEDED"
        case ST_SYSTEM_ERROR:
            return "ST_SYSTEM_ERROR"
        case ST_SKIPPED:
            return "ST_SKIPPED"
    }
    panic("All branches already covered.")
}

// Human-readable string representation.
func (s Status) String() string {
    switch s {
        case ST_WAITING: 
            return "Waiting"
        case ST_RUNNING:
            return "Running"
        case ST_CANCELLED:
            return "Cancelled"
        case ST_ACCEPTED:
            return "Accepted"
        case ST_COMPILATION_ERROR:
            return "Compilation Error"
        case ST_COMPILATION_SUCCESS:
            return "Compilation Success"
        case ST_WRONG_ANSWER:
            return "Wrong Answer"
        case ST_RUNTIME_ERROR:
            return "Runtime Error"
        case ST_HOSTILE_CODE:
            return "Hostile Code"
        case ST_TIME_LIMIT_EXCEEDED:
            return "Time Limit Exceeded"
        case ST_MEMORY_LIMIT_EXCEEDED:
            return "Memory Limit Exceeded"
        case ST_SYSTEM_ERROR:
            return "System Error"
        case ST_SKIPPED:
            return "Skipped"
    }
    panic("All branches already covered.")
}

const (
    // Job / case is waiting to be executed.
    ST_WAITING Status = iota
    // Job / case is being executed.
    ST_RUNNING
    // Job / case is cancelled.
    ST_CANCELLED
    // Successfully executed & output matches.
    ST_ACCEPTED
    // Case 0 failed to compile.
    ST_COMPILATION_ERROR
    // Case 0 compiled successfully.
    ST_COMPILATION_SUCCESS
    // Case 1~n has wrong output.
    ST_WRONG_ANSWER
    // Case 1~n was terminated by a signal.
    ST_RUNTIME_ERROR
    // Case 1~n invoked a dangerous system call.
    ST_HOSTILE_CODE
    // Case 1~n exceeded their time limit.
    ST_TIME_LIMIT_EXCEEDED
    // Case 1~n exceeded their memory limit.
    ST_MEMORY_LIMIT_EXCEEDED
    // Something went wrong with the judging system.
    ST_SYSTEM_ERROR
    // Case was skipped in packed judging.
    ST_SKIPPED
)

const (
    ST_MAX = ST_SKIPPED
)

// Judging mode.
type JudgeMode uint16;

// Mode bits of this mode.
func (m JudgeMode) ModeBits() JudgeMode {
    return m & 0x00ff
}

// Judger id of this mode.
// Available only when ModeBits() == [J_SPECIAL].
func (m JudgeMode) JudgerId() int {
    return int((m & 0xff00) >> 8)
}

const (
    // Lax judging.
    J_LAX JudgeMode = iota
    // Strict judging.
    J_STRICT
    // Special judging. Has to be combined with a judger id.
    J_SPECIAL
)

// Combines judger id with [J_SPECIAL].
func MakeSpecialJudgeMode(judger int) JudgeMode {
    return JudgeMode(judger << 8) + J_SPECIAL
}

// Case type.
// Usage should be superficial.
type Case struct {
    Stdin   string
    Stdout  string
    Args    []string
    Limits  Limits
    Points  int
}

// Result of judging a case.
type CaseResult struct {
    Status  Status
    Usages  Usages
    Points  int
    Extra   string
}

// Arguments passed to [NewJob].
type JobInit struct {
    Code    string
    Lang    string
    Needle  string
    Mode    JudgeMode
    Cases   []Case
    Groups  [][]int
}

// A job contains a collection of cases
// to be judged against.
type Job struct {
    Code    string
    Lang    string
    Needle  string
    Status  Status
    Mode    JudgeMode
    Cases   []Case
    Groups  [][]int
    Results []CaseResult
    Updated time.Time
}

// Creates a new job using given arguments.
func NewJob(init JobInit) Job {
    return Job{
        Code: init.Code,
        Lang: init.Lang,
        Needle: init.Needle,
        Status: ST_WAITING,
        Mode: init.Mode,
        Cases: init.Cases,
        Groups: init.Groups,
        Results: make([]CaseResult, len(init.Cases)+1),
        Updated: time.Now(),
    }
}

// Checks whether this job is neither waiting nor running.
func (j Job) Finished() bool {
    return j.Status != ST_WAITING && j.Status != ST_RUNNING
}