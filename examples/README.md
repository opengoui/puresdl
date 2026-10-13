# 示例

本目录镜像 SDL 官方仓库的 `SDL/examples/<类别>/<编号-名称>/`：每个 C 示例在这里都有同名目录，里面是用 `puresdl` 重写的 `main.go`。目录名与 SDL 保持一致，没有其他示例目录。

所有示例都支持 `-frames N`：运行 N 帧后自动退出，便于无头验证。

## 示例列表

| Go 目录 | C 源文件 | 硬件/资源/备注 |
| --- | --- | --- |
| `examples/asyncio/01-load-bitmaps` | `SDL/examples/asyncio/01-load-bitmaps/load-bitmaps.c` | 内嵌位图资源（go:embed） |
| `examples/audio/01-simple-playback` | `SDL/examples/audio/01-simple-playback/simple-playback.c` | 需要音频设备（dummy 驱动可无头） |
| `examples/audio/02-simple-playback-callback` | `SDL/examples/audio/02-simple-playback-callback/simple-playback-callback.c` | 需要音频设备（dummy 驱动可无头） |
| `examples/audio/03-load-wav` | `SDL/examples/audio/03-load-wav/load-wav.c` | 需要音频设备（dummy 驱动可无头） |
| `examples/audio/04-multiple-streams` | `SDL/examples/audio/04-multiple-streams/multiple-streams.c` | 需要音频设备（dummy 驱动可无头） |
| `examples/audio/05-planar-data` | `SDL/examples/audio/05-planar-data/planar-data.c` | 需要音频设备（dummy 驱动可无头） |
| `examples/camera/01-read-and-draw` | `SDL/examples/camera/01-read-and-draw/read-and-draw.c` | 需要摄像头（dummy 驱动可无头） |
| `examples/demo/01-snake` | `SDL/examples/demo/01-snake/snake.c` | 无特殊要求 |
| `examples/demo/02-woodeneye-008` | `SDL/examples/demo/02-woodeneye-008/woodeneye-008.c` | 无特殊要求 |
| `examples/demo/03-infinite-monkeys` | `SDL/examples/demo/03-infinite-monkeys/infinite-monkeys.c` | 无特殊要求 |
| `examples/demo/04-bytepusher` | `SDL/examples/demo/04-bytepusher/bytepusher.c` | 无特殊要求 |
| `examples/input/01-joystick-polling` | `SDL/examples/input/01-joystick-polling/joystick-polling.c` | 需要手柄/摇杆（无设备时仅显示空状态） |
| `examples/input/02-joystick-events` | `SDL/examples/input/02-joystick-events/joystick-events.c` | 需要手柄/摇杆（无设备时仅显示空状态） |
| `examples/input/03-gamepad-polling` | `SDL/examples/input/03-gamepad-polling/gamepad-polling.c` | 需要手柄/摇杆（无设备时仅显示空状态） |
| `examples/input/04-gamepad-events` | `SDL/examples/input/04-gamepad-events/gamepad-events.c` | 需要手柄/摇杆（无设备时仅显示空状态） |
| `examples/input/05-gamepad-rumble` | `SDL/examples/input/05-gamepad-rumble/gamepad-rumble.c` | 需要支持震动的手柄 |
| `examples/misc/01-power` | `SDL/examples/misc/01-power/power.c` | 无特殊要求 |
| `examples/misc/02-clipboard` | `SDL/examples/misc/02-clipboard/clipboard.c` | 读写系统剪贴板 |
| `examples/misc/03-locale` | `SDL/examples/misc/03-locale/locale.c` | 无特殊要求 |
| `examples/pen/01-drawing-lines` | `SDL/examples/pen/01-drawing-lines/drawing-lines.c` | 需要笔/数位板输入（无设备时空画布） |
| `examples/renderer/01-clear` | `SDL/examples/renderer/01-clear/clear.c` | 无特殊要求 |
| `examples/renderer/02-primitives` | `SDL/examples/renderer/02-primitives/primitives.c` | 无特殊要求 |
| `examples/renderer/03-lines` | `SDL/examples/renderer/03-lines/lines.c` | 无特殊要求 |
| `examples/renderer/04-points` | `SDL/examples/renderer/04-points/points.c` | 无特殊要求 |
| `examples/renderer/05-rectangles` | `SDL/examples/renderer/05-rectangles/rectangles.c` | 无特殊要求 |
| `examples/renderer/06-textures` | `SDL/examples/renderer/06-textures/textures.c` | 内嵌 PNG 资源（go:embed） |
| `examples/renderer/07-streaming-textures` | `SDL/examples/renderer/07-streaming-textures/streaming-textures.c` | 无特殊要求 |
| `examples/renderer/08-rotating-textures` | `SDL/examples/renderer/08-rotating-textures/rotating-textures.c` | 内嵌 PNG 资源（go:embed） |
| `examples/renderer/09-scaling-textures` | `SDL/examples/renderer/09-scaling-textures/scaling-textures.c` | 内嵌 PNG 资源（go:embed） |
| `examples/renderer/10-geometry` | `SDL/examples/renderer/10-geometry/geometry.c` | 内嵌 PNG 资源（go:embed） |
| `examples/renderer/11-color-mods` | `SDL/examples/renderer/11-color-mods/color-mods.c` | 无特殊要求 |
| `examples/renderer/14-viewport` | `SDL/examples/renderer/14-viewport/viewport.c` | 无特殊要求 |
| `examples/renderer/15-cliprect` | `SDL/examples/renderer/15-cliprect/cliprect.c` | 无特殊要求 |
| `examples/renderer/17-read-pixels` | `SDL/examples/renderer/17-read-pixels/read-pixels.c` | 无特殊要求 |
| `examples/renderer/18-debug-text` | `SDL/examples/renderer/18-debug-text/debug-text.c` | 无特殊要求 |
| `examples/renderer/19-affine-textures` | `SDL/examples/renderer/19-affine-textures/affine-textures.c` | 内嵌 PNG 资源（go:embed） |
| `examples/renderer/20-blending` | `SDL/examples/renderer/20-blending/blending.c` | 无特殊要求 |
| `examples/storage/01-user` | `SDL/examples/storage/01-user/user.c` | 读写用户存储目录 |

## 运行

```bash
go run ./examples/renderer/02-primitives
go run ./examples/demo/01-snake
```

## 无头运行

使用 SDL 的 dummy 驱动，不需要显示器、声卡或摄像头：

```bash
SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy SDL_CAMERA_DRIVER=dummy \
  go run ./examples/renderer/02-primitives -frames 30
```

CI 与本地验证都用这种方式把每个示例跑 30 帧并要求退出码为 0。

## 测试

每个示例都有自动化交互测试；测试方法、无法自动覆盖的内容和逐示例的手动检查清单见 [TESTING.md](TESTING.md)。
