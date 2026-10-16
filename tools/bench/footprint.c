// macOS's per-process physical footprint, including charged graphics memory.
// Build: clang -O2 tools/bench/footprint.c -o /tmp/rex-footprint
#include <libproc.h>
#include <mach/mach_time.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/resource.h>

int main(int argc, char **argv) {
    mach_timebase_info_data_t timebase;
    mach_timebase_info(&timebase);
    printf("[");
    for (int i = 1; i < argc; i++) {
        struct rusage_info_v4 info = {0};
        int pid = atoi(argv[i]);
        if (proc_pid_rusage(pid, RUSAGE_INFO_V4, (rusage_info_t *)&info)) return 1;
        printf("%s{\"pid\":%d,\"footprint\":%llu,\"peak\":%llu,\"rss\":%llu,\"cpu_ns\":%llu}",
            i > 1 ? "," : "", pid, info.ri_phys_footprint,
            info.ri_lifetime_max_phys_footprint, info.ri_resident_size,
            (unsigned long long)((__uint128_t)(info.ri_user_time + info.ri_system_time)
                * timebase.numer / timebase.denom));
    }
    printf("]\n");
    return 0;
}
