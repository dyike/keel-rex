#!/usr/bin/env python3
"""Builds report.html from run.py's results.log, out/*/result.json, meta.json
and memprobe.json (made with `go run ./tools/bench/memprobe` when missing).

    python3 report.py            # writes report.html next to this file
    python3 report.py --md       # also prints the summary table as Markdown
"""
import html, json, os, statistics as st, subprocess, sys, time

B = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(B))  # keel-rex module root
APPS = ('keel', 'mygo')
LABEL = {'keel': 'keel-rex', 'mygo': 'GoRex'}
SUB = {'keel': 'Keel / Gio', 'mygo': 'MyGo'}
WORK = [  # name, MB, description
    ('plain', 20, '20 MB 纯文本日志'),
    ('ansi', 10, '10 MB 256 色 / 真彩 SGR'),
    ('cjk', 5, '5 MB 中文 + emoji'),
    ('frames', 6.2, '500 帧备用屏全屏重绘'),
]

# ---------------------------------------------------------------- data

def load():
    rows = [json.loads(l) for l in open(os.path.join(B, 'results.log')) if l.startswith('{')]
    if not rows:
        sys.exit('results.log has no results: run run.py first')
    meta = json.load(open(os.path.join(B, 'meta.json'))) if os.path.exists(os.path.join(B, 'meta.json')) else {}
    probe_path = os.path.join(B, 'memprobe.json')
    if not os.path.exists(probe_path):
        print('running memprobe…', file=sys.stderr)
        out = subprocess.run(['go', 'run', './tools/bench/memprobe'], cwd=ROOT, capture_output=True, text=True)
        if out.returncode == 0:
            open(probe_path, 'w').write(out.stdout)
        else:
            print(out.stderr, file=sys.stderr)
    probe = json.load(open(probe_path)) if os.path.exists(probe_path) else None
    timelines = {}
    for a in APPS:
        p = os.path.join(B, 'out', f'{a}0', 'result.json')
        tp = os.path.join(B, 'out', f'{a}0', 'times')
        if os.path.exists(p) and os.path.exists(tp):
            timelines[a] = (json.load(open(p))['samples'], [l.split() for l in open(tp)])
    return rows, meta, probe, timelines


def stat(rows, app, f):
    v = []
    for r in rows:
        if r['app'] != app:
            continue
        try:
            x = f(r)
        except (KeyError, TypeError, ZeroDivisionError):
            x = None
        if x is not None:
            v.append(x)
    return (st.median(v), min(v), max(v)) if v else None


def fp(r, when, proc, key='phys_footprint'):
    d = r.get('footprint_' + when) or {}
    return (d.get(proc) or {}).get(key)

# ---------------------------------------------------------------- formatting

def esc(s):
    return html.escape(str(s))


def num(v, d=2):
    if v is None:
        return '–'
    if abs(v) >= 100:
        return f'{v:,.0f}'
    if abs(v) >= 10:
        return f'{v:.1f}'
    return f'{v:.{d}f}'


def ratio(k, g, lower_better=True):
    """'GoRex 快 17.7×' style text, from medians."""
    if not k or not g or k[0] == 0 or g[0] == 0:
        return ''
    r = k[0] / g[0] if lower_better else g[0] / k[0]
    if 0.9 < r < 1.1:
        return '持平'
    win = 'mygo' if r > 1 else 'keel'
    return f'{LABEL[win]} 领先 {max(r, 1 / r):.1f}×'

# ---------------------------------------------------------------- svg charts

def bars(groups, unit, fmt=num, width=640, bar=14, gap=4):
    """Grouped horizontal bars, each group titled above its bars.
    groups: [(label, {app: (med, min, max)})]."""
    vmax = max((s[2] for _, d in groups for s in d.values() if s), default=1) or 1
    n = max(len(d) for _, d in groups)
    gh = 20 + n * (bar + gap) + 10
    h = len(groups) * gh
    plot = width - 110
    out = [f'<svg class="chart" viewBox="0 0 {width} {h}" role="img" aria-label="bar chart">']
    y = 0
    for label, d in groups:
        out.append(f'<text class="lbl" x="0" y="{y + 12}">{esc(label)}</text>')
        for i, a in enumerate(a for a in APPS if a in d):
            s = d.get(a)
            if not s:
                continue
            by = y + 20 + i * (bar + gap)
            w = max(1.5, plot * s[0] / vmax)
            x0, x1 = plot * s[1] / vmax, plot * s[2] / vmax
            out.append(f'<rect class="b-{a}" x="0" y="{by}" width="{w:.1f}" height="{bar}" rx="3"><title>{LABEL[a]}: {fmt(s[0])} {unit} (min {fmt(s[1])}, max {fmt(s[2])})</title></rect>')
            if s[2] > s[1]:
                out.append(f'<line class="rng" x1="{x0:.1f}" x2="{x1:.1f}" y1="{by + bar / 2}" y2="{by + bar / 2}"/>')
            out.append(f'<text class="val" x="{max(w, x1) + 6:.1f}" y="{by + bar / 2}" dominant-baseline="middle">{fmt(s[0])} {esc(unit)}</text>')
        y += gh
    out.append('</svg>')
    return ''.join(out)


def stacked(groups, unit, width=640, bar=14, gap=4):
    """Stacked App + Server CPU bars. groups: [(label, {app: (app_med, srv_med)})]."""
    vmax = max((sum(v) for _, d in groups for v in d.values()), default=1) or 1
    gh = 20 + len(APPS) * (bar + gap) + 10
    h = len(groups) * gh
    plot = width - 210
    out = [f'<svg class="chart" viewBox="0 0 {width} {h}" role="img" aria-label="stacked bar chart">']
    y = 0
    for label, d in groups:
        out.append(f'<text class="lbl" x="0" y="{y + 12}">{esc(label)}</text>')
        for i, a in enumerate(APPS):
            if a not in d:
                continue
            ac, sc = d[a]
            by = y + 20 + i * (bar + gap)
            wa, ws = plot * ac / vmax, plot * sc / vmax
            out.append(f'<rect class="b-{a}" x="0" y="{by}" width="{max(wa, 1):.1f}" height="{bar}" rx="3"><title>{LABEL[a]} App: {ac:.2f} s</title></rect>')
            out.append(f'<rect class="b-{a} srv" x="{wa:.1f}" y="{by}" width="{max(ws, 1):.1f}" height="{bar}" rx="3"><title>{LABEL[a]} Server: {sc:.2f} s</title></rect>')
            out.append(f'<text class="val" x="{wa + ws + 6:.1f}" y="{by + bar / 2}" dominant-baseline="middle">App {ac:.2f} + Server {sc:.2f} {esc(unit)}</text>')
        y += gh
    out.append('</svg>')
    return ''.join(out)


def timeline(tl, width=640, height=230):
    """RSS of App (solid) and Server (dashed) over run 0, workloads shaded."""
    if not tl:
        return '<p class="muted">没有 out/*0/result.json，跳过时间线。</p>'
    pad_l, pad_b, pad_t = 44, 22, 8
    series, spans, tmax, vmax = {}, [], 0, 0
    for a, (samples, times) in tl.items():
        t0 = float(times[0][1])  # shell ready
        pts_app = [(s[0] - t0, s[1]) for s in samples if s[0] >= t0]
        pts_srv = [(s[0] - t0, s[3]) for s in samples if s[0] >= t0]
        series[a] = (pts_app, pts_srv)
        tmax = max(tmax, pts_app[-1][0] if pts_app else 0)
        vmax = max(vmax, max((p[1] for p in pts_app + pts_srv), default=0))
        if a == 'keel':
            spans = [(t[0], float(t[1]) - t0, float(t[2]) - t0) for t in times if t[0] in dict(WORK[i][:2] for i in range(len(WORK)))]
    vmax = (int(vmax / 200) + 1) * 200
    pw, ph = width - pad_l - 8, height - pad_t - pad_b
    X = lambda t: pad_l + pw * t / tmax
    Y = lambda v: pad_t + ph * (1 - v / vmax)
    out = [f'<svg class="chart" viewBox="0 0 {width} {height}" role="img" aria-label="memory timeline">']
    for name, a0, a1 in spans:
        out.append(f'<rect class="span" x="{X(a0):.1f}" y="{pad_t}" width="{max(X(a1) - X(a0), 2):.1f}" height="{ph}"/>')
        out.append(f'<text class="tick" x="{X(a0) + 2:.1f}" y="{pad_t + 10}">{name}</text>')
    for v in range(0, vmax + 1, 200):
        out.append(f'<line class="grid" x1="{pad_l}" x2="{width - 8}" y1="{Y(v):.1f}" y2="{Y(v):.1f}"/><text class="tick" x="{pad_l - 6}" y="{Y(v):.1f}" text-anchor="end" dominant-baseline="middle">{v}</text>')
    for t in range(0, int(tmax) + 1, 5):
        out.append(f'<text class="tick" x="{X(t):.1f}" y="{height - 6}" text-anchor="middle">{t}s</text>')
    for a, (pa, ps) in series.items():
        for pts, cls in ((pa, ''), (ps, ' dash')):
            d = ' '.join(f'{X(t):.1f},{Y(v):.1f}' for t, v in pts)
            out.append(f'<polyline class="ln-{a}{cls}" points="{d}"/>')
    out.append('</svg>')
    return ''.join(out)

# ---------------------------------------------------------------- page

CSS = '''
:root{--bg:#f7f7f5;--card:#fff;--ink:#1d1d1f;--muted:#6e6e73;--line:#e3e3e0;--keel:#3b6fd8;--mygo:#e0782f;--span:rgba(0,0,0,.045);--good:#1f8a4c;--bad:#c2410c;--code:#f1f1ee}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#161618;--card:#1f1f22;--ink:#ececef;--muted:#9a9aa2;--line:#33333a;--keel:#6f9bff;--mygo:#ff9a52;--span:rgba(255,255,255,.05);--good:#4ade80;--bad:#fb923c;--code:#2a2a2f}}
:root[data-theme="dark"]{--bg:#161618;--card:#1f1f22;--ink:#ececef;--muted:#9a9aa2;--line:#33333a;--keel:#6f9bff;--mygo:#ff9a52;--span:rgba(255,255,255,.05);--good:#4ade80;--bad:#fb923c;--code:#2a2a2f}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:15px/1.6 -apple-system,BlinkMacSystemFont,"PingFang SC","Helvetica Neue",sans-serif}
main{max-width:920px;margin:0 auto;padding:40px 16px 80px}
h1{font-size:28px;margin:0 0 6px;letter-spacing:-.01em}
h2{font-size:19px;margin:44px 0 12px}
h3{font-size:16px;margin:22px 0 6px}
p{margin:8px 0}
.muted{color:var(--muted)}
.meta{color:var(--muted);font-size:13px}
.legend{display:flex;gap:16px;font-size:13px;color:var(--muted);margin:6px 0 2px;flex-wrap:wrap}
.legend i{display:inline-block;width:10px;height:10px;border-radius:2px;margin-right:6px;vertical-align:-1px}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;margin-top:22px}
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:14px 16px}
.card .k{font-size:12px;color:var(--muted)}
.card .v{font-size:15px;margin-top:4px;font-variant-numeric:tabular-nums}
.card .r{font-size:13px;margin-top:4px;font-weight:600}
.panel{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:16px;margin:12px 0;overflow:hidden}
svg.chart{width:100%;height:auto;display:block;font-size:12px}
.lbl{fill:var(--ink);font-size:12.5px} .val,.tick{fill:var(--muted);font-size:11px}
.b-keel{fill:var(--keel)} .b-mygo{fill:var(--mygo)} .srv{opacity:.45}
.rng{stroke:var(--ink);stroke-opacity:.35;stroke-width:1.5}
.grid{stroke:var(--line)} .span{fill:var(--span)}
polyline{fill:none;stroke-width:1.8} .ln-keel{stroke:var(--keel)} .ln-mygo{stroke:var(--mygo)} .dash{stroke-dasharray:4 3}
.tbl{width:100%;border-collapse:collapse;font-size:13px;font-variant-numeric:tabular-nums}
.tbl th,.tbl td{padding:7px 8px;border-bottom:1px solid var(--line);text-align:right;vertical-align:top}
.tbl th:first-child,.tbl td:first-child{text-align:left}
.tbl th{color:var(--muted);font-weight:500}
.tbl small{color:var(--muted)}
.tbl tr.sec td{color:var(--muted);font-size:12px;padding-top:16px;text-align:left}
.scroll{overflow-x:auto}
.win-keel{color:var(--keel)} .win-mygo{color:var(--mygo)}
ol.recs{padding-left:20px} ol.recs>li{margin:0 0 22px}
.save{display:inline-block;font-size:12px;border-radius:99px;padding:1px 9px;margin-left:6px;background:color-mix(in srgb,var(--good) 14%,transparent);color:var(--good);font-weight:600}
code{background:var(--code);border-radius:4px;padding:1px 5px;font:12.5px ui-monospace,Menlo,monospace}
pre{background:var(--code);border-radius:8px;padding:12px 14px;overflow-x:auto;font:12.5px/1.55 ui-monospace,Menlo,monospace;margin:10px 0}
pre code{background:none;padding:0}
.arch{display:grid;grid-template-columns:1fr 1fr;gap:12px}
@media (max-width:640px){.arch{grid-template-columns:1fr}h1{font-size:23px}}
ul{padding-left:20px}
'''


def legend(extra=''):
    return f'<div class="legend"><span><i style="background:var(--keel)"></i>{LABEL["keel"]}（{SUB["keel"]}）</span><span><i style="background:var(--mygo)"></i>{LABEL["mygo"]}（{SUB["mygo"]}）</span>{extra}</div>'


def build(rows, meta, probe, timelines):
    S = lambda a, f: stat(rows, a, f)
    both = lambda f: {a: S(a, f) for a in APPS}
    runs = {a: sum(1 for r in rows if r['app'] == a) for a in APPS}
    when = meta.get('date', time.strftime('%Y-%m-%d %H:%M'))

    # headline numbers
    thr = {w: both(lambda r, w=w, mb=mb: mb / r[w]['wall']) for w, mb, _ in WORK}
    idle_app = both(lambda r: fp(r, 'idle', 'app'))
    idle_srv = both(lambda r: fp(r, 'idle', 'srv'))
    peak_app = both(lambda r: fp(r, 'end', 'app', 'phys_footprint_peak'))
    peak_srv = both(lambda r: fp(r, 'end', 'srv', 'phys_footprint_peak'))
    idle_cpu = both(lambda r: (r['idle']['app_cpu'] + r['idle']['srv_cpu']) / r['idle']['secs'] * 100)
    t_win = both(lambda r: r['t_window'])

    def card(k, d, unit, lower_better=True, fmt=num):
        kk, gg = d['keel'], d['mygo']
        r = ratio(kk, gg, lower_better)
        cls = 'win-mygo' if 'GoRex' in r else 'win-keel' if 'keel' in r else ''
        return (f'<div class="card"><div class="k">{esc(k)}</div><div class="v">{LABEL["keel"]} <b>{fmt(kk[0]) if kk else "–"}</b> · '
                f'{LABEL["mygo"]} <b>{fmt(gg[0]) if gg else "–"}</b> {esc(unit)}</div><div class="r {cls}">{esc(r)}</div></div>')

    cards = ''.join([
        card('纯文本吞吐', thr['plain'], 'MB/s', lower_better=False),
        card('空闲 App 内存 (footprint)', idle_app, 'MB'),
        card('空闲 CPU (App+Server)', idle_cpu, '%', fmt=lambda v: f'{v:.1f}'),
        card('窗口出现', t_win, 's'),
    ])

    thr_chart = bars([(f'{w} · {desc}', thr[w]) for w, _, desc in WORK], 'MB/s')
    mem_chart = bars([('App 空闲', idle_app), ('App 峰值', peak_app), ('Server 空闲', idle_srv), ('Server 峰值', peak_srv)], 'MB', fmt=lambda v: f'{v:,.0f}')
    cpu_groups = []
    for w, _, desc in [('idle', 0, '空闲 10 s')] + WORK:
        d = {}
        for a in APPS:
            ac, sc = S(a, lambda r, w=w: r[w]['app_cpu']), S(a, lambda r, w=w: r[w]['srv_cpu'])
            if ac and sc:
                d[a] = (ac[0], sc[0])
        cpu_groups.append(('空闲 10 s' if w == 'idle' else f'{w} · {desc}', d))
    cpu_chart = stacked(cpu_groups, 's')

    # full table
    tbl = [('启动', None)] + [
        ('窗口出现 (s)', lambda r: r['t_window'], True),
        ('shell 就绪 (s)', lambda r: r['t_shell'], True),
        ('启动累计 App CPU (s)', lambda r: r['startup_cpu']['app_cpu'], True),
    ] + [('空闲 10 s', None)] + [
        ('App CPU (s)', lambda r: r['idle']['app_cpu'], True),
        ('Server CPU (s)', lambda r: r['idle']['srv_cpu'], True),
        ('App footprint (MB)', lambda r: fp(r, 'idle', 'app'), True),
        ('Server footprint (MB)', lambda r: fp(r, 'idle', 'srv'), True),
        ('App RSS (MB)', lambda r: r['idle']['app_rss_max'], True),
    ] + [('整轮测试', None)] + [
        ('全程 App footprint 峰值 (MB)', lambda r: fp(r, 'end', 'app', 'phys_footprint_peak'), True),
        ('全程 Server footprint 峰值 (MB)', lambda r: fp(r, 'end', 'srv', 'phys_footprint_peak'), True),
    ]
    for w, mb, desc in WORK:
        tbl += [(f'{w} · {desc}', None),
                ('耗时 (s)', lambda r, w=w: r[w]['wall'], True),
                ('吞吐 (MB/s)', lambda r, w=w, mb=mb: mb / r[w]['wall'], False),
                ('App CPU (s)', lambda r, w=w: r[w]['app_cpu'], True),
                ('Server CPU (s)', lambda r, w=w: r[w]['srv_cpu'], True),
                ('App RSS 峰值 (MB)', lambda r, w=w: r[w]['app_rss_max'], True),
                ('Server RSS 峰值 (MB)', lambda r, w=w: r[w]['srv_rss_max'], True)]
    trs, md = [], [f'| 指标 | {LABEL["keel"]} | {LABEL["mygo"]} | 对比 |', '|---|---|---|---|']
    sec = ''
    for row in tbl:
        if row[1] is None:
            sec = row[0]
            trs.append(f'<tr class="sec"><td colspan="4">{esc(sec)}</td></tr>')
            continue
        name, f, lower = row
        k, g = S('keel', f), S('mygo', f)
        cell = lambda s: '–' if not s else f'{num(s[0])}<br><small>{num(s[1])}–{num(s[2])}</small>'
        r = ratio(k, g, lower)
        cls = 'win-mygo' if 'GoRex' in r else 'win-keel' if 'keel' in r else 'muted'
        trs.append(f'<tr><td>{esc(name)}</td><td>{cell(k)}</td><td>{cell(g)}</td><td class="{cls}">{esc(r)}</td></tr>')
        md.append(f'| {sec} · {name} | {num(k[0]) if k else "–"} | {num(g[0]) if g else "–"} | {r} |')

    # bundles
    bund = meta.get('bundles', {})
    bund_html = ''
    if bund:
        bund_html = '<p class="muted">' + ' · '.join(
            f'{LABEL[a]} .app {bund[a]["bytes"] / 1e6:.1f} MB（可执行文件 {bund[a]["exe_bytes"] / 1e6:.1f} MB）' for a in APPS if a in bund) + '</p>'

    # memprobe
    rec_html = recommendations(probe, idle_app, idle_srv, peak_app, peak_srv, S)
    before_html = before_after(rows)

    keel_plain = S('keel', lambda r: r['plain']['srv_cpu'])
    page = f'''<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>keel-rex vs GoRex</title><style>{CSS}</style></head><body><main>
<h1>keel-rex vs GoRex 性能对比</h1>
<p class="meta">{esc(meta.get('model', ''))} · {esc(meta.get('chip', ''))} · {meta.get('mem_gb', '?')} GB · macOS {esc(meta.get('macos', ''))} · {esc(meta.get('go', ''))} · 各 {runs['keel']} / {runs['mygo']} 轮，取中位数 · {esc(when)}</p>
<p>同一个 Rex 终端的两种实现：<b>keel-rex</b> 基于 Keel（Gio），<b>GoRex</b> 基于 MyGo。两边都是 release 构建，使用隔离的会话目录，只开一个终端面板，由假 shell 在面板里依次 <code>cat</code> 负载文件。</p>
<div class="cards">{cards}</div>
{before_html}

<h2>终端吞吐</h2>
<p class="muted">计时从 <code>cat</code> 开始到 PTY 里的数据被读完。细线是多轮的最小–最大值。</p>
<div class="panel">{legend()}{thr_chart}</div>

<h2>内存</h2>
<p class="muted">macOS <code>footprint</code> 统计的 phys_footprint，包含 Go 堆和 GPU 内存。空闲值在 shell 就绪 7 秒后采集，峰值是整轮测试中的最大值。</p>
<div class="panel">{legend()}{mem_chart}</div>
<div class="panel">{legend('<span>实线 App · 虚线 Server · 灰底为负载区间</span>')}{timeline(timelines)}<p class="muted" style="margin:6px 0 0;font-size:12px">第 0 轮的 RSS（MB）时间线，横轴为 shell 就绪后的秒数。</p></div>

<h2>CPU</h2>
<p class="muted">每个阶段的 CPU 时间：实色是 App 进程，浅色是 Server 进程。负载阶段包括 <code>cat</code> 结束后 4 秒内 App 追着渲染的部分。</p>
<div class="panel">{legend()}{cpu_chart}</div>

<h2>差距从哪来</h2>
<p>多数差距来自 App 层的架构选择，而不是 Keel 和 MyGo 这两个 UI 框架本身：</p>
<div class="arch">
<div class="card"><div class="k">keel-rex</div><ul>
<li>Server 用 fork 的 <b>charmbracelet/vt</b>（<code>third_party/vt</code>）解析：整行滚动、历史打包、成屏输出直接进历史，纯文本负载 Server CPU {num(keel_plain[0]) if keel_plain else '–'} s。</li>
<li>Server 把变化的行用二进制推给 App（长连接，最多 60 次/秒）；App 只重绘变化的行，没变的行重放录好的操作。</li>
<li>Gio 把文字画成矢量路径、每条路径一个 Metal 缓冲区：App 的 GPU 内存和 Go 堆里的字形缓存是剩下的主要差距。</li></ul></div>
<div class="card"><div class="k">GoRex</div><ul>
<li>Server 和 App 各跑一份 <b>libghostty-vt</b>（Zig，通过 purego 调用），Server 把原始字节流推给 App。</li>
<li>App 同样要做一遍 VT 解析，所以大量输出时 App CPU 比 keel-rex 高。</li>
<li>内嵌 JetBrains Mono，中文交给系统回退字体。</li></ul></div>
</div>
<p>吞吐和 Server 端的差距来自 VT 库和应用架构；App 进程的 CPU 和内存差距里，有一部分是 Keel / Gio 本身的：同样的 UI 场景单独对比，MyGo 每帧 CPU 低 2.4–2.8 倍、空闲内存低 6.6 倍（见 <code>uibench/report.html</code>）。</p>
{bund_html}

<h2>keel-rex 优化记录</h2>
{rec_html}

<h2>完整数据</h2>
<div class="panel scroll"><table class="tbl"><thead><tr><th>指标</th><th>{LABEL['keel']}</th><th>{LABEL['mygo']}</th><th>对比</th></tr></thead><tbody>{''.join(trs)}</tbody></table>
<p class="muted" style="font-size:12px">中位数，下方小字为最小–最大值。</p></div>

<h2>方法与局限</h2>
<ul class="muted">
<li><code>run.py</code> 用 <code>open -n</code> 启动 .app，<code>KEEL_REX_DIR</code> / <code>GOREX_DIR</code> 指向隔离目录。keel-rex 预置单面板 <code>layout.json</code>，去掉默认的 Git 面板。</li>
<li><code>shell.zsh</code> 作为 <code>$SHELL</code>，只在第一个面板里运行：先空闲 10 秒，再依次 <code>cat</code> 负载，用 <code>$EPOCHREALTIME</code> 打时间戳。</li>
<li>每 100 ms 用 <code>ps</code> 采样 App 和 Server 的 CPU / RSS，用 <code>winwait</code>（CGWindowList）测窗口出现时间。</li>
<li>没有测按键回显延迟和帧率。窗口尺寸基本相同但没有完全对齐（GoRex 会恢复自己记住的窗口大小）。测试时机器上还有其他应用在运行，数据有噪声。</li>
<li>重新生成：<code>python3 gen_data.py && RUNS=3 python3 run.py &gt; results.log && python3 report.py</code></li>
</ul>
</main></body></html>'''
    return page, '\n'.join(md)


def before_after(rows):
    """keel-rex before and after the VT work, from results-before.log."""
    path = os.path.join(B, 'results-before.log')
    if not os.path.exists(path):
        return ''
    old = [json.loads(l) for l in open(path) if l.startswith('{')]
    items = [(f'{w} 吞吐 (MB/s)', lambda r, w=w, mb=mb: mb / r[w]['wall'], False) for w, mb, _ in WORK]
    items += [('plain Server CPU (s)', lambda r: r['plain']['srv_cpu'], True),
              ('Server footprint 峰值 (MB)', lambda r: fp(r, 'end', 'srv', 'phys_footprint_peak'), True),
              ('App 空闲 footprint (MB)', lambda r: fp(r, 'idle', 'app'), True)]
    trs = []
    for name, f, lower in items:
        a, b, g = stat(old, 'keel', f), stat(rows, 'keel', f), stat(rows, 'mygo', f)
        if not a or not b:
            continue
        x = a[0] / b[0] if lower else b[0] / a[0]
        change = '持平' if 0.9 < x < 1.1 else (f'{x:.1f}× 更好' if x > 1 else f'{1/x:.1f}× 更差')
        trs.append(f'<tr><td>{esc(name)}</td><td>{num(a[0])}</td><td><b>{num(b[0])}</b></td><td>{change}</td><td class="muted">{num(g[0]) if g else "–"}</td></tr>')
    return f'''<h2>keel-rex 优化前后</h2>
<p class="muted">优化前是第一次测量（原版 charmbracelet/vt、逐个 1 KB 读取 PTY、45 ms 轮询 JSON 整屏、逐格排版、整包加载 PingFang）。具体改动见「keel-rex 优化记录」。</p>
<div class="panel scroll"><table class="tbl"><thead><tr><th>指标</th><th>优化前</th><th>现在</th><th>变化</th><th>GoRex</th></tr></thead><tbody>{''.join(trs)}</tbody></table></div>'''


def recommendations(probe, idle_app, idle_srv, peak_app, peak_srv, S):
    sb = (probe or {}).get('scrollback', [])
    sb_rows = ''.join(f'<tr><td>{x["lines"]:,} 行 × {x["cols"]} 列</td><td>{x["heap_mb"]:.0f} MB</td><td>{x["bytes_per_cell"]:.0f} B</td></tr>' for x in sb)
    ia = idle_app['keel'][0] if idle_app['keel'] else None
    pa = peak_app['keel'][0] if peak_app['keel'] else None
    return f'''
<h3>已完成</h3>
<ol class="recs">
<li><b>VT</b>（<code>keel-rex/third_party/vt</code>，fork 的 charmbracelet/vt，属于终端自己的领域）：整行滚动；历史改成环形缓冲，后台打包成「文本 + 压缩样式」，按 10000 行和 8 MB 双重封顶；一屏以上的整行输出（含行内 SGR、折行、CJK/emoji）直接从字节写进历史；连续文本整段写入；备用屏不再保留历史。每项都有和逐字节处理比对的随机差分测试，2000 个种子。</li>
<li><b>Server</b>：读 PTY 和解析分两个 goroutine，到达的数据合并成大块再交给 VT。</li>
<li><b>Server→App</b>：长连接推送，只发变化的行，二进制编码，最多 60 次/秒，锁外编码。</li>
<li><b>App 绘制</b>：按行、按同样式的段排版，没变化的行重放上一帧录好的操作；周期性检查改用 keel 的 <code>cx.Poll</code>，只有状态变了才重绘。</li>
<li><b>keel（底层改动都收敛在这里）</b>：
<ul>
<li>字体：<code>theme.LoadFontsWhere</code> / <code>LoadFontFilesWhere</code> 只加载需要的字形；Shaper 首次排版时才初始化；fork 的 go-text 把系统字体改成内存映射、字形轮廓按需解析。</li>
<li>Gio fork：Metal 缓冲区复用；staging 缓冲和离屏纹理长时间不用就收缩或释放；长段文字不进路径缓存；显示缓冲从 3 块减到 2 块。</li>
<li><code>cx.Poll</code>：在帧之外执行的周期检查。</li>
</ul></li>
</ol>
<div class="panel"><table class="tbl"><thead><tr><th>历史行数</th><th>存活堆</th><th>每 cell</th></tr></thead><tbody>{sb_rows}</tbody></table>
<p class="muted" style="font-size:12px">memprobe 用的是同一种样式的整行文字。</p></div>
<h3>还没追平的</h3>
<ol class="recs">
<li><b>App 内存（空闲 {num(ia)} MB，峰值 {num(pa)} MB）</b>：空闲时 RSS 已经和 GoRex 相当，footprint 多出来的主要是 GPU 内存和 Go 堆在大量输出后留下的余量。要再降，得把 Gio 的矢量文字改成字形图集。</li>
<li><b>App 空闲 CPU</b>：Go 代码只占 1–2%，其余是光标闪烁时整窗重绘在 Metal 和 AppKit 里的开销。MyGo 遇到这种小改动只在 CPU 上重画一小块，keel 要做到同样的效果，需要支持局部重绘。</li>
<li><b>Server 内存峰值</b>：大量输出时 Go 堆的临时余量，空闲时已经降到十几 MB。</li>
</ol>
'''


if __name__ == '__main__':
    rows, meta, probe, timelines = load()
    page, md = build(rows, meta, probe, timelines)
    path = os.path.join(B, 'report.html')
    open(path, 'w').write(page)
    print(path)
    if '--md' in sys.argv:
        print(md)
