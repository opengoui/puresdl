# puresdl

puresdl 是 SDL 3 的纯 Go 绑定（Go module `github.com/opengoui/puresdl`，包名 `sdl`）。

- **无 cgo、无 C 工具链**：基于 [ebitengine/purego](https://github.com/ebitengine/purego) 在运行时 `dlopen` libSDL3，`CGO_ENABLED=0` 即可构建，可直接交叉编译 darwin / linux / windows。
- **自动生成**：`sdl/*_gen.go` 由 `cmd/sdlsync`（内部调用 `cmd/gensdl` 的同一套流水线）从 **`SDL_VERSION` 固定的 SDL 官方 release** 的 C 头文件生成（解析用 `modernc.org/cc/v4`，输出用 `dave/jennifer`），覆盖约 1100 个函数、全部枚举、结构体、联合体、回调类型与常量，并保留头文件里的 doxygen 文档。
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

puresdl 运行时需要 SDL3 动态库。绑定生成自 SDL 官方 release `release-3.4.18` 的头文件（见仓库根的 `SDL_VERSION`），与同版本动态库的导出符号完全一致；更新的库通常也能用。

| 平台 | 安装 | 搜索顺序 |
| --- | --- | --- |
| macOS | `brew install sdl3` | `SDL3_LIBRARY`，`libSDL3.dylib`，`libSDL3.0.dylib`，`/opt/homebrew/lib`，`/usr/local/lib`，`/opt/local/lib`，`SDL3.framework` |
| Linux | `apt install libsdl3-0`（发行版需提供 SDL3；否则自行从源码构建） | `SDL3_LIBRARY`，`libSDL3.so.0`，`libSDL3.so`（先走动态链接器，再试 `/usr/lib/<multiarch>`、`/usr/lib64`、`/usr/local/lib`、`/usr/lib`） |
| Windows | 从 [SDL releases](https://github.com/libsdl-org/SDL/releases) 下载 `SDL3.dll`，放在可执行文件旁或 `PATH` 中 | `SDL3_LIBRARY`，可执行文件所在目录的 `SDL3.dll`，`SDL3.dll`（系统 DLL 搜索顺序） |

也可以设置环境变量 `SDL3_LIBRARY=/path/to/libSDL3.dylib`，或在代码里调用 `sdl.Load(path)`。`sdl.LoadError()` 返回加载失败原因（成功为 nil），`sdl.Available("SDL_Init")` 查询某个符号是否存在于已加载的库中。

## 命名与类型约定

完整说明见 [`sdl/doc.go`](sdl/doc.go)（`go doc ./sdl`）。

- 去掉 `SDL_` 前缀：`SDL_CreateWindow` → `sdl.CreateWindow`，`SDL_Window` → `sdl.Window`，`SDL_INIT_VIDEO` → `sdl.INIT_VIDEO`。键码常量的前缀是 `SDLK_` 而不是 `SDL_`，因此保持原样（`sdl.SDLK_ESCAPE`）。去前缀后若与已有 Go 名字冲突，冲突项会被跳过。
- C 的 `bool`、整数、浮点、枚举映射到同宽度的 Go 类型；不透明句柄是指向空结构体的指针（`*sdl.Window`）。
- 标志位常量带类型：`sdl.WINDOW_RESIZABLE` 是 `sdl.WindowFlags`，`sdl.INIT_VIDEO` 是 `sdl.InitFlags`，`sdl.KMOD_*` 是 `sdl.Keymod`，`sdl.BUTTON_*` 是 `sdl.MouseButtonFlags`。同类型常量可直接 `|`；赋给结构体里的原始整数字段时需要显式转换（如 `uint16(sdl.HAPTIC_SINE)`）。
- `const char *` 参数是 Go `string`；文档允许 NULL 且 NULL 只表示“用默认值”的参数，传空字符串即传 NULL。NULL 与空串语义不同的参数（`SetStringProperty` 的 `value`：NULL 删除属性；`SetAppMetadataProperty`、`SetGamepadMapping`、`InsertTrayEntryAt` 的 `label`；`OpenTitleStorage` 的 `override`：NULL 是默认标题根目录）是 `*string`：`nil` 即 NULL，`sdl.Ptr("")` 是空串。
- `const char *` 返回值被复制成 Go `string`（NULL 变成 `""`）；文档要求调用者 `SDL_free` 的返回值会被复制后自动释放。NULL 表示“未设置”的函数（`GetHint`、`GetEnvironmentVariable`、`Getenv`、`GetenvUnsafe`、`GetAppMetadataProperty`）另有 `GetHintOK` 等 `OK` 变体，返回 `(string, bool)`；原函数签名不变。
- 可空参数与 NULL 结果的分类表在 `internal/bindgen/policy.go`（`nullParams`、`nullDistinctResults`、`nullFailureResults`）；SDL 升级后出现未分类的项时，`gensdl` 与 `go test ./internal/bindgen` 都会失败。
- printf 风格函数（`sdl.SetError`、`sdl.Log` 等）接受 Go 格式串与参数，先用 `fmt.Sprintf` 格式化，再作为字面量交给 SDL，不涉及 C 变参。
- C 联合体（如 `sdl.Event`）是对齐的原始存储，通过访问器方法读取成员：`event.Type()`、`event.Key()`、`event.Window()`…；带指针结果的访问器返回指向联合体内部的指针。
- 返回值是 `SDL_free` 数组的函数返回 Go 切片（已拷贝并释放）。需要自己处理 SDL 分配的数组时有三个泛型辅助：`sdl.GoStrings(p **byte)`（NULL 结尾的 `char **`）、`sdl.GoStringsN(p, n)`（定长 `char **`）和 `sdl.Slice(p *T, n)`（`unsafe.Slice` 的 nil 安全封装，不拷贝）；前两者只拷贝、不释放。
- 文档里写明 SDL 会保留指针参数而不拷贝的函数（`IOFromMem`、`PutAudioStreamDataNoCopy`、`SetRelativeMouseTransform`、`AddTimer` 等）在 godoc 中带有 `Keep alive:` 说明：调用方必须让这块 Go 内存在 SDL 使用期间保持可达（例如放进包级变量）。
- 文档注释由 doxygen 转换而来：`\param`/`\returns` 等被整理成 `Parameters:`/`Returns:` 段落，Markdown 链接、粗体、表格被还原为纯文本或缩进块；文中提到的 SDL 名字会改写成 Go 名字（含手写的 `VERSIONNUM`、`FOURCC` 等宏函数），没有对应 Go 符号的名字写成带反引号的 C 名字。生成器会检查文档里是否出现既未绑定、也不是手写、又不在 `docAllow`（`internal/bindgen/doc.go`）里的 `SDL_` 名字，发现则生成失败并列出。
- 回调 typedef 是 `uintptr` 类型，用对应的 `NewXxx` 构造器从 Go 函数创建（如 `sdl.NewEventFilter`）。purego 每个进程最多约 2000 个回调且从不释放，所以每个回调只创建一次并复用。
- 结构体按值传递的函数（如 `GUID`）由 purego 直接处理；布局由生成的 `layout_gen_test.go` 与 C 前端计算的结果比对。
- 线程：SDL 要求大多数视频与事件函数在主线程运行。macOS 上必须在 `init` 中调用 `runtime.LockOSThread()`。

## SDL 版本与重新生成

仓库根的 `SDL_VERSION` 是单行文件，记录生成绑定所用的 SDL release 标签（当前为 `release-3.4.18`）。`cmd/sdlsync` 按这个标签从 GitHub 下载 `libsdl-org/SDL` 的源码包，**只**解出 `include/SDL3/*` 到本地缓存，再调用与 `cmd/gensdl` 相同的流水线。生成结果不依赖本地 SDL 仓库，也不含日期或路径：同一标签总是产生相同输出。

```bash
go generate ./sdl                        # 按 SDL_VERSION 下载（有缓存则跳过）并重新生成
go run ./cmd/sdlsync -generate           # 等价
go run ./cmd/sdlsync -check              # 重新生成到临时目录，与仓库里的 sdl/*_gen*.go、docs/skipped.txt 比较（CI 用）
go run ./cmd/sdlsync -list               # 列出最近的正式 release 标签
go run ./cmd/sdlsync -latest             # 查询最新正式 release 并同步其头文件（不改 SDL_VERSION）
export SDL_INCLUDE=$(go run ./cmd/sdlsync -print-include)   # 给需要头文件的测试/脚本用
```

`cmd/sdlsync` 参数：

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `-version` | `SDL_VERSION` 的内容 | 目标版本，接受 `3.4.18`、`v3.4.18`、`release-3.4.18`，统一成 `release-3.4.18`；`prerelease-`/`preview-`/`-rc` 版本会被拒绝 |
| `-allow-prerelease` | 否 | 允许预发布标签 |
| `-latest` | 否 | 通过 `https://api.github.com/repos/libsdl-org/SDL/releases/latest` 取最新正式 release（读取 `GITHUB_TOKEN`；触发频率限制时报错并提示设置 token） |
| `-write` | 否 | 解析出的标签与 `SDL_VERSION` 不同时更新该文件 |
| `-list`、`-n` | 否、10 | 打印最近 n 个非预发布标签后退出 |
| `-generate` | 否 | 同步后重新生成 `sdl/*_gen.go`、`sdl/layout_gen_test.go`、`docs/skipped.txt` |
| `-check` | 否 | 重新生成到临时目录，有差异则以差异摘要（新增/删除/变化的文件及首个不同行）失败 |
| `-print-include` | 否 | 在 stdout 打印头文件目录（其余日志都在 stderr） |
| `-offline` | 否 | 只使用缓存，不访问网络 |
| `-cache-dir` | `os.UserCacheDir()/puresdl/sdl` | 缓存目录；每个标签一个子目录，含 `include/SDL3/` 与 `.sync.json`（源码包 sha256、文件数、版本、`SDL_REVISION`） |
| `-github-api-url`、`-download-url` | 官方地址 | 覆盖 API 与源码包地址（环境变量 `SDLSYNC_GITHUB_API_URL`、`SDLSYNC_DOWNLOAD_URL`），测试用 |
| `-root` | `.` | 仓库根（`go generate ./sdl` 用 `-root ..`） |

同步时会校验：`SDL_version.h` 的 `SDL_MAJOR/MINOR/MICRO_VERSION` 必须与标签一致、头文件数量合理、解包只接受 `include/SDL3` 下的普通文件（拒绝 `..`、绝对路径、符号链接、超大文件），并通过临时目录加 rename 原子写入。生成文件头形如 `Code generated by gensdl from the SDL 3.4.18 headers (release-3.4.18, revision SDL-release-3.4.18-0-g829a65d76); DO NOT EDIT.`。GitHub 的 `archive` 源码包里 `SDL_revision.h` 没有写入构建时的版本串，所以 `SDL_REVISION` 取头文件里已有的值，没有时由标签和源码包记录的 commit 推导（`.sync.json` 的 `revision_source` 说明来源）。

### 升级 SDL 版本

1. `go run ./cmd/sdlsync -latest -write -generate`（或 `-version 3.4.20 -write -generate` 指定版本）。
2. 若 `internal/bindgen` 报“未分类的 NULL 参数/结果”或“过期条目”，按提示修改 `policy.go` 里的分类表（`nullParams`、`nullDistinctResults`、`nullFailureResults`）。
3. 修复手写代码里引用了已消失符号的地方；`sdl/macros_test.go` 的期望值是用系统 clang 对该版本头文件算出来的，版本变化时重算并更新注释和 `TestMacroVersion`。
4. `go vet ./... && CGO_ENABLED=0 SDL_VIDEODRIVER=dummy go test ./...`，审查 `docs/skipped.txt` 与 `sdl/*_gen.go` 的 diff，提交。

`.github/workflows/sdl-update.yml` 每周自动执行第 1 步，有新 release 时开 PR；CI 的 `generated` 任务运行 `go run ./cmd/sdlsync -check`，保证仓库里的生成代码与 `SDL_VERSION` 一致。想用本地 SDL 仓库做实验时仍可直接 `go run ./cmd/gensdl -include /path/to/SDL/include -out sdl -skipped docs/skipped.txt`。

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

找不到 libSDL3 时，`sdl` 包的运行时测试会被跳过。需要头文件的生成器测试使用 `sdlsync` 的缓存（先运行一次 `go run ./cmd/sdlsync`）或 `$SDL_INCLUDE`，CI 上找不到则失败。`sdl/abi_test.go` 在已加载的库版本恰好等于 `SDL_VERSION` 时，要求每个已绑定的符号都能 `Available`，用来发现头文件与动态库之间的漂移；版本不同则跳过并说明。

### 平台验证状态

- darwin/arm64：本机完整测试。
- linux/amd64、linux/arm64：在 Docker（Ubuntu 26.04，`libsdl3-0` 3.4.2，`SDL_VIDEODRIVER=dummy`，amd64 为 QEMU 模拟）中跑通 `sdl` 测试与 `examples/hello`。
- windows/amd64、windows/arm64、darwin/amd64：只做交叉编译与 `go vet`（CI 在真机上运行测试，见 `.github/workflows/ci.yml`）。
- 32 位目标（linux/386、linux/arm、windows/386）：包可以编译，但 purego 在 32 位上不支持按值传递结构体，所以 `Load` 直接返回 `sdl.ErrUnsupportedArch`，不会绑定任何函数。

## 已知限制

完整清单见 [docs/skipped.txt](docs/skipped.txt)（当前共 135 项：131 个 libc 替代函数 + 3 个 `va_list` 函数 + 应用自己定义的 `SDL_main`）。

- **C 变参 / `va_list`**：purego 不支持真正的 C 变参。只有以 `const char *fmt, ...` 结尾的 printf 风格函数被特殊处理（在 Go 侧格式化）；其余变参函数不绑定。带 `va_list` 的 `SDL_*V`、`SDL_IOvprintf` 等不绑定。
- **`wchar_t`**：映射为手写的 `sdl.Wchar`（非 Windows 为 `int32`，Windows 为 `uint16`），辅助函数 `WcharString(*Wchar) string` 与 `WcharCString(string) []Wchar` 负责与 Go 字符串互转（含 UTF-16 代理对）。`const wchar_t*` 输入（`HIDOpen` 的 `serial_number`）由包装函数接受 `string`，空串传 NULL；输出缓冲区（`HIDGet*String`）和 `HIDDeviceInfo` 的字符串字段使用 `*Wchar`。
- **libc 替代函数**：`SDL_stdinc.h` 中的字符串、数学、内存拷贝、字符分类等（`SDL_strlen`、`SDL_sin`、`SDL_memcpy`…，约 131 个）在 Go 里有标准库等价物，不绑定；保留的只有 `malloc/free` 系列、环境变量、随机数、CRC/murmur、iconv、断言相关等。
- **仅限特定平台的函数**：一些函数只在部分平台的头文件里声明（仅 Windows 的 `SDL_RegisterApp`、`SDL_SetWindowsMessageHook`、`SDL_GetDXGIOutputInfo`，仅 Linux 的 `SDL_SetLinuxThreadPriority`、`SDL_IsUbuntuTouch` 等）。三个目标（darwin/arm64、linux/amd64、windows/amd64）取并集，其文档会注明 “Only declared by the SDL headers for …”；在其他平台调用会因符号缺失而 panic。
- **32 位目标**：见上文“平台验证状态”，只能编译，不能使用。
- **GUID 按值传递**：purego 在 amd64 上会错误分类“只含一个数组字段的 16 字节结构体”，所以 `SDL_GUID` 在 C 边界上用等价的 `guidABI`（两个 `uint64`）传递，对外 API 仍是 `sdl.GUID`。
- **回调上限**：purego 每进程约 2000 个回调，且不能释放。Windows 上回调也不支持结构体参数。
- **库版本**：头文件固定为 SDL release 3.4.18，并用同版本的 Homebrew 动态库验证过：1115 个绑定符号全部存在（`sdl/abi_test.go`）。运行时用更旧的库时，缺失的函数仍有 Go 签名，但调用会 panic 并提示 `sdl: SDL_Xxx is not available in the loaded SDL3 library`；用 `sdl.Available(...)` 提前检测，或用 `sdl.GetVersion()` 判断版本。3.5 才有的新 API（通知、传感器扩展等）在升级到对应 release 之前不提供。
- `SDL_main` 式的回调入口（`SDL_RunApp`、`SDL_EnterAppMainCallbacks`）虽然有绑定，但在 Go 程序里不适用，请自己写 `main`。

## 目录结构

```
puresdl/
├── SDL_VERSION            固定的 SDL release 标签（如 release-3.4.18）
├── cmd/gensdl/            生成器命令行入口（使用本地头文件目录）
├── cmd/sdlsync/           按 SDL_VERSION 同步头文件、生成、校验
├── internal/
│   ├── ccfront/           C 头文件前端：cc/v4 解析 + 多目标合并 -> ir
│   │   └── data/          内嵌：各目标预定义宏、libc 桩头文件
│   ├── gensdl/            生成流水线（gensdl 与 sdlsync 共用）
│   ├── sdlsync/           release 标签解析、GitHub API、头文件缓存、-check
│   ├── ir/                与目标无关的 C API 中间表示
│   └── bindgen/           ir -> Go 源码（jennifer）
├── sdl/                   生成的绑定包 + 手写的 loader*.go、doc.go、测试
├── examples/hello/        最小可运行示例
└── docs/
    ├── architecture.md    生成器架构
    ├── puregotk-analysis.md  对 puregotk（GIR 生成器）的分析与对比
    └── skipped.txt        未绑定声明清单（由生成器写出）
```
