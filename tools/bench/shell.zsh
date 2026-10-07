#!/bin/zsh -f
# Fake login shell: the first pane to start runs the workload, then becomes a normal zsh.
zmodload zsh/datetime
export SHELL=/bin/zsh
if [[ -n $BENCH_OUT ]] && mkdir "$BENCH_OUT/lock" 2>/dev/null; then
  print "shell $EPOCHREALTIME $COLUMNS $LINES" >> $BENCH_OUT/times
  sleep ${BENCH_SETTLE:-4}
  print "idle_start $EPOCHREALTIME" >> $BENCH_OUT/times
  sleep ${BENCH_IDLE:-10}
  print "idle_end $EPOCHREALTIME" >> $BENCH_OUT/times
  for w in ${=BENCH_WORKLOADS:-plain ansi cjk frames}; do
    t0=$EPOCHREALTIME; cat $BENCH_DATA/$w.txt; t1=$EPOCHREALTIME
    print "$w $t0 $t1" >> $BENCH_OUT/times
    sleep ${BENCH_GAP:-4}
  done
  clear
  print "done $EPOCHREALTIME" >> $BENCH_OUT/times
fi
exec /bin/zsh -l
