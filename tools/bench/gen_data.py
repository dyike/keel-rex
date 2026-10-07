# Generates data/{plain,ansi,cjk,frames}.txt for run.py (seeded, deterministic).
import os, random
random.seed(1)
d = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'data') + '/'
os.makedirs(d, exist_ok=True)
words = [''.join(random.choice('abcdefghijklmnopqrstuvwxyz') for _ in range(random.randint(2, 9))) for _ in range(500)]
with open(d + 'plain.txt', 'w') as f:
    n = i = 0
    while n < 20_000_000:
        l = f"{i:08d} INFO " + ' '.join(random.choice(words) for _ in range(14)) + '\n'; f.write(l); n += len(l); i += 1
with open(d + 'ansi.txt', 'w') as f:
    n = 0
    while n < 10_000_000:
        parts = []
        for _ in range(10):
            c = random.randint(16, 231)
            parts.append(f"\x1b[38;5;{c}m{random.choice(words)}\x1b[0m \x1b[1;38;2;{random.randint(0,255)};{random.randint(0,255)};{random.randint(0,255)}m{random.choice(words)}\x1b[0m")
        l = ' '.join(parts) + '\n'; f.write(l); n += len(l.encode())
cjk = '终端渲染性能测试中文字符宽度对齐验证与混合排版效果检查日志输出滚动'
emo = '🚀✅🔥📦🎉'
with open(d + 'cjk.txt', 'w') as f:
    n = 0
    while n < 5_000_000:
        l = ''.join(random.choice(cjk) for _ in range(30)) + ' ' + random.choice(emo) + ' ' + random.choice(words) + '\n'; f.write(l); n += len(l.encode())
with open(d + 'frames.txt', 'w') as f:
    f.write('\x1b[?1049h\x1b[?25l')
    for fr in range(500):
        f.write('\x1b[H')
        for r in range(40):
            f.write(f"\x1b[48;5;{(r+fr)%216+16}m\x1b[38;5;{(r*7+fr)%216+16}m" + ''.join(random.choice('▁▂▃▄▅▆▇█ ░▒▓') for _ in range(100)) + '\x1b[0m\r\n')
    f.write('\x1b[?25h\x1b[?1049l')
