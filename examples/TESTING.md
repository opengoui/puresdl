# 示例的自动化测试与手动检查清单

每个示例目录里都有 `main_test.go`（同为 `package main`），与 `main.go` 放在一起。测试在无头环境下运行，不开真实窗口、不联网、不写源码树。

```bash
CGO_ENABLED=0 go test ./examples/... -count=1                  # 不需要设置任何 SDL_* 环境变量
go test ./examples/... -count=3 -shuffle=on                    # 稳定性检查
go test -race ./examples/...
go test ./examples/renderer/04-points -run TestXxx -update     # 重写该包 testdata/ 下的 golden，之后必须审查 diff
```

## 测试如何工作

- **测试宿主 `internal/sdltest`**：每个示例的 `TestMain` 调用 `os.Exit(sdltest.Main(m))`。`Main` 先调用 `Env()`，选择 dummy 视频/音频/摄像头驱动和 software 渲染器（用 SDL hint 加进程环境变量），所以调用者不需要设置 `SDL_VIDEODRIVER` 等变量。SDL 要求视频与事件在主线程，`Main` 让主 goroutine 留在主 OS 线程，测试在另一个 goroutine 里运行，需要主线程的代码用 `sdltest.OnMain` / `sdltest.Call` 投递过去。SDL 状态是进程全局的，这类测试不能 `t.Parallel()`。
- **找不到 libSDL3 就跳过**：`Setup` / `Skip` 检查 `sdl.LoadError()`，失败时 `t.Skip`，不会失败。可用 `SDL3_LIBRARY=/nonexistent go test ./examples/...` 验证。
- **示例的可测试结构**：`main.go` 被拆成 `newApp` / `handleEvent` / `draw` / `close` 之类的小单元，时钟 `now`、随机数、电源/存储等外部来源可注入；`run()` 只负责 Init、窗口、主循环，由 `run(N)` 冒烟测试覆盖。`-frames N` 参数保留。
- **事件注入**：`sdltest.NewInput(t, windowID)` 通过 `sdl.PushEvent` 注入键盘、鼠标、窗口等事件，`Pump` / `Drain` 取出来喂给 `handleEvent`。注入的事件不会改变轮询状态（键盘/鼠标状态数组），只有消费事件的代码能观察到。事件只能在 `sdl.Init` 之后注入，所以 `run()` 自己 Init/Quit 的路径无法被注入驱动。
- **虚拟设备**：`AttachJoystick` / `AttachGamepad` 创建 SDL 虚拟摇杆/手柄（`VirtualJoystick` 可设置按键、轴、帽，并记录 `Rumbles()`）；`NewAudioProbe` 创建不绑定设备的音频流，可以确定性地检查队列深度；`Clock` 提供可控时钟，避免 sleep 和时间断言。
- **像素捕获与 golden**：`NewCanvas` 在软件表面上建渲染器，`Capture` 用 `RenderReadPixels` 读回；`AssertPixel` / `AssertRegion` 检查像素，`AssertImage` / `AssertImageTol`（带容差，吸收抗锯齿差异）与 `testdata/<name>.png` 比较。失配时实际输出写到 `testdata/failed/`（已在 `.gitignore`）。`-update` 重写 golden，提交前必须人工审查图片 diff。需要缩放/letterbox 的测试改用隐藏的 dummy 窗口 + software 渲染器。

## 如何新增一个示例测试

1. 把 `main.go` 重构成可测试单元（状态结构体 + `handleEvent` + `draw`，`draw` 不 present，时钟可注入），`run()` 保持薄。
2. 新建 `main_test.go`，`TestMain` 用 `sdltest.Main`；每个用到 SDL 的测试先 `sdltest.Setup(t, flags)`（或 `Skip`）。
3. 断言可观察行为：像素、事件处理结果、队列深度、状态迁移。不要断言源码子串，不要用 sleep，不要写源码树（临时文件用 `t.TempDir()`）。
4. 至少一个 `run(3)` 冒烟测试；跑 `-count=5`、`-race`、`-shuffle=on` 确认不抖动。

## 无法自动覆盖的内容

- 真实窗口管理器行为：窗口缩放、关闭按钮、letterbox 黑边的屏幕表现（逻辑呈现下 `RenderReadPixels` 只返回逻辑区域，测试改查 `GetRenderLogicalPresentationRect`）。
- 真实 GPU / Metal 渲染器输出、vsync 与呈现节奏、动画观感。
- 真实音频设备：音高、可听性、格式转换、卡顿（dummy 设备会按实时速度消耗队列）。
- 真实输入硬件：多鼠标/键盘设备归属、实体手柄的校准、电量、震动、笔的压力/倾斜、摄像头与系统权限弹窗。
- 真实系统资源：系统剪贴板、系统语言设置、电源状态、用户存储目录。
- 调试字体字形的外观（只检查有墨迹/位置）。

## 手动检查清单

通用做法：`go run ./examples/<类别>/<编号-名称>`（真实窗口）。除特别说明外，关闭按钮应退出程序，把窗口拖拽缩放后画面保持逻辑比例居中，四周是黑边。

### asyncio

- [ ] `asyncio/01-load-bitmaps`：窗口为 640x480，可缩放，标题正确；应看到多张位图被异步加载后显示在窗口中；缩放时保持 4:3 居中。加载顺序不固定，属正常。

### audio（需要真实声卡与耳朵）

- [ ] `audio/01-simple-playback`：启动后应听到持续的正弦音，音高与音量稳定，无爆音；关闭窗口停止。
- [ ] `audio/02-simple-playback-callback`：应听到 440Hz 音调，无卡顿/欠载（underrun）；窗口关闭后声音停止；黑色窗口缩放时保持 letterbox。
- [ ] `audio/03-load-wav`：应听到 `sample.wav` 循环播放，音质清晰；按 Esc 或关闭窗口退出（Go 移植额外支持 Esc，C 版只响应关闭）。
- [ ] `audio/04-multiple-streams`：同时听到两个声音（sample/sword）按时间错开混合，节奏正确；窗口为黑色。
- [ ] `audio/05-planar-data`：窗口显示 LEFT / RIGHT 提示；点击左半边应只从左声道发声，右半边只从右声道；音高正确（4000Hz 采样的声音），播放期间文字高亮，结束后恢复。

### camera

- [ ] `camera/01-read-and-draw`：有摄像头时，弹出系统权限提示，允许后应显示实时画面，帧率流畅、朝向/镜像正确；拒绝权限应退出并有提示；没有摄像头时输出提示并以 0 退出；Esc 退出。

### demo

- [ ] `demo/01-snake`：方向键/WASD/手柄方向控制蛇移动，吃到食物变长，撞墙或自身重新开始；画面清晰、帧率稳定；实体摇杆/帽开关可用。
- [ ] `demo/02-woodeneye-008`：连接多个鼠标和键盘时，每个设备控制一个玩家视角；鼠标相对移动捕获可用，射击与得分显示正常；检查不同键盘布局下按键对应。
- [ ] `demo/03-infinite-monkeys`：屏幕上不断滚动随机字符，满行后换行/滚屏；缩放窗口后文字仍清晰；调试字体外观正常。
- [ ] `demo/04-bytepusher`：拖入一个 BytePusher 程序文件（如 esolangs 上的演示）应开始运行，画面与声音正确，60Hz 稳定；键盘 0-9/A-F 映射的键能被程序读取；窗口大小按整数倍缩放。

### input

- [ ] `input/01-joystick-polling`：接入实体摇杆应看到其名称、轴、按钮和帽的实时状态；拔掉后消失；校准与死区符合预期。
- [ ] `input/02-joystick-events`：推动摇杆轴/球/帽、按下按钮时，屏幕上出现浮动文字并逐渐淡出；插拔设备和电池变化有提示；Esc 不会退出（与 C 一致，只响应关闭）。
- [ ] `input/03-gamepad-polling`：接入手柄后背景是 `gamepad_front.png`，按下按钮/推动摇杆时对应矩形与图案位置对齐；扳机以数值显示；缩放窗口后保持比例。
- [ ] `input/04-gamepad-events`：手柄事件（按钮、轴、插拔、电量）以文字提示淡出显示；动画计时平滑。
- [ ] `input/05-gamepad-rumble`：按手柄按钮应触发震动（低频/高频不同），松开约 5 秒内震动自动停止；热插拔手柄；Esc 退出。

### misc

- [ ] `misc/01-power`：显示当前电源状态（电池/接电/百分比/剩余时间）；拔插电源适配器后状态更新；台式机显示无电池。
- [ ] `misc/02-clipboard`：复制一段外部文本后，窗口显示粘贴内容；点击/按键设置剪贴板后到别的程序粘贴能得到内容；文字过多时在约 36 行处截断（沿用 C 行为）。
- [ ] `misc/03-locale`：显示的语言列表与系统设置一致；更改系统首选语言后重启应更新。

### pen

- [ ] `pen/01-drawing-lines`：用数位板/触控笔在窗口里绘画，线条粗细随压力变化，位置与笔尖对齐（含 HiDPI）；橡皮端应擦除；无笔设备时仅为空画布。

### renderer（每项都应在真实窗口里看一遍）

- [ ] `renderer/01-clear`：背景色随时间平滑渐变；缩放有 letterbox；Esc 或关闭退出。
- [ ] `renderer/02-primitives`：点、线、矩形、填充矩形等图元，窗口缩放后 4:3 内容居中；关闭按钮退出。
- [ ] `renderer/03-lines`：线条图案平滑无断裂，黑边正确。
- [ ] `renderer/04-points`：大量移动的点平滑运动，缩放后居中。
- [ ] `renderer/05-rectangles`：矩形尺寸按时间动画，过渡平滑。
- [ ] `renderer/06-textures`：三份纹理（直接、缩放、居中）显示正确，动画流畅。
- [ ] `renderer/07-streaming-textures`：流式纹理的图案逐帧变化，无撕裂；只有关闭事件会退出。
- [ ] `renderer/08-rotating-textures`：纹理匀速旋转，边缘抗锯齿平滑（取决于 GPU 渲染器）。
- [ ] `renderer/09-scaling-textures`：纹理周期性缩放，保持居中，平滑。
- [ ] `renderer/10-geometry`：彩色三角形的顶点颜色与纹理混合显示，动画平滑；不同 SDL 版本的过滤差异属正常。
- [ ] `renderer/11-color-mods`：纹理颜色随时间循环变化，平滑，无闪烁。
- [ ] `renderer/14-viewport`：三个视口各自显示纹理，边界正确，缩放后保持比例。
- [ ] `renderer/15-cliprect`：裁剪矩形随鼠标/时间移动，只显示矩形内的纹理，边缘无异常。
- [ ] `renderer/17-read-pixels`：旋转的精灵不断显示，读回再绘制的结果与原图一致；真实 GPU/Metal 上无明显卡顿。
- [ ] `renderer/18-debug-text`：调试文本字形清晰、对齐；页脚的时间值随时间变化；非 ASCII 字符显示为占位符方格。
- [ ] `renderer/19-affine-textures`：立方体各面纹理映射正确（不出现扭曲），旋转平滑；缩放黑边正确。
- [ ] `renderer/20-blending`：各混合模式的叠加结果与预期颜色一致；软件渲染器下自定义 screen 混合显示 `[UNSUPPORTED]`，在 Metal/Vulkan/OpenGL 渲染器下应显示真实结果；letterbox 居中。

### storage

- [ ] `storage/01-user`：点击窗口触发保存/读取：窗口应先变蓝，再变绿（成功）或红（失败）；检查用户存储目录里写入了文件；不要在不想污染用户目录时使用 `-autoclick`；在控制台/云存储平台上 `StorageReady` 可能延迟。

## 测试数量与发现的缺陷汇总

| 示例 | 测试数 | 发现的缺陷 |
| --- | --- | --- |
| asyncio/01-load-bitmaps | 12 | 0（间接受 `NewUserdataHandle` 修复影响） |
| audio/01-simple-playback | 9 | 0 |
| audio/02-simple-playback-callback | 10 | 0 |
| audio/03-load-wav | 7 | 0 |
| audio/04-multiple-streams | 10 | 0 |
| audio/05-planar-data | 13 | 3（Escape 退出；声音从未播放；绑定 `PutAudioStreamPlanarDataSlices` 拒绝 nil 平面） |
| camera/01-read-and-draw | 13 | 0 |
| demo/01-snake | 30 | 0 |
| demo/02-woodeneye-008 | 19 | 0 |
| demo/03-infinite-monkeys | 23 | 0 |
| demo/04-bytepusher | 24 | 0 |
| input/01-joystick-polling | 14 | 1（绑定：`NewUserdataHandle` 令牌是小整数而非合法非 Go 指针） |
| input/02-joystick-events | 19 | 1（Escape 退出） |
| input/03-gamepad-polling | 18 | 0 |
| input/04-gamepad-events | 16 | 0 |
| input/05-gamepad-rumble | 10 | 0 |
| misc/01-power | 14 | 1（Escape 退出） |
| misc/02-clipboard | 24 | 0 |
| misc/03-locale | 9 | 0（C 版绘制未初始化缓冲，Go 版已正确处理） |
| pen/01-drawing-lines | 13 | 0 |
| renderer/01-clear | 7 | 0 |
| renderer/02-primitives | 9 | 0 |
| renderer/03-lines | 7 | 0 |
| renderer/04-points | 12 | 0 |
| renderer/05-rectangles | 9 | 0 |
| renderer/06-textures | 8 | 0 |
| renderer/07-streaming-textures | 7 | 0 |
| renderer/08-rotating-textures | 11 | 0 |
| renderer/09-scaling-textures | 9 | 0 |
| renderer/10-geometry | 10 | 0 |
| renderer/11-color-mods | 6 | 0 |
| renderer/14-viewport | 7 | 0 |
| renderer/15-cliprect | 9 | 0 |
| renderer/17-read-pixels | 12 | 0 |
| renderer/18-debug-text | 10 | 0 |
| renderer/19-affine-textures | 9 | 0 |
| renderer/20-blending | 14 | 0 |
| storage/01-user | 15 | 0 |
