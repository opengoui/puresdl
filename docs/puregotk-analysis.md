# puregotk 如何自动生成 GTK 的 Go 绑定

> 对象：`/Users/tbxark/Repos/opengoui/puregotk`（module `codeberg.org/puregotk/puregotk`）。
> 结论部分经过源码核对：`gen.go`、`internal/gir/{pass,types,util}`、`templates/go`、`internal/core/core.go`。

puregotk 的生成器是一个 **GIR（GObject-Introspection XML）→ Go** 的两遍流水线：解析 GIR，先收集全局类型表，再按文件渲染一个很大的 `text/template`，最后用 gofumpt 格式化。产物是纯 Go：每个 C 函数是一个包级函数变量 `var xFoo func(...)`，在 `init()` 里经 `dlopen` + `dlsym` + `purego.RegisterFunc` 绑定，全程无 cgo。

## 1. 输入：GIR XML

- `internal/gir/spec/*.gir` 内置 17 份：Adw、GLib、GModule、GObject、Gdk、GdkPixbuf、Gio、Graphene、Gsk、Gtk-4.0、JavaScriptCore、Pango、PangoCairo、Secret、Soup、WebKit、cairo（`copygir.sh` 从 flatpak 的 `org.gnome.Sdk` 里复制）。
- `internal/gir/types/types.go` 用 `encoding/xml` 结构体镜像整个 GIR schema（大部分来自 gotk4），标签带命名空间，涉及三个 XML 命名空间：core、C（`c:type`、`c:identifier`）、GLib（`glib:get-type`、`glib:signal`）。
- `Repository` → `Namespace`（Aliases/Classes/Interfaces/Records/Enums/Functions/Unions/Bitfields/Callbacks/Constants）；函数类共享 `CallableAttrs`（name、`c:identifier`、`throws`、Parameters、ReturnValue）；参数/字段/返回值用 `AnyType`（`*Type` 或 `*Array`）。
- 文档用 `,innerxml` 抓取，`Doc.StringSafe()` 转为 `//` 注释。
- 只使用 `r.Namespaces[0]`，即假定每个 GIR 文件只有一个命名空间。
- GIR 的信息量远大于 C 头文件：它直接给出所有权（transfer）、方向（in/out）、nullable、数组长度参数、`throws`、类继承、信号、属性。这决定了 puregotk 能生成“面向对象”的包装，而不只是函数绑定。

## 2. 两遍 pass

入口 `gen.go`：`os.RemoveAll("v4")` → 遍历收集 `*.gir` → `pass.New(girs, "codeberg.org/puregotk/puregotk/v4", pass.Options{IncludeBothVersions:true})` → `p.First()` → 构建模板并注册 FuncMap → `p.Second("v4", tmpl)` → 把手写文件复制进输出。`pkg/gir/{pass,util}` 是对 internal 的再导出，使外部项目可以用自己的 GIR 生成自己的绑定（`examples/mylib-gtk-meson-go`）。

**第一遍 `collectTypes`**：遍历所有 GIR，把每个 class、record、enum、bitfield、callback、interface、alias 登记到 `KindMap`（key 是规范化的 `ns.Name`）。`Kind`：`UnknownType / AliasType / CallbackType / ClassesType / InterfacesType / RecordsType / SliceType / OtherType`。enum 与 bitfield 登记为 AliasType（源码里有 TODO）；alias 取目标类型的 Kind，目标尚未收集时回退成 AliasType（依赖遍历顺序）。存在的原因：第二遍处理任一文件时，需要知道别的命名空间里类型的种类，例如 `Gtk.Widget` 是类、`Gio.AsyncReadyCallback` 是回调，才能选对转换代码。

**第二遍 `writeGo`**：再次遍历每个命名空间，构造 `TemplateArg`，按 GIR 文件名分组（`FilenameSafe`，使 Go 文件和 C 头文件对应），执行模板，再 `gofumpt/format.Source`。启用 `IncludeBothVersions` 时每个文件渲染两次（errgroup 并发）。

## 3. 类型映射

`types.go` 中的 `convList` 是基元表：`gboolean→bool`、`gint→int32`、`guint→uint32`、`gsize→uint`、`gssize→int`、`glong→int32`（LLP64 假设）、`gdouble→float64`、`utf8/filename→string`、`GType→types.GType`、`gpointer→uintptr`、`char**→[]string`、`va_list→[]interface{}`（仅为了能编译）。

`Type.Template` 的流程：先查 `convList`；否则规范化名字（同命名空间省略前缀）、查 `Kind`、数 `c:type` 里的 `*`；有指针且不是回调时加相应数量的 `*`；**record 且无指针（按值）→ `uintptr`**（注释写明 purego 无法按值传结构体）。数组：查 `convList`，默认 `uintptr`，元素带 `*` 则 `uintptr`，否则 `[]T`，定长数组 `[N]T`；`length` 属性被刻意忽略。

结构体 / 联合体的处理：

- record 输出为带 `_ structs.HostLayout` 的 Go 结构体（Go 1.23+ 保证 C 布局），仅作为 C 内存的视图，并带 `GoPointer() uintptr`。字段规则：`gint` 强制 `int32`，字符串字段为 `uintptr`，回调字段为私有 `xName uintptr`，`parent_class` 保留完整嵌入结构。
- union 一律输出为 `type X = uintptr`（源码注释：“structs are not yet supported”）。
- class 输出为只含 `Ptr uintptr` 的包装结构体，方法通过继承链展开。

## 4. 模板输出（`templates/go`，465 行）

- 命名：包名是小写命名空间；`util.SnakeToCamel` 转驼峰；构造函数经 `ConstructorName`（`gtk_button_new_with_label` → `NewButtonWithLabel`）；方法对应 `x<Class><Method>` 变量 + `(*Button) GetChild` 方法；与字段重名的方法加 `Fn` 后缀；参数名是 `<Camel>Var`，避免与类型名冲突。
- 每个函数：`var xButtonGetChild func(uintptr) uintptr` + 导出包装。包装负责把 Go 的 `string`/类实例/`[]string` 转成 purego 能传的值，返回类实例时检查 `cret == 0`，需要时 `IncreaseRef`（floating ref sink），并 `cls.Ptr = cret`。
- `throws="1"` 的函数：生成 `var cerr *glib.Error`，把 `&cerr` 作为额外参数，返回 `(T, error)`；失败时返回 `nil, cerr`。
- 只有含函数的文件才生成 `init()`（`NeedsInit`）：

  ```go
  func init() {
      core.SetPackageName("GTK", "gtk4")
      core.SetSharedLibraries("GTK", []string{"libgtk-4.so.1", "libgtk-4.1.dylib"})
      var libs []uintptr
      for _, libPath := range core.GetPaths("GTK") {
          lib, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
          if err != nil { panic(err) }
          libs = append(libs, lib)
      }
      core.PuregoSafeRegister(&xButtonGLibType, libs, "gtk_button_get_type")
      // ... 每个 C 函数一行
  }
  ```

  共享库名来自 GIR 的 `shared-library`（`.so`），生成器据此推出 macOS 的 `.dylib` 名。
- 手写补充（`templates/glib*`、`gobject`、`gtype`，由 `gen.go` 复制进输出）：回调注册表、`IncreaseRef`、`types.GType`（避免 import 环）。GIR 无法描述的东西靠它们补上。

## 5. 回调

回调类型的 Kind 是 `CallbackType`。作为参数时，包装函数调用 `glib.NewCallback(cb)`（可为空时 `NewCallbackNullable`）。信号连接（`ConnectXxx(cb *func(...))`）的模式：

1. 以用户 `*func` 的地址为 key，在 `glib` 的注册表（`sync.RWMutex` + map）里查已有 trampoline，命中则直接复用——避免耗尽 purego 的回调槽；
2. 否则生成 trampoline `func(clsPtr uintptr, ...)`，把裸指针包成 Go 类实例后调用用户函数，用 `purego.NewCallback` 创建并 `SaveCallback`；
3. 虚方法覆写（`Override<Name>`）和 `Get<Name>` 同样通过 `purego.NewCallback` 处理回调字段。

puregotk 使用 purego 的 **fork**（`codeberg.org/puregotk/purego`），加了 `NewCallbackFnPtr`、`UnrefCallback*` 以及 `[]string` 参数/返回支持，因此能释放回调槽：sysv 平台（darwin、linux amd64/arm64）真实 unref；Windows 为 no-op（有总量上限）；其他平台 panic。这三种实现靠文件名后缀与 build tag 区分（`glib_sysv`、`glib_windows`、`glib_other`）。

## 6. out 参数

GIR 参数 `direction="out"` 在模板模型里由 `AddAPI`/`AddPure` 处理：C 侧本来就是指针，所以对基元 Go 类型补一个 `*`（`*int32`），对非基元类型沿用已有指针；调用时原样传递，由调用方准备存储。`throws` 的 `GError**` 是 out 参数的特例（见第 4 节）。`caller-allocates` 等细节没有专门建模。

## 7. 文档 build tag 双版本

`pass.Options.IncludeBothVersions`：每个输出文件生成两份，由 `buildTag` 区分——

- `foo.go`：`//go:build puregotkdocs`，带 GIR 文档注释（LGPL）；
- `foo_nodocs.go`：`//go:build !puregotkdocs`，无注释（MIT）。

原因是 GIR 文档受 LGPL 约束，默认构建选无文档版本，使二进制与模块许可更宽松；需要文档（pkg.go.dev、IDE）时加 `-tags puregotkdocs`。`gen.sh` 因此对两个 tag 各跑一次 `go vet`。代价：产物体积翻倍、diff 噪声大。

## 8. 运行时加载（`internal/core/core.go`，234 行）

- **库定位** `GetPaths(name)`：① 环境变量 `PUREGOTK_<NAME>_PATH`（完整路径）；② `PUREGOTK_LIB_FOLDER` 目录；③ 内置的 `GOOS_GOARCH` 目录表（linux amd64/arm64、darwin arm64/amd64，**没有 windows 条目**）；④ `pkg-config --libs-only-L`；⑤ 都失败则 panic 并提示环境变量。每个文件名依次尝试后缀 `""`、`.0`、`.1`、`.2`，返回所有命中。
- **符号绑定** `PuregoSafeRegister(fptr, libs, name)`：依次对每个已打开的库 `Dlsym`，找到就 `RegisterFunc`，**找不到静默跳过**，函数变量保持 nil，调用时 nil-panic。这使同一份绑定可适配不同版本的库。
- **字符串辅助**：`CString`、`ByteSlice`（NULL 结尾的 `char**`）、`GoString`、`GoStringSlice`，拷贝自 purego 内部实现。

对 puresdl 的借鉴点：同样的“全局先加载、逐符号宽容绑定”；不同点是 puresdl 用 stub 函数代替 nil（调用时有可读的 panic 信息）、显式 `Available()`、Windows 用 `syscall.LoadLibrary`、不依赖 pkg-config。

## 9. 局限

- **purego 不能（稳妥地）按值传结构体**：puregotk 一律把 record 按值映射成 `uintptr`，union 也是 `uintptr`，因此凡是按值收发结构体的 C API 都不可用或类型不安全（源码里反复出现 “structs are not yet supported”）。注：purego 本身在 darwin/linux 的 amd64/arm64 与 Windows amd64/arm64 上已支持小结构体按值，但需要 Go 侧精确复刻 C 布局且不做校验，puregotk 选择不碰。
- 缺符号静默变 nil，错误延后到调用时。
- 依赖 purego fork，需要跟进上游。
- 回调槽有限（Windows 不能释放）。
- 路径表缺 Windows；`glong→int32` 的 LLP64 假设在 LP64 的 Linux/macOS 上并不成立。
- 双版本文件使产物成倍增加。
- 一个 GIR 文件只支持一个命名空间；alias 解析依赖遍历顺序。
- GIR 的 `length` 数组关系被忽略，数组和切片需要手动处理。

## 10. 对比：puregotk 与 puresdl

| 维度 | puregotk（GIR + text/template） | puresdl（C 头文件 + cc/v4 + jennifer） |
| --- | --- | --- |
| 输入 | GObject-Introspection `.gir` XML（语义已标注：所有权、方向、nullable、throws、继承） | SDL3 C 头文件（只有 C 类型，语义靠 doxygen 文本与命名约定推断） |
| 前端 | `encoding/xml` 结构体镜像 schema | `modernc.org/cc/v4` 完整预处理 + 解析，内嵌各目标预定义宏与 libc 桩，不需要 C 编译器 |
| 中间层 | 第一遍 `KindMap` + 第二遍 `TemplateArg` | 显式 `ir` 包，布局/偏移/枚举值由 C 前端算好 |
| 多平台 | GIR 与平台无关；库名靠 `.so`→`.dylib` 推导，没有 Windows | 对 darwin/linux/windows 各解析一次并合并，记录每个函数的声明目标 |
| 输出 | 一个 465 行的 `text/template` 渲染 Go 源文本，gofumpt 格式化 | `dave/jennifer` 构造 Go AST，`go/format` 格式化 |
| 生成风格 | 面向对象：类/继承/接口/信号/属性，`Ptr uintptr` 包装 | 扁平 C 风格 API：去 `SDL_` 前缀的函数、结构体、枚举 |
| 结构体 | `HostLayout` 视图；按值一律 `uintptr` | 真实布局结构体，字段偏移有生成测试比对；联合体为对齐存储 + 访问器；按值交给 purego |
| 字符串 | `string` 直接交给 purego；`[]string` 靠 fork | `const char*`→`string`（含 NULL 规则），返回值拷贝、需要时 `SDL_free` |
| 回调 | 注册表 + trampoline 复用，fork 提供 unref | typedef → `uintptr` 类型 + `NewXxx` 构造器，不释放（上限约 2000，文档提示） |
| out 参数 | 按 GIR `direction` 加 `*`；`throws` → `error` | 保留 C 的指针参数形态，不做封装 |
| 变参 | 无（GIR 不收录） | printf 风格在 Go 侧 `Sprintf`，其余跳过 |
| 缺符号 | 静默 nil，调用 nil-panic | 桩函数 panic 并带消息，`Available()` 可查 |
| 文档 | 双 build tag（含/不含 GIR 文档）规避许可问题 | 单份，保留 doxygen 文本，C 名改写为 Go 名 |
| 验证 | `go vet` 两个 tag | `layout_gen_test.go` 布局断言 + 针对真实 libSDL3 的 dummy 驱动测试 |
| purego | 使用 fork | 使用上游 `ebitengine/purego` |
