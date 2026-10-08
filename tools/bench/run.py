#!/usr/bin/env python3
"""Black-box benchmark of Rex Keel (keel/gio) vs GoRex (mygo)."""
import json, os, shutil, subprocess, sys, time, signal
B = os.path.dirname(os.path.abspath(__file__))
APPS = {
  'keel': dict(app=sys.argv[1] if len(sys.argv) > 1 else '', ),
}
def ps_all():
    out = subprocess.run(['ps', '-axo', 'pid=,ppid=,rss=,time=,command='], capture_output=True, text=True).stdout
    rows = []
    for line in out.splitlines():
        p = line.split(None, 4)
        if len(p) == 5:
            rows.append((int(p[0]), int(p[1]), int(p[2]), p[3], p[4]))
    return rows
def cpu_secs(t):
    # [[dd-]hh:]mm:ss.cc
    d = 0
    if '-' in t: dd, t = t.split('-'); d = int(dd) * 86400
    parts = [float(x) for x in t.split(':')]
    s = 0
    for x in parts: s = s * 60 + x
    return d + s
def find(exe):
    app = srv = None
    for pid, ppid, rss, t, cmd in ps_all():
        if cmd.startswith(exe):
            if ' -server' in cmd: srv = pid
            else: app = pid
    return app, srv
def sample(pids):
    r = {}
    for pid, ppid, rss, t, cmd in ps_all():
        if pid in pids: r[pid] = (rss / 1024, cpu_secs(t))
    return r
def footprint(pid):
    if not pid: return None
    out = subprocess.run(['footprint', '-p', str(pid)], capture_output=True, text=True).stdout
    r = {}
    for line in out.splitlines():
        line = line.strip()
        for key in ('phys_footprint:', 'phys_footprint_peak:'):
            if line.startswith(key):
                v, u = line[len(key):].split()[:2]
                r[key[:-1]] = round(float(v) * {'B': 1/2**20, 'KB': 1/1024, 'MB': 1, 'GB': 1024}.get(u, 1), 1)
        p = line.split()
        if len(p) >= 6 and p[-1] in ('Untagged',) or line.endswith('(graphics)') and 'Owned physical footprint (unmapped) (graphics)' in line:
            pass
    return r

def run_one(name, bundle, exe, run_id, env_dir_var, extra_args):
    st = os.path.join(B, 'state', f'{name}{run_id}'); shutil.rmtree(st, ignore_errors=True); os.makedirs(st)
    out = os.path.join(B, 'out', f'{name}{run_id}'); shutil.rmtree(out, ignore_errors=True); os.makedirs(out)
    if name == 'keel':
        json.dump({"Version":1,"Active":0,"Tabs":[{"Title":"bench","Focus":1,"Root":{"id":1,"Dir":os.path.join(B,'work')}}]}, open(os.path.join(st,'layout.json'),'w'))
        json.dump({"fontSize":12.5,"appearance":"dark","windowWidth":1000,"windowHeight":620}, open(os.path.join(st,'settings.json'),'w'))
    env = {env_dir_var: st, 'SHELL': os.path.join(B,'shell.zsh'), 'BENCH_OUT': out, 'BENCH_DATA': os.path.join(B,'data')}
    for k in ('BENCH_IDLE','BENCH_WORKLOADS','BENCH_GAP','BENCH_SETTLE'):
        if k in os.environ: env[k] = os.environ[k]
    cmd = ['open', '-n', '-a', bundle]
    for k, v in env.items(): cmd += ['--env', f'{k}={v}']
    cmd += ['--args'] + extra_args
    t0 = time.time()
    subprocess.run(cmd, check=True)
    app = None
    while app is None and time.time() - t0 < 10:
        app, _ = find(exe)
        if app is None: time.sleep(0.002)
    w = subprocess.run([os.path.join(B,'winwait'), str(app), '15'], capture_output=True, text=True).stdout.split()
    t_win = float(w[0]); win = (w[1], w[2])
    times = os.path.join(out, 'times')
    samples = []  # (t, app_rss, app_cpu, srv_rss, srv_cpu)
    srv = None; done = False; fp_idle = None; deadline = time.time() + 300
    while not done and time.time() < deadline:
        if srv is None: _, srv = find(exe)
        s = sample({app, srv})
        a = s.get(app, (0, 0)); v = s.get(srv, (0, 0))
        samples.append((time.time(), a[0], a[1], v[0], v[1]))
        txt = open(times).read() if os.path.exists(times) else ''
        if 'done' in txt: done = True
        if fp_idle is None and 'idle_start' in txt and time.time() > float(txt.split('idle_start ')[1].split()[0]) + 3:
            fp_idle = dict(app=footprint(app), srv=footprint(srv))
        time.sleep(0.1)
    fp_app, fp_srv = footprint(app), footprint(srv) if srv else None
    ev = {}
    for line in open(times):
        p = line.split()
        ev[p[0]] = [float(x) for x in p[1:3]] if p[0] not in ('shell',) else [float(p[1]), p[2], p[3]]
    def at(t):  # nearest sample at or after t
        for s in samples:
            if s[0] >= t: return s
        return samples[-1]
    def span(t_a, t_b):
        a, b = at(t_a), at(t_b)
        return dict(app_cpu=round(b[2]-a[2],2), srv_cpu=round(b[4]-a[4],2),
                    app_rss_max=round(max(s[1] for s in samples if t_a<=s[0]<=t_b+0.1) if any(t_a<=s[0]<=t_b+0.1 for s in samples) else b[1],1),
                    srv_rss_max=round(max((s[3] for s in samples if t_a<=s[0]<=t_b+0.1), default=b[3]),1))
    res = dict(app=name, run=run_id, window=win,
               t_window=round(t_win - t0, 3), t_shell=round(ev['shell'][0] - t0, 3), cols=ev['shell'][1], rows=ev['shell'][2],
               startup_cpu=dict(app_cpu=at(ev['shell'][0]+1)[2], srv_cpu=at(ev['shell'][0]+1)[4]), footprint_idle=fp_idle,
               idle=dict(secs=round(ev['idle_end'][1-1]-ev['idle_start'][0],1), **span(ev['idle_start'][0], ev['idle_end'][0])),
               footprint_end=dict(app=fp_app, srv=fp_srv), app_pid=app)
    ws = [k for k in ev if k not in ('shell','idle_start','idle_end','done')]
    for i, k in enumerate(ws):
        t_a, t_b = ev[k]
        nxt = ev[ws[i+1]][0] if i+1 < len(ws) else ev['done'][0]
        res[k] = dict(wall=round(t_b - t_a, 3), **{('tail_'+kk if False else kk): vv for kk, vv in span(t_a, nxt - 0.05).items()})
    json.dump(dict(res, samples=samples), open(os.path.join(out, 'result.json'), 'w'))
    # teardown
    for pid in (app, srv):
        if pid:
            try: os.kill(pid, signal.SIGKILL)
            except ProcessLookupError: pass
    subprocess.run(['pkill', '-KILL', '-f', 'BENCH_OUT'], capture_output=True)
    time.sleep(1.5)
    return res

def write_meta(targets):
    def sh(*c): return subprocess.run(c, capture_output=True, text=True).stdout.strip()
    def du(p): return sum(os.path.getsize(os.path.join(r, f)) for r, _, fs in os.walk(p) for f in fs)
    meta = dict(date=time.strftime('%Y-%m-%d %H:%M'), model=sh('sysctl', '-n', 'hw.model'),
                chip=sh('sysctl', '-n', 'machdep.cpu.brand_string'), mem_gb=int(sh('sysctl', '-n', 'hw.memsize')) // 2**30,
                macos=sh('sw_vers', '-productVersion'), go=sh('go', 'env', 'GOVERSION'),
                bundles={n: dict(path=t[0], bytes=du(t[0]), exe_bytes=os.path.getsize(t[1])) for n, t in targets.items()})
    json.dump(meta, open(os.path.join(B, 'meta.json'), 'w'), indent=1)


if __name__ == '__main__':
    K = os.environ['KEEL_APP']; G = os.environ['GOREX_APP']
    targets = {
      'keel': (K, os.path.join(K, 'Contents/MacOS/Rex Keel'), 'KEEL_REX_DIR', []),
      'mygo': (G, os.path.join(G, 'Contents/MacOS/GoRex'), 'GOREX_DIR', []),
    }
    runs = int(os.environ.get('RUNS', '1')); which = os.environ.get('ONLY', 'keel,mygo').split(',')
    allr = []
    for r in range(runs):
        for n in which:
            res = run_one(n, *targets[n][:2], r, *targets[n][2:])
            print(json.dumps(res), flush=True); allr.append(res)
    json.dump(allr, open(os.path.join(B, 'results.json'), 'w'), indent=1)
    write_meta(targets)
