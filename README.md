# puresdl

puresdl 是 SDL 3 的纯 Go 绑定（Go module `github.com/opengoui/puresdl`，包名 `sdl`）。

- **无 cgo、无 C 工具链**：基于 [ebitengine/purego](https://github.com/ebitengine/purego) 在运行时 `dlopen` libSDL3，`CGO_ENABLED=0` 即可构建，可直接交叉编译 darwin / linux / windows。
- **自动生成**：`sdl/*_gen.go` 由 `cmd/gensdl` 从 SDL3 C 头文件生成（解析用 `modernc.org/cc/v4`，输出用 `dave/jennifer`），覆盖约 1300 个函数、全部枚举、结构体、联合体、回调类型与常量，并保留头文件里的 doxygen 文档。
- **宽容加载**：导入包不会因为缺少 SDL3 而 panic；调用到当前动态库没有导出的新函数时才 panic，并给出明确提示。

## 快速开始

先安装 SDL3（见下节），然后：

```go
package main

import (
	"fmt"
	"runtime"

	"github.com/opengoui/puresdl/sdl"
)

func init() {
	// SDL 的视频/事件函数必须在进程主线程调用（macOS 强制要求）。
	// 在 init 里锁定后，main.main 就运行在主线程上。
	runtime.LockOSThread()
}

func main() {
	if err := sdl.LoadError(); err != nil {
		panic(err) // 找不到 libSDL3
	}
	if !sdl.Init(sdl.INIT_VIDEO) {
		panic(sdl.GetError())
	}
	defer sdl.Quit()

	var window *sdl.Window
	var renderer *sdl.Renderer
	if !sdl.CreateWindowAndRenderer("hello", 640, 480, sdl.WINDOW_RESIZABLE, &window, &renderer) {
		panic(sdl.GetError())
	}
	defer sdl.DestroyWindow(window)
	defer sdl.DestroyRenderer(renderer)

	var event sdl.Event
	for running := true; running; {
		for sdl.PollEvent(&event) {
			if sdl.EventType(event.Type()) == sdl.EVENT_QUIT {
				running = false
			}
		}
		sdl.SetRenderDrawColor(renderer, 30, 120, 220, 255)
		sdl.RenderClear(renderer)
		sdl.RenderPresent(renderer)
		sdl.Delay(16)
	}
	fmt.Println("bye")
}
```

仓库自带一个更完整的示例 [`examples/hello`](examples/hello/main.go)：颜色随时间渐变，按 Esc 或关闭窗口退出，并支持 `-frames N`（运行 N 帧后退出），因此可以无头运行：

```bash
go run ./examples/hello                                   # 打开窗口
SDL_VIDEODRIVER=dummy go run ./examples/hello -frames 30  # 无头：不开窗口，30 帧后退出
```

## 安装 SDL3

puresdl 运行时需要 SDL3 动态库（本仓库按 3.4.x 动态库测试，头文件取自 3.5.0）。

| 平台 | 安装 | 搜索顺序 |
| --- | --- | --- |
| macOS | `brew install sdl3` | `SDL3_LIBRARY`，`libSDL3.dylib`，`libSDL3.0.dylib`，`/opt/homebrew/lib`，`/usr/local/lib`，`/opt/local/lib`，`SDL3.framework` |
| Linux | `apt install libsdl3-0`（发行版需提供 SDL3；否则自行从源码构建） | `SDL3_LIBRARY`，`libSDL3.so.0`，`libSDL3.so` |
| Windows | 从 [SDL releases](https://github.com/libsdl-org/SDL/releases) 下载 `SDL3.dll`，放在可执行文件旁或 `PATH` 中 | `SDL3_LIBRARY`，`SDL3.dll` |

也可以设置环境变量 `SDL3_LIBRARY=/path/to/libSDL3.dylib`，或在代码里调用 `sdl.Load(path)`。`sdl.LoadError()` 返回加载失败原因（成功为 nil），`sdl.Available("SDL_Init")` 查询某个符号是否存在于已加载的库中。

## 命名与类型约定

完整说明见 [`sdl/doc.go`](sdl/doc.go)（`go doc ./sdl`）。

- 去掉 `SDL_` 前缀：`SDL_CreateWindow` → `sdl.CreateWindow`，`SDL_Window` → `sdl.Window`，`SDL_INIT_VIDEO` → `sdl.INIT_VIDEO`。键码常量的前缀是 `SDLK_` 而不是 `SDL_`，因此保持原样（`sdl.SDLK_ESCAPE`）。去前缀后若与已有 Go 名字冲突，冲突项会被跳过。
- C 的 `bool`、整数、浮点、枚举映射到同宽度的 Go 类型；不透明句柄是指向空结构体的指针（`*sdl.Window`）。
- 标志位常量带类型：`sdl.WINDOW_RESIZABLE` 是 `sdl.WindowFlags`，`sdl.INIT_VIDEO` 是 `sdl.InitFlags`，`sdl.KMOD_*` 是 `sdl.Keymod`，`sdl.BUTTON_*` 是 `sdl.MouseButtonFlags`。同类型常量可直接 `|`；赋给结构体里的原始整数字段时需要显式转换（如 `uint16(sdl.HAPTIC_SINE)`）。
- `const char *` 参数是 Go `string`；文档允许 NULL 且 NULL 只表示“用默认值”的参数，传空字符串即传 NULL。NULL 与空串语义不同的参数（`SetStringProperty` 的 `value`：NULL 删除属性；`SetAppMetadataProperty`、`SetGamepadMapping`、`InsertTrayEntryAt` 的 `label`）是 `*string`：`nil` 即 NULL，`sdl.Ptr("")` 是空串。
- `const char *` 返回值被复制成 Go `string`（NULL 变成 `""`）；文档要求调用者 `SDL_free` 的返回值会被复制后自动释放。NULL 表示“未设置”的函数（`GetHint`、`GetEnvironmentVariable`、`Getenv`、`GetenvUnsafe`、`GetAppMetadataProperty`）另有 `GetHintOK` 等 `OK` 变体，返回 `(string, bool)`；原函数签名不变。
- 可空参数与 NULL 结果的分类表在 `internal/bindgen/policy.go`（`nullParams`、`nullDistinctResults`、`nullFailureResults`）；SDL 升级后出现未分类的项时，`gensdl` 与 `go test ./internal/bindgen` 都会失败。
- printf 风格函数（`sdl.SetError`、`sdl.Log` 等）接受 Go 格式串与参数，先用 `fmt.Sprintf` 格式化，再作为字面量交给 SDL，不涉及 C 变参。
- C 联合体（如 `sdl.Event`）是对齐的原始存储，通过访问器方法读取成员：`event.Type()`、`event.Key()`、`event.Window()`…；带指针结果的访问器返回指向联合体内部的指针。
- 返回值是 `SDL_free` 数组的函数返回 Go 切片（已拷贝并释放）。需要自己处理 SDL 分配的数组时有三个泛型辅助：`sdl.GoStrings(p **byte)`（NULL 结尾的 `char **`）、`sdl.GoStringsN(p, n)`（定长 `char **`）和 `sdl.Slice(p *T, n)`（`unsafe.Slice` 的 nil 安全封装，不拷贝）；前两者只拷贝、不释放。
- 文档里写明 SDL 会保留指针参数而不拷贝的函数（`IOFromMem`、`PutAudioStreamDataNoCopy`、`SetScancodeName` 等）在 godoc 中带有 `Keep alive:` 说明：调用方必须让这块 Go 内存在 SDL 使用期间保持可达（例如放进包级变量）。
- 文档注释由 doxygen 转换而来：`\param`/`\returns` 等被整理成 `Parameters:`/`Returns:` 段落，Markdown 链接、粗体、表格被还原为纯文本或缩进块；文中提到的 SDL 名字会改写成 Go 名字（含手写的 `VERSIONNUM`、`FOURCC` 等宏函数），没有对应 Go 符号的名字写成带反引号的 C 名字。生成器会检查文档里是否出现既未绑定、也不是手写、又不在 `docAllow`（`internal/bindgen/doc.go`）里的 `SDL_` 名字，发现则生成失败并列出。
- 回调 typedef 是 `uintptr` 类型，用对应的 `NewXxx` 构造器从 Go 函数创建（如 `sdl.NewEventFilter`）。purego 每个进程最多约 2000 个回调且从不释放，所以每个回调只创建一次并复用。
- 结构体按值传递的函数（如 `GUID`）由 purego 直接处理；布局由生成的 `layout_gen_test.go` 与 C 前端计算的结果比对。
- 线程：SDL 要求大多数视频与事件函数在主线程运行。macOS 上必须在 `init` 中调用 `runtime.LockOSThread()`。

## 重新生成

```bash
go generate ./sdl
# 等价于
go run ./cmd/gensdl -include /path/to/SDL/include -out sdl -skipped docs/skipped.txt
```

`cmd/gensdl` 参数：

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `-include` | `$SDL_INCLUDE` | 含有 `SDL3/SDL.h` 的目录（必填） |
| `-out` | `sdl` | 输出 Go 包目录；运行前会删除旧的 `*_gen.go` 与 `*_gen_test.go` |
| `-skipped` | 空 | 把未绑定声明的清单写到该文件 |

生成过程不需要 C 编译器：宏定义与 libc 桩头文件都内嵌在 `internal/ccfront/data`。生成器的结构见 [docs/architecture.md](docs/architecture.md)。不要手改 `*_gen.go`，改生成器后重新生成。

测试需要真实的 libSDL3，使用 dummy 驱动：

```bash
CGO_ENABLED=0 SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy go test ./...
```

找不到 libSDL3 时，`sdl` 包的运行时测试会被跳过。

## 已知限制

完整清单见 [docs/skipped.txt](docs/skipped.txt)（当前共 138 项：135 个 libc 替代函数 + 3 个 `va_list` 函数）。

- **C 变参 / `va_list`**：purego 不支持真正的 C 变参。只有以 `const char *fmt, ...` 结尾的 printf 风格函数被特殊处理（在 Go 侧格式化）；其余变参函数不绑定。带 `va_list` 的 `SDL_*V`、`SDL_IOvprintf` 等不绑定。
- **`wchar_t`**：映射为手写的 `sdl.Wchar`（非 Windows 为 `int32`，Windows 为 `uint16`），辅助函数 `WcharString(*Wchar) string` 与 `WcharCString(string) []Wchar` 负责与 Go 字符串互转（含 UTF-16 代理对）。`const wchar_t*` 输入（`HIDOpen` 的 `serial_number`）由包装函数接受 `string`，空串传 NULL；输出缓冲区（`HIDGet*String`）和 `HIDDeviceInfo` 的字符串字段使用 `*Wchar`。
- **libc 替代函数**：`SDL_stdinc.h` 中的字符串、数学、内存拷贝、字符分类等（`SDL_strlen`、`SDL_sin`、`SDL_memcpy`…，约 135 个）在 Go 里有标准库等价物，不绑定；保留的只有 `malloc/free` 系列、环境变量、随机数、CRC/murmur、iconv、断言相关等。
- **仅限特定平台的函数**：一些函数只在部分平台的头文件里声明（仅 Windows 的 `SDL_RegisterApp`、`SDL_SetWindowsMessageHook`、`SDL_GetDXGIOutputInfo`，仅 Linux 的 `SDL_SetLinuxThreadPriority`、`SDL_IsUbuntuTouch` 等）。三个目标（darwin/arm64、linux/amd64、windows/amd64）取并集，其文档会注明 “Only declared by the SDL headers for …”；在其他平台调用会因符号缺失而 panic。
- **回调上限**：purego 每进程约 2000 个回调，且不能释放。Windows 上回调也不支持结构体参数。
- **SDL 3.4 与 3.5**：头文件是 3.5.0，但常见发行版动态库是 3.4.x，约 30 个新函数在旧库里不存在（用 3.4.18 实测：1147 个绑定中 32 个缺失，其中 `SDL_main`、`SDL_IsUbuntuTouch` 这类只属于特定平台或构建的符号也算在内）。它们仍有 Go 签名，但调用时 panic 并提示 `sdl: SDL_Xxx is not available in the loaded SDL3 library`；用 `sdl.Available(...)` 提前检测，或用 `sdl.GetVersion()` 判断版本。
- `SDL_main` 式的回调入口（`SDL_RunApp`、`SDL_EnterAppMainCallbacks`）虽然有绑定，但在 Go 程序里不适用，请自己写 `main`。

## 目录结构

```
puresdl/
├── cmd/gensdl/            生成器命令行入口
├── internal/
│   ├── ccfront/           C 头文件前端：cc/v4 解析 + 多目标合并 -> ir
│   │   └── data/          内嵌：各目标预定义宏、libc 桩头文件
│   ├── ir/                与目标无关的 C API 中间表示
│   └── bindgen/           ir -> Go 源码（jennifer）
├── sdl/                   生成的绑定包 + 手写的 loader*.go、doc.go、测试
├── examples/hello/        最小可运行示例
└── docs/
    ├── architecture.md    生成器架构
    ├── puregotk-analysis.md  对 puregotk（GIR 生成器）的分析与对比
    └── skipped.txt        未绑定声明清单（由生成器写出）
```
