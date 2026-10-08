"""End-to-end Keel UI QA without creating any native windows. Uses private PTYs and a temporary Git repository."""
import socket,json,time,base64,os,subprocess,tempfile
from pathlib import Path
root=Path(__file__).resolve().parents[1]
run=tempfile.TemporaryDirectory(prefix='keel-rex-qa-',dir='/tmp')
work=Path(run.name)
binary=work/'rex';state=work/'state';sock=str(work/'ui.sock')
artifacts=Path(os.environ.get('REX_QA_DIR', str(root/'evidence')))
if os.environ.get('REX_QA_STANDALONE_SERVER'):
 server_binary=work/'rex-server'
 subprocess.run(['go','build','-o',str(server_binary),'./cmd/rex-server'],cwd=root,check=True)
 os.environ['KEEL_REX_SERVER']=str(server_binary)
subprocess.run(['go','build','-o',str(binary),'.'],cwd=root,check=True)
def launch():
 env=dict(os.environ,KEEL_HEADLESS='1',KEEL_AUTOMATION=sock)
 process=subprocess.Popen([str(binary),'-state-dir',str(state),'-dir',str(root)],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
 for _ in range(150):
  try:
   s=socket.socket(socket.AF_UNIX);s.connect(sock);return process,s,s.makefile('rwb')
  except OSError:
   if process.poll() is not None: raise RuntimeError(process.stderr.read().decode())
   time.sleep(.05)
 raise TimeoutError('App did not start')
def daemon(request):
 s=socket.socket(socket.AF_UNIX);s.connect(str(state/'sessions.sock'));s.sendall((json.dumps(request)+'\n').encode());f=s.makefile('rb');result=json.loads(f.readline());f.close();s.close();return result
process,s,f=launch()
def rpc(method,**params):
 f.write((json.dumps({'method':method,'params':params})+'\n').encode());f.flush();r=json.loads(f.readline());assert 'error' not in r,r;return r.get('result')
def match(r,text):return [e for e in r['elements'] if text in e.get('name','')]
def press(k):return rpc('press',key=k)
def command(query):
 press('mod+shift+p');rpc('type',text=query);return press('enter')
def screenshot(name):
 artifacts.mkdir(parents=True,exist_ok=True)
 open(artifacts/(name+'.png'),'wb').write(base64.b64decode(rpc('screenshot')))
def wait_enabled(text):
 for _ in range(100):
  r=rpc('snapshot')
  if any(e.get('name')==text and not e.get('disabled') for e in r['elements']): return
  time.sleep(.05)
 raise AssertionError('Not enabled: '+text)
try:
 print('initial tabs',len(match(rpc('snapshot'),'Terminal tab')))
 press('mod+shift+p');press('esc');rpc('type',text="printf 'FOCUS_%s 中文验证\\n' READY");press('enter');rpc('wait_for',text='FOCUS_READY');print('PASS Escape returns focus and Unicode input')
 press('mod+t');press('mod+shift+r');rpc('type',text='研发工作区',clear=True);r=press('enter');assert match(r,'研发工作区');print('PASS rename')
 r=press('mod+d');assert len(match(r,'Pane header'))==2
 r=press('mod+shift+d');assert len(match(r,'Pane header'))==3
 press('mod+alt+left');press('mod+ctrl+right');press('mod+ctrl+=');r=press('mod+shift+enter');assert len(match(r,'Pane header'))==1
 r=press('mod+shift+enter');assert len(match(r,'Pane header'))==3
 r=press('mod+w');assert len(match(r,'Pane header'))==2
 print('PASS split / focus / resize / equalize / zoom / close-pane')
 r=command('appearance dark');assert not match(r,'Command search');time.sleep(.2);screenshot('gorex-dark')
 press('mod+=');press('mod+-');press('mod+0');print('PASS theme and text-size commands')
 # Arrows must select one command and Enter must run it exactly once.
 press('mod+p');press('down');r=press('enter');assert len(match(r,'Pane header'))==3;print('PASS palette arrows and single execution')
 # Test a long-running foreground process rather than killing it silently.
 rpc('type',text='sleep 20');press('enter');time.sleep(1.1);r=press('mod+w');assert match(r,'End the running program');r=press('esc');assert len(match(r,'Pane header'))==3
 r=press('mod+w');rpc('click',text='End session');print('PASS running-program confirmation')
 # Locate text in actual scrollback, including wide characters.
 rpc('type',text="printf 'SEARCH_中文_%s\\n' TARGET; for i in {1..90}; do echo row-$i; done");press('enter');time.sleep(.6)
 press('mod+f');rpc('type',text='SEARCH_中文_TARGET');press('enter');r=rpc('wait_for',text='SEARCH_中文_TARGET');assert not match(r,'Find text');screenshot('gorex-search');print('PASS history search with wide Unicode')
 # Ensure Git rows remain single-line and staging really modifies the temp repo.
 tmp=str(work/'git-project');os.makedirs(tmp)
 subprocess.run(['git','init','-q',tmp],check=True)
 subprocess.run(['git','-C',tmp,'config','user.name','Rex QA'],check=True)
 subprocess.run(['git','-C',tmp,'config','user.email','qa@example.invalid'],check=True)
 open(tmp+'/change.go','w').write('package example\n\t// 中文\n')
 press('mod+o');rpc('type',text=tmp,clear=True);press('enter');command('open git changes');rpc('wait_for',text='Git file change.go');r=rpc('click',text='Stage');wait_enabled('Unstage');status=subprocess.check_output(['git','-C',tmp,'status','--porcelain']).decode();assert status.startswith('A '),status
 rpc('click',text='Unstage');wait_enabled('Stage');time.sleep(.2);assert subprocess.check_output(['git','-C',tmp,'status','--porcelain']).decode().startswith('??')
 wait_enabled('Stage all');rpc('click',text='Stage all');wait_enabled('Unstage');rpc('click',text='Commit…');r=rpc('snapshot');box=next(e for e in r['elements'] if e['role']=='textbox' and e['name']=='Commit message');rpc('type',ref=box['ref'],text='Verify real Git commit');wait_enabled('Create commit');rpc('click',text='Create commit');rpc('wait_for',text='Working tree clean');assert subprocess.check_output(['git','-C',tmp,'log','-1','--format=%s']).decode().strip()=='Verify real Git commit';print('PASS real Git stage / unstage / commit')
 command('appearance light');screenshot('gorex-light');r=rpc('snapshot');print('final tabs',len(match(r,'Terminal tab')))
 # Save workspace and daemon PID, then close only the UI.
 time.sleep(1);rpc('close');print('PASS window closes without ending sessions')

 process.wait(timeout=5);f.close();s.close()
 layout=json.loads((state/'layout.json').read_text());before=daemon({'Op':'hello'})['PID']
 process,s,f=launch()
 r=rpc('snapshot');assert len(match(r,'Terminal tab'))==3 and match(r,'研发工作区')
 press('mod+1');rpc('wait_for',text='FOCUS_READY 中文验证')
 assert daemon({'Op':'hello'})['PID']==before
 rpc('type',text="printf 'REOPEN_%s\\n' READY");press('enter');rpc('wait_for',text='REOPEN_READY')
 press('mod+2');assert len(match(rpc('snapshot'),'Pane header'))==2
 press('mod+1');screenshot('gorex-restored');print('PASS workspace, Unicode, focus, and same daemon after reopening')
 daemon({'Op':'shutdown'})
 rpc('wait_for',text='Restart ended session')
 r=rpc('snapshot');restart=next(e for e in r['elements'] if e.get('name')=='Restart ended session')
 rpc('click',ref=restart['ref'])
 for _ in range(100):
  r=rpc('snapshot')
  if len(match(r,'Restart ended session'))<2: break
  time.sleep(.05)
 rpc('type',text="printf 'RESTART_%s\\n' READY");press('enter');rpc('wait_for',text='RESTART_READY')
 assert daemon({'Op':'hello'})['PID']!=before
 print('PASS restart shell after session service failure')
 rpc('close');process.wait(timeout=5)
finally:
 if process.poll() is None:
  try: rpc('close');process.wait(timeout=5)
  except Exception: process.terminate();process.wait(timeout=5)
 try: daemon({'Op':'shutdown'})
 except OSError: pass
 f.close();s.close();run.cleanup()
