# keel-rex vs GoRex 黑盒 benchmark

`run.py` 用 `open -n` 启动两个 .app，用隔离状态目录（`KEEL_REX_DIR` / `GOREX_DIR`）和假 shell `shell.zsh`
在首个面板里依次 `cat` 负载文件，同时每 100ms 采样 App 和 Server 进程的 CPU/RSS，空闲和结束时用 `footprint` 取内存。
`memprobe` 在单个进程里复现 keel-rex 的字体加载和 VT 历史，测各项占用的 Go 堆。`report.py` 把这些结果生成 `report.html`。

```sh
python3 gen_data.py                       # data/：plain 20MB、ansi 10MB、cjk 5MB、frames 500 帧
swiftc -O winwait.swift -o winwait
KEEL_APP=".../Rex Keel.app" GOREX_APP=".../GoRex.app" RUNS=3 python3 run.py > results.log
python3 report.py --md                    # 写 report.html；缺 memprobe.json 时先跑 go run ./tools/bench/memprobe
```

## 终端渲染回归

项目根目录运行：

```sh
REX_RENDER_PROFILE_DIR=/tmp/rex-render go test ./internal/ui -run '^$' -bench '^BenchmarkTerminalScrolling$' -benchtime=120x -count=1
# 在同级 Keel 仓库运行公共绘制入口的对照基准
go -C ../keel test ./ui/theme -run '^$' -bench '^BenchmarkGlyphRendererScrolling$' -benchtime=120x -count=1
```

基准保持同一个离屏 GPU 渲染器，让 127 列、31 行每帧变化；画布为 1912×992 物理像素、2× 缩放。覆盖普通文本、彩色粗体和中英文混排。每次重新创建截图窗口会掩盖 GPU 资源保留，因此这里连续渲染后才生成截图和内存摘要。

`REX_RENDER_PROFILE_DIR` 可省略。设置后输出三类负载的 PNG，以及 macOS 的 `vmmap -summary`；`live-heap-MiB` 是采样前 GC 后的整个测试进程 Go 堆。GPU 设备不可用时基准失败，避免误报无绘制的性能。不同子基准共享字体 Shaper，物理内存包含前面负载与驱动保留资源；比较时必须使用相同顺序和帧数。

2026-10-09，Apple M5 Pro、每种负载 120 帧的一轮同机对照：

| 负载 | 原整段绘制 ms/帧 | Keel GlyphRenderer ms/帧 | 每帧分配 MB，前 → 后 | footprint MiB，前 → 后 |
| --- | ---: | ---: | ---: | ---: |
| 普通文本 | 8.55 | 3.52 | 28.10 → 0.80 | 550.4 → 287.6 |
| 彩色粗体 | 13.85 | 5.06 | 29.33 → 2.31 | 373.5 → 287.6 |
| 中英文混排 | 19.28 | 3.85 | 49.53 → 2.73 | 687.4 → 337.1 |

这是固定离屏负载，不能直接推算日常窗口占用；真实 App 还包含窗口表面、UI、多个面板和会话。公共优化放在同级 Keel 的 `GlyphRenderer`，此基准验证 rex 的接入方式。图集处理水平小数位置后，边缘可能比原覆盖纹理的小数平移更清晰；整数位置、复杂文字回退、中文、Emoji、选区和主题切换分别有 Keel 与 rex 的绘制回归覆盖。
