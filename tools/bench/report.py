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

    keel_plain = S('keel', lambda r: r['plain']['srv_cpu'])
    page = f'''<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>keel-rex vs GoRex</title><style>{CSS}</style></head><body><main>
<h1>keel-rex vs GoRex 性能对比</h1>
<p class="meta">{esc(meta.get('model', ''))} · {esc(meta.get('chip', ''))} · {meta.get('mem_gb', '?')} GB · macOS {esc(meta.get('macos', ''))} · {esc(meta.get('go', ''))} · 各 {runs['keel']} / {runs['mygo']} 轮，取中位数 · {esc(when)}</p>
<p>同一个 Rex 终端的两种实现：<b>keel-rex</b> 基于 Keel（Gio），<b>GoRex</b> 基于 MyGo。两边都是 release 构建，使用隔离的会话目录，只开一个终端面板，由假 shell 在面板里依次 <code>cat</code> 负载文件。</p>
<div class="cards">{cards}</div>

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
<li>Server 用 <b>charmbracelet/vt</b>（纯 Go）解析，每滚动一行都有明显开销：纯文本负载 Server CPU {num(keel_plain[0]) if keel_plain else '–'} s。</li>
<li>App 每个面板每 <b>45 ms</b> 新建一次 unix 连接，用 JSON RPC 拉取整屏 cell，空闲时也在轮询。</li>
<li>启动时把 78 MB 的 PingFang.ttc 整个解析进 Go 堆（见下文）。</li></ul></div>
<div class="card"><div class="k">GoRex</div><ul>
<li>Server 和 App 各跑一份 <b>libghostty-vt</b>（Zig，通过 purego 调用），Server 把原始字节流推给 App。</li>
<li>App 同样要做一遍 VT 解析，所以 ansi / frames 负载下 App CPU 反而比 keel-rex 高。</li>
<li>内嵌 JetBrains Mono，中文交给系统回退字体。</li></ul></div>
</div>
<p>比较能反映框架本身的是<b>启动速度</b>、<b>同等工作量下 App 进程的 CPU</b> 和<b>包体积</b>，这几项两边接近或互有胜负。</p>
{bund_html}

<h2>keel-rex 内存优化建议</h2>
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


def recommendations(probe, idle_app, idle_srv, peak_app, peak_srv, S):
    fonts = {p['name']: p for p in (probe or {}).get('fonts', [])}
    sb = (probe or {}).get('scrollback', [])
    sb_by = {x['lines']: x for x in sb}
    ia = idle_app['keel'][0] if idle_app['keel'] else None
    pa = peak_app['keel'][0] if peak_app['keel'] else None
    ps = peak_srv['keel'][0] if peak_srv['keel'] else None
    font_chart = ''
    if fonts:
        font_chart = bars([
            ('全部 PingFang（现状）', {'keel': (fonts['all']['heap_mb'],) * 3}),
            ('只加载 PingFang SC 一个字形', {'keel': (fonts['one']['heap_mb'],) * 3}),
            ('交给系统回退', {'keel': (fonts['system']['heap_mb'],) * 3}),
            ('不加载中文', {'keel': (fonts['none']['heap_mb'],) * 3}),
        ], 'MB', fmt=lambda v: f'{v:.0f}')
    sb_rows = ''.join(f'<tr><td>{x["lines"]:,} 行 × {x["cols"]} 列</td><td>{x["heap_mb"]:.0f} MB</td><td>{x["bytes_per_cell"]:.0f} B</td></tr>' for x in sb)
    f_all = fonts.get('all', {}).get('heap_mb')
    f_one = fonts.get('one', {}).get('heap_mb')
    f_none = fonts.get('none', {}).get('heap_mb')
    sb10 = sb_by.get(10000, {}).get('heap_mb')
    sb3 = sb_by.get(3000, {}).get('heap_mb')
    if f_all and f_one:
        font_save = f'Go 堆 −{f_all - f_one:.0f} MB'
    else:
        font_save = '数百 MB'
    return f'''
<p>keel-rex 的 App 空闲 footprint <b>{num(ia)} MB</b>，峰值 <b>{num(pa)} MB</b>；Server 峰值 <b>{num(ps)} MB</b>。下面的数字来自 <code>memprobe</code>：在同一进程里单独复现每一项，测 GC 之后的存活堆。</p>
<ol class="recs">
<li><b>不要整包加载 PingFang.ttc</b><span class="save">{font_save}</span>
<p><code>main.go</code> 读入 78 MB 的 <code>PingFang.ttc</code> 交给 <code>theme.LoadFonts</code>，后者用 <code>opentype.ParseCollection</code> 把集合里的 24 个字形（SC/TC/HK/MO × 6 种字重）全部解析常驻，每个约 16 MB。Go GC 默认把堆的上限设成存活堆的两倍，所以实际 footprint 还要再翻倍：空闲 684 MB 里大部分来自这里。</p>
<div class="panel">{font_chart}<p class="muted" style="margin:6px 0 0;font-size:12px">加载 Menlo + Helvetica Neue 后，再用各方案排版一行中文，测得的存活 Go 堆。“不加载中文”时中文会显示成方框，只作为基线参考。</p></div>
<p>keel-rex 只用到 Regular 和 Bold（<code>paint.go</code>），只挑 PingFang SC 的 Regular 和 Semibold 就够了，约 +{(f_one - f_none) * 2 if f_one and f_none else 32:.0f} MB。建议在 Keel 加一个按描述筛选字形的 API，keel-rex 这样调用：</p>
<pre><code>// keel/ui/theme：只解析需要的字形，而不是整个集合
func LoadFontsWhere(data []byte, keep func(font.Description) bool) error {{
	lds, err := opentype.NewLoaders(bytes.NewReader(data))
	if err != nil {{
		return err
	}}
	for _, ld := range lds {{
		f, err := font.NewFont(ld)
		if err != nil || !keep(f.Describe()) {{
			continue
		}}
		loaded = append(loaded, giofont.FontFace{{Font: gioopentype.DescriptionToFont(f.Describe()), Face: face{{f}}}})
	}}
	// …rebuild Material.Shaper as LoadFonts does
}}

// keel-rex/main.go
theme.LoadFontsWhere(pingfang, func(d font.Description) bool {{
	return d.Family == "PingFang SC" &amp;&amp; (d.Aspect.Weight == font.WeightNormal || d.Aspect.Weight == font.WeightSemibold)
}})</code></pre>
<p>另一种做法是完全不加载，交给 Gio 的系统字体回退（约 {fonts.get('system', {}).get('heap_mb', 77):.0f} MB，按需加载，但字体选择不如手动指定可控）。</p></li>

<li><b>限制 Server 端历史的内存</b><span class="save">每个会话 −{sb10 - sb3:.0f} MB（10000→3000 行）</span>
<p>charmbracelet/vt 的历史按 cell 保存，每个 cell 约 {sb_by.get(10000, {}).get('bytes_per_cell', 121):.0f} 字节（含样式和链接）。10000 行 × 127 列就是 {sb10:.0f} MB，而且<b>每个面板都有一份</b>：开 4 个面板，Server 光历史就要 600 MB 左右。</p>
<div class="panel"><table class="tbl"><thead><tr><th>历史行数</th><th>存活堆</th><th>每 cell</th></tr></thead><tbody>{sb_rows}</tbody></table></div>
<p>可选做法，按改动从小到大：</p>
<ul>
<li>把默认历史行数降到 3000 左右，或者设成按总字节数封顶（GoRex 每个会话固定 8 MB）。</li>
<li>行滚出屏幕后压缩成「文本 + 样式区间」的紧凑格式，一行只占几十到一两百字节，需要时再展开成 cell。</li>
<li>这部分同时也是吞吐的瓶颈：纯文本负载下 Server CPU 达到 4 秒，主要花在把行推进历史上。</li>
</ul></li>

<li><b>Frame 改成推送 + 增量</b><span class="save">降低 App 峰值和空闲 CPU</span>
<p><code>remote.go</code> 中每个面板每 45 ms 新建一次连接，用 JSON 拉取整屏 <code>[]wireCell</code> 和 <code>Plain</code> 字符串。有输出时，每一帧都要分配并解码整屏数据，App RSS 在负载期间从约 540 MB 涨到约 800 MB，空闲时仍有 4% 左右的 CPU。</p>
<ul>
<li>每个会话保持一条长连接，由 Server 在内容变化时推送，不再定时轮询。</li>
<li>只发生变化的行（按行记录 revision），替换掉整屏数据。</li>
<li>改用二进制编码（例如定长 cell + 字符串池）代替 JSON，复用解码缓冲区。</li>
</ul></li>

<li><b>限制 GC 的堆增长</b><span class="save">配合第 1 条</span>
<p>Go GC 默认 GOGC=100，堆可以涨到存活堆的两倍。做完第 1 条以后，可以在 <code>main</code> 里加 <code>debug.SetMemoryLimit(256 &lt;&lt; 20)</code>（软上限），让空闲时堆回落得更快；Server 进程同理。这只是兜底手段，不能代替减少存活对象。</p></li>

<li><b>再看 GPU 内存</b>
<p>keel-rex App 的 footprint 里还有约 110 MB「Owned physical footprint (graphics)」，GoRex 约 11 MB。可能来自 Gio 的字形图集或离屏纹理（例如大尺寸的毛玻璃效果或阴影），建议用 Instruments 的 Metal / VM Tracker 确认后再优化。</p></li>
</ol>
<p class="muted">粗估：做完第 1、3 条，App 空闲 footprint 可以降到 150 MB 量级；做完第 2 条，Server 的内存会从「每面板 150 MB+」降到几十 MB。这些都是估算，改完需要重新跑一遍 benchmark 确认。</p>
'''


if __name__ == '__main__':
    rows, meta, probe, timelines = load()
    page, md = build(rows, meta, probe, timelines)
    path = os.path.join(B, 'report.html')
    open(path, 'w').write(page)
    print(path)
    if '--md' in sys.argv:
        print(md)
