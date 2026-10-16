#!/usr/bin/env python3
"""Compare native windows using macOS physical footprint, never RSS as memory.

Pass two .app bundles (or executables). Uses fresh state, equal window/pane
counts, byte-identical paced output in every pane, and only kills its own PIDs.
"""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import signal
import shlex
import statistics
import subprocess
import time

HERE = Path(__file__).resolve().parent


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def processes():
    rows = {}
    for line in command('ps', '-axo', 'pid=,ppid=,command=').splitlines():
        pid, parent, cmd = line.split(None, 2)
        rows[int(pid)] = (int(parent), cmd)
    return rows


def executable(path):
    path = Path(path).resolve()
    if path.suffix == '.app':
        import plistlib
        info = plistlib.loads((path / 'Contents/Info.plist').read_bytes())
        return path / 'Contents/MacOS' / info['CFBundleExecutable']
    return path


def tree(name, n, work, counter):
    if n == 1:
        counter[0] += 1
        if name == 'keel':
            return {'id': counter[0], 'Dir': str(work), 'Kind': 'terminal'}
        return {'dir': str(work), 'cols': 80, 'rows': 24}
    a, b = tree(name, n//2, work, counter), tree(name, n-n//2, work, counter)
    if name == 'keel':
        return {'Vertical': n == 2, 'Ratio': .5, 'First': a, 'Second': b}
    return {'vertical': n == 2, 'ratio': .5, 'a': a, 'b': b}


def write_state(name, state, panes, width, height, font):
    state.mkdir()
    settings = {'fontSize': font, 'appearance': 'dark', 'windowWidth': width, 'windowHeight': height}
    (state/'settings.json').write_text(json.dumps(settings))
    root = tree(name, panes, state, [0])
    if name == 'keel':
        layout = {'Version': 1, 'Active': 0, 'Tabs': [{'Title': 'bench', 'Focus': 1, 'Root': root}]}
    else:
        layout = {'active': 0, 'tabs': [{'name': 'bench', 'focus': 0, 'root': root}]}
    (state/'layout.json').write_text(json.dumps(layout))


def prepare_workload(out):
    # Include changing colors, bold text, CJK, emoji, and npm's Braille spinner.
    data = bytearray()
    for i in range(20000):
        data.extend((f'\x1b[38;5;{16+i%216}m{i:08d} INFO worker={i%32} '
                     f'\x1b[1mterminal output\x1b[0m 中文终端内存测试 ⠋ 🚀 value={i*7919:09d}\r\n').encode())
    (out/'output.bin').write_bytes(data)
    worker = out/'worker.py'
    worker.write_text('''import os,time,pathlib
p=pathlib.Path(os.environ['REX_BENCH_STATE']); pid=os.getpid()
ready=p/f'ready-{pid}'; ready.write_text('ready')
raw=pathlib.Path(os.environ['REX_BENCH_DATA']).read_bytes();data=raw*2
chunks=int(os.environ['REX_BENCH_CHUNKS'])
last=''
finished=False
while True:
 stage=(p/'stage').read_text() if (p/'stage').exists() else 'idle'
 if stage!=last:
  os.write(1, ('\\r\\nMEMBENCH_'+stage+'\\r\\n').encode());last=stage
  size=os.get_terminal_size(1)
  (p/f'geometry-{pid}').write_text(str((size.columns,size.lines)))
  (p/f'ack-{pid}').write_text(stage);finished=False
 if stage=='scroll' and not finished:
  # 32 KiB every 31.25 ms: 1 MiB/s per terminal, same bytes and timing.
  started=time.monotonic()
  for n in range(chunks):
   offset=(n*32768)%len(raw)
   pending=memoryview(data)[offset:offset+32768]
   while pending:pending=pending[os.write(1,pending):]
   time.sleep(max(0,started+(n+1)/32-time.monotonic()))
  (p/f'done-{pid}').write_text(str(chunks*32768));finished=True
 else:time.sleep(.05)
''')
    shell = out/'shell.zsh'
    shell.write_text('#!/bin/zsh -f\nexport SHELL=/bin/zsh\nexec '+shlex.quote(str(Path(os.sys.executable).resolve()))+' '+shlex.quote(str(worker))+'\n')
    shell.chmod(0o755)
    return shell, hashlib.sha256(data).hexdigest()


def wait_until(check, description, timeout=20):
    end = time.monotonic()+timeout
    while time.monotonic()<end:
        if check(): return
        time.sleep(.1)
    raise RuntimeError('timeout: '+description)


def run_one(name, binary, panes, run, args, helper, winwait, shell):
    suffix = f'-retry{args.attempt}' if args.attempt else ''
    state = args.out/f'{args.label}-{name}-{panes}p-{run}{suffix}'
    write_state(name, state, panes, args.width, args.height, args.font)
    env = {**os.environ, 'KEEL_REX_DIR' if name=='keel' else 'GOREX_DIR': str(state),
           'SHELL': str(shell), 'REX_BENCH_STATE': str(state), 'REX_BENCH_DATA': str(args.out/'output.bin'),
           'REX_BENCH_CHUNKS': str(math.ceil(args.active*32))}
    # Native windows only. Automation twins or headless rendering change memory.
    for k in ('KEEL_HEADLESS', 'KEEL_AUTOMATION', 'KEEL_REX_SERVER', 'GOREX_DEBUG'):
        env.pop(k, None)
    if name=='gorex' and (args.width,args.height)!=(1000,620):
        # GoRex's settings expose font size, but its window options are fixed.
        # Its existing debug hook can set the requested native window size.
        debug=state/'debug';debug.mkdir()
        env['GOREX_DEBUG']=str(debug)
        (debug/'do').write_text(f'size {args.width} {args.height}\n')
    log = (state/'app.log').open('w')
    app = subprocess.Popen([str(binary)], env=env, stdout=log, stderr=log)
    owned = {app.pid}
    result = {'name': name, 'panes': panes, 'run': run, 'binary': str(binary), 'stages': {}}
    try:
        geometry = command(str(winwait), str(app.pid), '15').split()
        if len(geometry)!=3: raise RuntimeError('no native window: '+str(geometry))
        # AppKit can briefly publish its initial frame before applying options.
        time.sleep(2)
        result['window'] = list(map(int, command(str(winwait), str(app.pid), '15').split()[1:]))
        if result['window'] != [args.width,args.height]:
            raise RuntimeError('window size mismatch: '+str(result['window']))
        wait_until(lambda: len(list(state.glob('ready-*')))==panes, 'terminal count')
        servers = [pid for pid, (_, cmd) in processes().items()
                   if cmd.startswith(str(binary)+' ') and ' -server' in cmd]
        if len(servers)!=1: raise RuntimeError('server count: '+str(servers))
        server = servers[0]; owned.add(server)
        result['pids'] = {'app': app.pid, 'server': server}
        bundle = binary.parents[2]
        if bundle.suffix == '.app':
            # Activate the already running test bundle. Occluded/inactive
            # windows may stop drawing; never compare them with an active one.
            subprocess.run(['open', '-a', str(bundle)], check=True)
        # Let fonts, layout, terminal resize and startup idle reclamation settle.
        time.sleep(4)
        for stage, duration in [('idle',args.idle),('scroll',args.active),('settled',args.settle)]:
            if bundle.suffix == '.app':
                subprocess.run(['open', '-a', str(bundle)], check=True)
            (state/'stage').write_text(stage)
            wait_until(lambda: len(list(state.glob('ack-*')))==panes and
                       all(p.read_text()==stage for p in state.glob('ack-*')), stage+' acknowledged')
            print(json.dumps({'event':'stage','name':name,'panes':panes,'run':run,'stage':stage,'state':str(state)}),flush=True)
            window_info=json.loads(command(str(winwait),str(app.pid),'info'))
            if window_info['frontmost_pid']!=app.pid or len(window_info['bounds'])!=1:
                raise RuntimeError('benchmark window is not foreground: '+str(window_info))
            samples = []
            end = time.monotonic()+duration
            while time.monotonic()<end:
                if len(samples)%5==0 and json.loads(command(str(winwait),str(app.pid),'info'))['frontmost_pid']!=app.pid:
                    raise RuntimeError('foreground changed during '+stage)
                measured = json.loads(command(str(helper),str(app.pid),str(server)))
                samples.append({'time':time.time(),'app':measured[0],'server':measured[1]})
                time.sleep(.2)
            if stage=='scroll':
                wait_until(lambda:len(list(state.glob('done-*')))==panes, 'identical output completed',timeout=60)
                result['bytes_per_terminal']=[int(p.read_text()) for p in state.glob('done-*')]
            if json.loads(command(str(winwait),str(app.pid),'info'))['frontmost_pid']!=app.pid:
                raise RuntimeError('foreground changed during '+stage)
            stats = {'samples': samples, 'window':window_info,
                     'sample_seconds':samples[-1]['time']-samples[0]['time'],
                     'terminal_grids':[p.read_text() for p in state.glob('geometry-*')]}
            # Settled samples exclude the first half while deferred GC/Metal trimming runs.
            tail = samples[len(samples)//2:] if stage!='scroll' else samples
            for role in ('app','server'):
                values = [s[role]['footprint']/2**20 for s in tail]
                cpu=(samples[-1][role]['cpu_ns']-samples[0][role]['cpu_ns'])/1e9
                stats[role] = {'median_mib':round(statistics.median(values),2),'max_mib':round(max(values),2),
                               'cpu_seconds':round(cpu,3),
                               'process_peak_mib':round(max(s[role]['peak'] for s in samples)/2**20,2)}
                vm = command('vmmap','-summary',str(result['pids'][role]))
                (state/f'{stage}-{role}-vmmap.txt').write_text(vm)
            totals = [(s['app']['footprint']+s['server']['footprint'])/2**20 for s in tail]
            stats['total'] = {'median_mib':round(statistics.median(totals),2),'max_mib':round(max(totals),2)}
            result['stages'][stage] = stats
            print(json.dumps({'event':'result','name':name,'panes':panes,'run':run,'stage':stage,
                              **{k:stats[k] for k in ('app','server','total')}}),flush=True)
        (state/'result.json').write_text(json.dumps(result,indent=2))
        return result
    finally:
        # Own process tree only; never match or stop an installed app's sessions.
        rows = processes()
        while True:
            expanded = owned | {pid for pid,(parent,_) in rows.items() if parent in owned}
            if expanded==owned:break
            owned=expanded
        for pid in owned:
            try:os.kill(pid,signal.SIGKILL)
            except ProcessLookupError:pass
        app.wait();log.close()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--keel',type=executable,required=True)
    p.add_argument('--gorex',type=executable,required=True)
    p.add_argument('--out',type=Path,required=True)
    p.add_argument('--label',default='baseline')
    p.add_argument('--only',default='keel,gorex')
    p.add_argument('--panes',default='1,4')
    p.add_argument('--runs',type=int,default=3)
    p.add_argument('--width',type=int,default=1000)
    p.add_argument('--height',type=int,default=620)
    p.add_argument('--font',type=float,default=12.5)
    p.add_argument('--idle',type=float,default=5)
    p.add_argument('--active',type=float,default=10)
    p.add_argument('--settle',type=float,default=8)
    args=p.parse_args();args.out=args.out.resolve();args.out.mkdir(parents=True,exist_ok=True)
    helper=args.out/'footprint';winwait=args.out/'winwait'
    subprocess.run(['clang','-O2',str(HERE/'footprint.c'),'-o',str(helper)],check=True)
    subprocess.run(['swiftc','-O',str(HERE/'winwait.swift'),'-o',str(winwait)],check=True)
    shell,digest=prepare_workload(args.out)
    meta={'metric':'proc_pid_rusage RUSAGE_INFO_V4 ri_phys_footprint','sha256':digest,
          'width':args.width,'height':args.height,'font_size':args.font,
          'chip':command('sysctl','-n','machdep.cpu.brand_string'),
          'macos':command('sw_vers','-productVersion'),
          'gogc_environment':os.environ.get('GOGC'),
          'binaries':{name:{'sha256':hashlib.sha256(getattr(args,name).read_bytes()).hexdigest(),
                            'build':command('go','version','-m',str(getattr(args,name)))} for name in args.only.split(',')},
          'results':[]}
    for run in range(args.runs):
        for panes in map(int,args.panes.split(',')):
            for name in args.only.split(','):
                for attempt in range(5):
                    args.attempt=attempt
                    try:
                        result=run_one(name,getattr(args,name),panes,run,args,helper,winwait,shell)
                    except RuntimeError as error:
                        if 'foreground' not in str(error) or attempt==4:raise
                        print(json.dumps({'event':'discard','reason':str(error),'name':name,'panes':panes,'run':run}),flush=True)
                        continue
                    meta['results'].append(result)
                    break
                (args.out/(args.label+'-results.json')).write_text(json.dumps(meta,indent=2))


if __name__=='__main__':main()
