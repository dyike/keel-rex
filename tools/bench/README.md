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
