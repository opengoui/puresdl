# 着色器来源

`PositionColorTransform.vert` 与 `SolidColor.frag` 来自 [TheSpydog/SDL_gpu_examples](https://github.com/TheSpydog/SDL_gpu_examples)（Copyright (C) 2024 Caleb Cornett，zlib 许可证，全文见 [LICENSE](LICENSE)），未作修改。`.hlsl` 是源码，`.spv`、`.msl`、`.dxil` 是用 [SDL_shadercross](https://github.com/libsdl-org/SDL_shadercross) 编译出的产物：

```bash
shadercross PositionColorTransform.vert.hlsl -o PositionColorTransform.vert.spv   # 以及 .msl、.dxil
```

顶点着色器输入：location 0 是 `float3` 位置，location 1 是 `float4` 颜色；顶点 uniform 槽 0 是列主序 `float4x4` 变换。MSL 入口是 `main0`，SPIR-V 与 DXIL 入口是 `main`。
