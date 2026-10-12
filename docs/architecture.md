# 生成器架构

`sdl/*_gen.go` 由 `go run ./cmd/gensdl` 产生，流水线分三层：

```
SDL3 C 头文件
   │  internal/ccfront   （modernc.org/cc/v4，多目标解析并合并）
   ▼
internal/ir              （与目标无关的 C API 描述：类型、布局、枚举值、常量、文档）
   │  internal/bindgen   （命名、类型映射、包装规则，dave/jennifer 输出）
   ▼
sdl/*_gen.go  bind_gen.go  layout_gen_test.go
```

`cmd/gensdl/main.go` 只做编排：`ccfront.Load` → 打印 warnings → 删除旧的 `*_gen.go` / `*_gen_test.go` → `bindgen.Generate` → 可选写 `-skipped` 清单。

## 1. ccfront：C 头文件前端

目标是在没有 C 编译器、没有 SDK 的机器上也能生成（CI 可在任何系统上运行）。

- **内嵌数据**（`internal/ccfront/data`，`//go:embed`）：
  - `predef/<triple>.h`：用 `clang --target=<triple> -dM -E` 预先抓取的预定义宏，每个目标一份（`arm64-apple-darwin`、`x86_64-unknown-linux-gnu`、`x86_64-pc-windows-msvc`，另有 i386 linux）。`Load` 把它作为 `<predefined>` 源文件喂给 cc/v4，从而决定 `#ifdef __APPLE__`、`_WIN32` 等分支。
  - `stubs/`、`stubs_win/`：最小化的 libc 桩头文件（`stdint.h`、`stddef.h`、`stdbool.h`、`stdarg.h`、`wchar.h`、`string.h`…，Windows 版另有 `sal.h` 等空头文件）。SDL 头文件 `#include` 它们时只需要类型定义，不需要真实实现。
  - Windows 目标额外追加 `__cdecl`、`__stdcall`、`__declspec(x)` 等为空的宏。
  - 运行时先把 `data` 解压到临时目录，再作为 `SysIncludePaths` 传给 cc/v4。
- **入口源码**是一个虚拟的 `umbrella.c`：`#include <SDL3/SDL.h>`、`SDL_main.h`、`SDL_vulkan.h`。
- **多目标合并**（`ccfront.Targets`）：darwin/arm64、linux/amd64、windows/amd64 各解析一次。
  - 第一个目标（darwin/arm64）提供类型布局和常量；
  - 函数按名字合并，每个 `ir.Function.Targets` 记录声明它的 `goos/goarch`（只在部分平台声明的函数，生成的文档里会注明）；
  - 类型布局在目标之间不同则产生 warning。
- **过滤**：`excluded`（`SDL_oldnames.h`、`SDL_egl.h`、`SDL_intrin.h`、`SDL_main_impl.h`、`SDL_begin_code.h`…）、`SDL_opengl*`、`SDL_test*` 不属于 libSDL3 的 ABI，被排除；静态/内联函数、`_` 开头的名字被忽略。
- **常量**：`cc.Config.EvalAllMacros` 让 cc/v4 求值所有对象式宏，整数/浮点/字符串值转成 `ir.Const`（`SDL_LINE/FILE/FUNCTION` 除外）；`SDL_MAJOR/MINOR/MICRO_VERSION` 推出 `API.Version`。
- **文档**（`docs.go`）：取每个声明上方的 doxygen 块，抽出 `\param` 文本（`bindgen` 靠它判断参数是否允许 NULL）。

## 2. ir：中间表示

`internal/ir` 里只有数据结构：`Type`（`Void/Bool/Int/Float/Pointer/Array/Func/Struct/Union/Enum/Named`）、`Record`、`EnumDecl`、`Typedef`、`Function`、`Const`、`API`。关键点是 **ABI 事实由 C 前端算好**：`Type.Size/Align`、`Field.Offset`、枚举值、`Record.Size`。bindgen 不猜布局。聚合与 typedef 节点是共享的，可以按指针比较是否同一声明。

## 3. bindgen：Go 代码生成

### 命名（`names.go`）

- 类型、函数、常量、枚举成员：去掉 `SDL_` 前缀并首字母大写（`SDL_CreateWindow` → `CreateWindow`，`SDL_INIT_VIDEO` → `INIT_VIDEO`）。`Uint32` 之类内建 typedef 直接映射成 Go 内建类型。
- 结构体字段：`snake_case` → `CamelCase`，数字开头加 `F`；重名加 `_`。
- 参数名：Go 关键字或会遮蔽内建标识符的名字加 `_` 后缀（`string_`），空名用 `p<i>`。
- 用 `claim(goName, cName)` 登记，Go 名字冲突时跳过后来者并写入 skipped 清单。
- 文件名：`SDL_video.h` → `video_gen.go`，保持与头文件一一对应。
- 匿名嵌套结构体按 `父名_字段名` 命名。

### 类型映射（`types.go`）

| C | Go |
| --- | --- |
| `bool` | `bool` |
| 整数 | 按宽度与符号取 `int8…uint64`；`char` 是 `byte` |
| `float/double` | `float32/float64` |
| `Uint8…Sint64`、`size_t`、`intptr_t` 等 | 对应 Go 内建类型 |
| `enum` | 具名 Go 整数类型 + `const` 块 |
| 指针 | `*T`；`void *` → `unsafe.Pointer`；`char *` → `*byte`；函数指针 → `uintptr`；未输出的结构体 → `unsafe.Pointer` |
| 数组 | `[N]T` |
| 不透明结构体（`typedef struct X X;`） | `type X struct{}`，句柄是 `*X` |
| 标量 typedef | 具名 Go 类型（`type WindowFlags uint64`） |
| `va_list` | 不支持（记入 skipped） |
| `wchar_t` | `Wchar`（手写，`sdl/wchar*.go`：非 Windows `int32`，Windows `uint16`）；`const wchar_t*` 输入参数在包装函数里为 `string`（空串即 NULL，`wcstrNull`），其余为 `*Wchar` |

### 带类型的常量（`policy.go: constType`）

宏常量默认无类型，但 SDL 的标志位（`SDL_WindowFlags`、`SDL_InitFlags`、`SDL_Keymod`…）都有对应的整数 typedef。`ccfront` 在 `ir.Const` 记录 `#define` 的偏移，并在 `ir.Typedef` 记录 typedef 偏移；若某个整数宏与它之前最近的 typedef 之间（同一头文件）只有预处理指令、注释和空行，就把 typedef 名写入 `Const.After`。`bindgen` 据此输出 `Name TypedefName = value`，条件是：

- 该 typedef 被输出为具名 Go 类型（`Uint32` 等内建 typedef 不算，所以 `SDL_MAX_UINT32` 保持无类型）；
- 底层是整数（枚举、结构体 typedef 不算），值能放进该类型（`SDL_HAPTIC_GAIN` 等超出 `uint16` 的值保持无类型）；
- 常量之间做 `|` 仍然合法，因为同类型常量表达式的结果还是该类型。

`constTypeOverrides` 是人工核对头文件后的例外表：`SDL_WINDOWPOS_*` 是 `int` 坐标，保持无类型；`SDL_PEN_MOUSEID`、`SDL_TOUCH_MOUSEID` 等在头文件里被强转为 `SDL_MouseID`/`SDL_TouchID`；`SDL_BUTTON_*` 显式归入 `MouseButtonFlags`。`gensdl` 会打印被加类型的常量总数。需要把带类型常量赋给结构体里的原始整数字段（如 `HapticPeriodic.Type` 是 `uint16`）时，需要显式转换。

### 函数包装规则（`funcs.go`）

每个 C 函数生成一个未导出的 `var fnXxx func(...)`（原始签名，由 `bindAll` 在加载库后 `register` 绑定）和一个导出的包装函数：

- `const char *` 参数 → `string`，用 `cstr` 转成以 NUL 结尾的拷贝；`\param` 文档里出现 `NULL` 时必须在 `policy.go: nullParams` 里分类（未分类则生成失败）：NULL 只表示默认值的用 `cstrNull`（空串传 NULL，文档加注）；NULL 与空串语义不同的（`distinct`）参数公开类型为 `*string`，用 `cstrPtr`（`nil` → NULL，`sdl.Ptr("")` → 空串）。
- `char *` / `const char *` 返回 → 拷贝成 `string`（`GoString`，NULL 变 `""`）；`\returns` 提到 NULL 的结果同样必须在 `nullDistinctResults`（NULL 表示“未设置”，额外生成 `<Name>OK` 返回 `(string, bool)`，原函数不变）或 `nullFailureResults`（仅表示失败/无此项，`""` 足够）中分类。`CheckNullPolicy` 会在 `Generate` 和 `internal/bindgen` 测试里校验分类表既不缺项也无过期项；文档里提到 `SDL_free` 的非 const 返回值，拷贝后调用 `Free`。
- 以 `const char *fmt, ...` 结尾的 printf 风格变参函数 → `(format string, args ...any)`，经 `cprintf` 做 `fmt.Sprintf` 并把 `%` 转义成 `%%`，作为字面量交给 SDL；其余变参函数、带 `va_list` 的函数、`SDL_stdinc.h` 里非保留名单（`policy.go` 的 `keptStdinc`）的函数一律跳过，原因写入 skipped。
- 其余参数/返回值直接用映射类型；结构体按值传递交给 purego。
- 只在少于 3 个目标声明的函数，在文档加 “Only declared by the SDL headers for …”。
- 文档转换在 `doc.go`：先按段落预处理（代码块与 Markdown 表格 → 缩进的预格式块，`[文本](Category…)` 链接、`**粗体**`、HTML 实体、行内 `\p \c \a \ref \b \e \em \sa`、行首 `@tag` 还原为纯文本），再按出现顺序输出 `Parameters:` / `Returns:` / `Note:` / `Thread safety:` / `Since:` / `See also:` 段落（`\returns` 之后的正文段落留在原位，不再被提前或缩进成预格式文本），并折叠连续空注释行。
- 文档里的 C 标识符通过 `docNames`（已绑定）和 `Options.HandDir`/输出目录里手写文件的导出名（如 `VERSIONNUM`）改写成 Go 名字；其余 `SDL_` 名字写成带反引号的 C 名字。`checkDocNames` 在生成结束时校验：这些名字必须是被跳过的声明或在 `docAllow` 里，否则生成失败并列出全部违规名字（`SDL_HINT_*` 这类通配前缀、`SDL_xxx.h` 文件名不算）。
- 文档（函数文档或参数文档）里出现 `not copied`、`NoCopy`、`must remain/stay valid`、`keeps a reference` 等表示 SDL 保留指针的措辞，且函数有指针参数时，在文档加 `Keep alive:` 说明（`funcs.go: keepAliveNote`）。

### 加载与缺失符号（手写，`sdl/loader*.go`）

`register(fptr, symbol)` 用 `Dlsym`（Windows 用 `syscall.GetProcAddress`）找符号；找不到则安装一个会 panic 并给出明确信息的桩函数，并记录到 `missing`，`Available()` 据此回答。这样 3.5 头文件生成的绑定可以对 3.4 动态库工作。

### 回调（`decls.go: emitCallback`）

函数指针 typedef 生成 `type Xxx uintptr` 与构造器 `NewXxx(fn func(...)) Xxx`，内部调用 `purego.NewCallback`。参数/返回类型无法映射时跳过构造器。purego 每进程最多约 2000 个回调且不能释放，文档里提示每个回调只创建一次。

### 联合体（`emitUnion`）

联合体输出为 `struct{ data [N]uintN }`，其中 `uintN` 取联合体对齐宽度以保证对齐，再为每个成员生成：

- 标量成员：`Name()` getter 与 `SetName(v)` setter；
- 结构体/联合体/数组成员：`Name()` 返回指向内部存储的指针（别名）。

`sdl.Event` 就是这样：`event.Type()`、`event.Key()`、`event.Window()`…

### 结构体与布局测试

普通结构体按 C 字段顺序输出；某个字段类型不受支持时只降级该字段：指针字段变成 `unsafe.Pointer`，其他类型变成同尺寸同对齐的不透明数组，并记入 skipped；结构体其余字段保持类型化。每个输出的类型都在 `layout_gen_test.go` 里记录期望的 `Sizeof/Alignof` 与每个字段的 `Offsetof`，`TestGeneratedLayout` 把 Go 布局与 cc/v4 算出的 C 布局逐项比对，保证按值/按指针传递时与 C 一致。

### 输出

用 jennifer 构造 AST，`go/format` 格式化，文件头 `Code generated by gensdl from the SDL <version> headers; DO NOT EDIT.`。每个头文件一个 `<name>_gen.go`（类型 → 常量 → 函数），外加 `bind_gen.go`（`bindAll` 与 `allBindings`）和 `layout_gen_test.go`。

## 4. 如何扩展

- **新增一类映射**（例如 `wchar_t`）：在 `types.go` 的 `builtinNames` 里映射到手写类型（或从 `unsupportedNames` 移除），并在 `bindFunc` 里加转换规则。`wchar_t` 的宽度随平台而异（ccfront 按 darwin/linux 为 4 字节、windows 为 2 字节），所以 `Wchar` 由带构建标签的手写文件定义；生成的布局只在 `wchar_t` 指向的元素宽度上不同，指针与结构体大小一致，`layout_gen_test.go` 在各平台都成立。
- **保留更多 `SDL_stdinc.h` 函数**：编辑 `policy.go` 的 `keptStdinc` 正则。
- **新增目标平台**：在 `ccfront.Targets` 加一项，并在 `data/predef/` 放对应 triple 的预定义宏（`clang --target=<triple> -dM -E - </dev/null`）；若该平台需要不同的 libc 桩，参照 `stubs_win` 另建目录并在 `translate` 里选择。同时在 `sdl/loader_*.go` 添加库搜索路径。
- **增加包装规则**：在 `funcs.go: bindFunc` 的参数循环里增加新的 `argPlan`（参数类型、原始类型、转换表达式）；返回值同理。
- **排除头文件**：`front.go` 的 `excluded` 与 `headerExcluded`。
- 改完生成器后：`go run ./cmd/gensdl -include <SDL/include> -out sdl -skipped docs/skipped.txt`，运行 `CGO_ENABLED=0 SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy go test ./...`，并检查 `docs/skipped.txt` 的 diff。
