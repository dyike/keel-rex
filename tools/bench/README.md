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

### 正式 v0.1.9 复测与大字号问题

2026-10-09，同机正式依赖复测：默认 12.5 字号三轮 120 帧，普通文本 3.81–4.37 ms/帧、彩色粗体 5.03–5.20 ms/帧、中英文 3.95–4.01 ms/帧。增加到 1200 帧后分别为 3.39、4.69、3.98 ms/帧，GC 后存活堆分别为 60.89、56.55、79.95 MiB，与短基准相比增长不到 0.1 MiB；这轮测试未观察到随帧数增长的 Go 堆泄漏。绘制、选区和主题切换回归通过。

本地应用实际使用 22.5 字号。补充基准保持 127 列、31 行和 2× 缩放，按字号扩大画布以容纳全部行列；22.5 画布为 3441×1786，25 画布为 3824×1984。每类 120 帧，结果如下：

| 字号与基线 | 普通文本 ms/帧 | 彩色粗体 ms/帧 | 中英文 ms/帧 |
| --- | ---: | ---: | ---: |
| 22.5，小数基线 | 16.08 | 15.35 | 12.16 |
| 25，整数基线 | 3.91 | 6.78 | 4.19 |

22.5 字号各负载采样时物理内存为 549–794 MiB。更大字号的整数基线对照反而更快，结合 `GlyphRenderer` 对小数垂直位置使用矢量回退的实现，说明这条回退路径仍存在性能缺口。默认字号基准不能代表所有字号，后续公共优化需要在 Keel 中解决并覆盖小数垂直位置。

```sh
go test ./internal/ui -run '^$' -bench '^BenchmarkTerminalScrolling$' -benchtime=1200x -count=1
# 同时覆盖小数基线与更大字号的整数基线
go test ./internal/ui -run '^$' -bench '^BenchmarkTerminalScrollingLargeFont$' -benchtime=120x -count=1
```

另对当时正在使用的正式 App 采样 30 秒：App 平均 CPU 42.85%、RSS 389–397 MiB，Server 平均 CPU 2.71%、RSS 63–68 MiB；`vmmap` 的 App 物理内存约 1.1 GiB，图形资源约 596 MiB、IOSurface 约 166 MiB。没有控制该窗口的输入和输出，这些数字不是空闲基准，也不能据此确认内存泄漏。堆栈采样可见 Gio GPU packing 与 Metal 绘制开销。

### 小数基线优化复测

同日将改动放入同级 Keel 的 `GlyphRenderer`：垂直相位以 1/64 像素精度进入掩码缓存，按相位分批栅格化并隔离临时 GPU 覆盖缓存；自定义绘制器的图片页按需分配，上限 32 MiB、每个掩码最多 32 种颜色，独立 `GlyphAtlas` 的默认预算不变。保留复杂文字、透明颜色、位图字形和预算不足时的回退。

保持上一节 22.5 字号、127×31、2× 和三种负载顺序，每类 120 帧：

| 负载 | v0.1.9 ms/帧 | 本地优化 ms/帧 | 物理内存 MiB，前 → 后 |
| --- | ---: | ---: | ---: |
| 普通文本 | 16.08 | 4.56 | 794.1 → 375.2 |
| 彩色粗体 | 15.35 | 10.06 | 548.7 → 459.9 |
| 中英文 | 12.16 | 5.09 | 604.1 → 437.1 |

每类 1200 帧时分别为 4.11、8.07、4.07 ms/帧。三类掩码占用约 0.40、0.84、0.73 MiB，图片页分别为 3、32、4 MiB，与 120 帧测试相同；彩色文本仍有少量预算回退。首次栅格化和上传计入时间，长基准摊薄了预热成本。这是离屏渲染器结果，不能直接推算实际窗口的 CPU 或总内存。

Keel 的像素回归覆盖 64 种垂直相位、接近整数边界的位置、负原点、普通/粗体、复杂文本和透明颜色；原覆盖容差保持不变，并增加垂直覆盖中心与缓存复用断言。两个仓库的完整 race 测试及静态检查通过。基准额外报告 `mask-MiB`、`page-MiB` 与 `vector-draws/frame`，用于观察缓存是否饱和。
