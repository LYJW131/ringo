# Ringo 品牌资产

Ringo（林檎）是日语的「苹果」。标记是一只横着切开的苹果：切面是圆的，像一张唱片；果核露出五个心室，正好是一颗五角星；果核周围的果肉透着蜜色，是熟透的富士苹果才有的「蜜芯」。名字和图形都不使用 Apple 的商标。

## 文件

| 文件 | 用途 |
| --- | --- |
| `mark.svg` | 彩色标记，透明底。网页、文档、深浅背景都可用 |
| `mark-green.svg` | 青苹果版，只用于开发版（`mygo dev` 的图标） |
| `mark-mono.svg` | 单色标记：外圈加五个心室，不画籽。用于需要着色或很小的场合 |
| `wordmark.svg` / `wordmark-dark.svg` | 字标，浅色底 / 深色底。字母是画出来的单线圆头笔画，不依赖任何字体，`o` 就是标记本身 |
| `../icon/AppIcon.icon` | macOS 26 的 Liquid Glass 图标文档（Icon Composer），两层：`slice`（果皮与果肉）和 `core`（心室与籽） |
| `../icon/AppIcon-dev.icon` | 开发版图标，青苹果 |
| `../resources/icon.png`、`icon-dev.png` | 上面两份文档按 macOS 的画法渲染成 1024 像素的 PNG，旧版 macOS、Windows 和 Linux 用 |
| `../internal/app/tray/tray-template.png` | macOS 菜单栏的模板图（黑色加透明度，系统着色），36 像素，对应 18 点的 2 倍 |
| `../internal/app/tray/tray.png` | Windows 和 Linux 通知区域的彩色图标，64 像素 |

## 颜色

| 名称 | 值 | 用在哪 |
| --- | --- | --- |
| 果皮红 | `#FF6A5C` → `#E3213F`（自上而下） | 标记的外圈 |
| 果肉 | `#FFFDF2` → `#FFF4D6` → `#F9E0A8`（由内向外） | 标记的切面 |
| 蜜色 | `#FFD76A` → `#F7A823` → `#EA8A0E` | 五个心室；果核周围的晕是 `#FFC247` |
| 籽 | `#5A2A11`，中心点 `#8C4A14` | 籽 |
| 红玉 | `#C4244A` → `#6B0F2A` | 应用图标的底色（浅色外观） |
| 夜红玉 | `#3A1620` → `#150A0D` | 应用图标的底色（深色外观） |
| 青苹果 | 皮 `#D2E86A` → `#8DBF2E`，底 `#A6CF45` → `#4F8F22` | 开发版 |
| 墨 | `#2B1418` | 浅色底上的字标 |
| 纸 | `#FFF6E6` | 深色底上的字标 |
| 界面强调色 | 浅色外观 `oklch(0.57 0.215 14)`，深色外观 `oklch(0.66 0.2 14)` | 按钮、选中态、进度；定义在 `internal/app/ui_kit.go` 的 `paletteFor` |

## 重新生成

图形只有一处定义：`icon/main.go` 里的几何常量。改完后在仓库根目录运行（需要装有 Xcode 26 的 Mac，用它自带的 `ictool` 渲染）：

```bash
go run ./icon
```

```bash
go run ./icon/tray
```

第一条写出两份图标文档的图层、`resources/` 下的两张 PNG 和本目录的五个 SVG；第二条写出两张托盘图标，加 `-preview <目录>` 会另外输出放大的预览图。

## 用法

- 标记四周留出至少它直径 1/8 的空白。
- 小于 20 像素时用 `mark-mono.svg`，不要缩小彩色标记。
- 不改变比例，不换颜色，不在标记上叠文字。
- 不与 Apple 的标志并排使用，不暗示 Ringo 是 Apple 的产品。
