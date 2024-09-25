#include <sys/prctl.h>
#include <seccomp.h>

__attribute__((constructor))
void __(void) {
    scmp_filter_ctx ctx;

    {{ if .Mode }}
    ctx = seccomp_init(SCMP_ACT_TRACE(0));

    {{ range .Actions }}
    seccomp_rule_add(ctx, SCMP_ACT_ALLOW, {{ .Syscall }}, 0);
    {{ end }}

    {{ else }}
    ctx = seccomp_init(SCMP_ACT_ALLOW);

    {{ range .Actions }}
    seccomp_rule_add(ctx, SCMP_ACT_TRACE({{ .Deduction }}), {{ .Syscall }}, 0);
    //seccomp_rule_add(ctx, SCMP_ACT_KILL_PROCESS, {{ .Syscall }}, 0);
    {{ end }}

    {{ end }}
    
    seccomp_load(ctx);
    prctl(PR_SET_SECCOMP, 1);
}