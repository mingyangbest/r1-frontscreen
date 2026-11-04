# R1 FrontScreen

海康威视 R1 / R1X 前置触摸屏的显示工具。三页信息面板（环境 / 详情 / 系统）加一个网页面板，能看 CPU、内存、磁盘、温度和网络，也能调亮度、风扇和灯效。

**能装的机器只有海康 R1 / R1X**（Intel N100）。程序用 KMS/DRM 直接画在机器内置的那块 376×960 小屏上，背光和风扇读的也是这台机器自己的节点。普通电脑、群晖、威联通装了也点不亮，因为它们没有这块屏。

还需要能 SSH 进 fnOS（或 Debian 系）并拿到 `sudo`。三条都满足，直接看 [部署方式](#部署方式)，一条命令装完，不用装 Go，也不用编译。

> 本项目基于 [lemon6515/r1-toolbox-go](https://github.com/lemon6515/r1-toolbox-go) 增强 fork，遵循上游 **GPL-3.0 + Commons Clause** 许可证，感谢上游作者的开源工作。

## 界面预览

### 三页信息屏（当前）

| 环境（主屏） | 详情 | 系统 |
|:---:|:---:|:---:|
| ![环境](docs/screenshots/env.png?v=096) | ![详情](docs/screenshots/detail.png?v=096) | ![系统](docs/screenshots/system.png?v=096) |

*三页左右滑动。首屏放 CPU 60 秒曲线，每核明细在详情页。读数与进度条共用一套阈值色：低于 50% 绿，50% 以上沙金，80% 以上珊瑚。图由 `cmd/preview` 在真机上采样渲染，不是截屏；局域网和公网地址由工具当场换成 RFC 5737 文档地址。*

管理面板（`http://<设备IP>:8080`，浅色 / 深色两套）：

| 浅色 | 深色 |
|:---:|:---:|
| ![面板浅色](docs/screenshots/panel-light.png?v=096) | ![面板深色](docs/screenshots/panel-dark.png?v=096) |

*面板负责这块屏的一切：实时预览、页面显隐与顺序、亮度和开关屏、风扇曲线与 PWM 映射、灯效、显示设置、告警与 Bark 推送、24 小时历史、容器列表与日志、磁盘占用、硬件扫描。不做容器编排，也不替代 fnOS 自带的系统管理。屏幕有异常时顶部会出现一条横幅，平时不显示，没有横幅就代表一切正常。面板两列布局，窄屏自动单列。图中存储卡的大文件列表含私人文件名，公开图里换成了说明文字，其余都是真机实况。*

<details>
<summary><b>天气图标全集（8 种）</b></summary>

| 晴 | 多云 | 雨 | 雪 | 雾 | 雷雨 | 阴 | 夜 |
|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| ![sun](docs/screenshots/weather-icons.png) |

上图为 8 种图标合成验收图，用 `preview -wx <类型>` 逐个渲染。图标是全屏唯一允许多色的区域：太阳金、雨蓝、雪白、雾灰蓝、霾土黄、闪电金。夜间的月与星保持中性银白。

</details>

## 本 fork 的主要改进

| 方向 | 内容 |
|---|---|
| 性能 | CPU 占用从约 89% 降到单核 2% 左右，靠跳帧渲染、局部重绘和圆角遮罩缓存 |
| 安全 | 面板令牌鉴权，只认 `X-Token` 请求头；连续 8 次错误锁 IP 5 分钟；令牌明文只在屏幕上点按时显示 10 秒 |
| 体验 | 阈值色统一、天气图标静态化、夜间自动调暗、配置文件保存保留注释 |
| 可靠性 | systemd 看门狗，主循环卡死 30 秒内自动重启；外部命令带超时；采样在后台协程 |
| 代码 | 渲染器拆成 canvas / design / textrender / theme / widgets，设计常量集中在一处 |

## 硬件与系统要求

- 海康威视 R1 或 R1X NAS。程序走 KMS/DRM，需要背光 sysfs 和风扇 hwmon，**无法在普通电脑上运行**
- 飞牛 fnOS，或 Debian / Ubuntu 系发行版
- 装预编译包不需要 Go；从源码编译需要 Go 1.19+

### 字体

两族分开渲染，各自探测，互不影响：

| 用途 | 字体 | 来源 |
|---|---|---|
| 数字 / 拉丁 | IBM Plex Sans（Thin / Light / Bold） | 嵌在程序里，首次启动解到 `/tmp/r1-toolbox-fonts/`，不用自己装 |
| 中文 | Noto Sans CJK → 文泉驿微米黑 → 文泉驿正黑 → Droid | 探测系统字体，取第一个能打开的 |

数字必须等宽。之前的 Cantarell 是比例宽度，100px 下 `1` 是 42px、`8` 是 58px，时钟从 `15:31` 换到 `08:05` 会宽出 41px。IBM Plex Sans 十个数码一样宽，而且本来就是给仪器面板设计的。字体按 OFL 1.1 分发，见 `internal/ui/assets/fonts/OFL.txt`。

中文不嵌字体，是因为思源黑体单个字重就有 10MB。想用思源执行 `sudo apt install fonts-noto-cjk` 即可自动生效，没装则退回文泉驿，两种都不影响功能。

## 设计规范（改界面时照这个来）

| 项目 | 规则 |
|---|---|
| **卡片标题** | 18px、卡色实色、与卡内第一行内容同一左基线 |
| **卡名** | 2–4 字对象名，不带修饰词（CPU、内存、磁盘、温度、设备、网络 IP、Docker、控制） |
| **文字色阶** | white（时钟 / 卡标题 / 主机名）> sub（主读数）> dim（标签）> tab（分隔符与页签） |
| **间距** | 只有四档：3（值与单位）、7（并列项）、8（`·` 两侧）、10（斜杠两侧） |
| **卡片配色** | 每卡一个低饱和色相，相邻卡片不同色 |

间距只有四个档位。2026-11-01 之前这里散落着 2 到 10 九个值，同样一件事在不同卡里疏密不一样，单看每张卡都没问题，整屏摆一起就不齐。

## 部署方式

> **三条硬前提**：海康 R1 / R1X、fnOS 或 Debian 系、能 SSH 并拿到 `sudo`。不满足的话，装完只会得到一个跑起来但点不亮屏幕的进程。

### 🚀 第一次装？按这个顺序来

> [!IMPORTANT]
> 第一次接触这个项目，不用先读完整篇 README，按下面 5 步走就行，细节在下面的方式 A 里。

1. 确认机器：海康 R1 / R1X，系统是 fnOS 或 Debian 系。不是这台机器就先停，装完屏幕也点不亮。
2. 找到 NAS 的局域网 IP，在电脑终端里 SSH 登录（fnOS 需要先开启 SSH 服务）：

   ```bash
   ssh <你的NAS用户名>@<NAS的局域网IP>
   ```

3. 只取安装脚本，两行装完，不用 clone 整个仓库、不用装 Go、不用编译：

   ```bash
   curl -fsSL https://raw.githubusercontent.com/mingyangbest/r1-frontscreen/master/install.sh -o install.sh
   sudo bash install.sh
   ```

   脚本全程不提问，装完会告诉你屏幕、面板地址和令牌在哪看。

4. 验收三件事：小屏点亮并能左右滑三页；浏览器打开 `http://<NAS的IP>:8080`；滑到系统页点「控制」卡里的「面板令牌」一行，显示 10 秒，填进面板。
5. 屏幕没亮先别重装：看 `sudo journalctl -u r1-toolbox -n 50`；GRUB 里有关屏参数时用 `sudo bash install.sh --fix-grub` 后重启；确认看的是机器自带小屏，不是 HDMI。

> [!WARNING]
> 面板是明文 HTTP，只在内网用，不要映射到公网。以后升级重跑一次安装脚本就行，配置不会被覆盖。

### 方式 A：一条命令（推荐）

```bash
git clone https://github.com/mingyangbest/r1-frontscreen.git
cd r1-frontscreen
sudo ./install.sh
```

只想装一次、不打算改代码的话，用上面第 3 步那两行命令就够了，不必 clone 整个仓库。

脚本取 Release 里的预编译包，不装 Go，不编译，全程不提问：

1. 检查机型、`/dev/fb0`、`/dev/dri/card*`，不对就停下来说清楚原因
2. 只装运行时依赖：`libsdl2-2.0-0`、`libsdl2-ttf-2.0-0`、中文字体、`smartmontools`、`lm-sensors`、`i2c-tools`
3. 下载 `r1-toolbox-linux-amd64` 并校验架构，Release 带 `.sha256` 时一并核对；旧二进制自动备份
4. 已有配置先备份、不覆盖，备份放在 `/var/backups/r1-toolbox/`，只补缺失的

装完会告诉你三件事：屏幕是否点亮、面板地址、令牌在屏幕哪一页看。

常用参数：

| 参数 | 用途 |
|---|---|
| `--port 9000` | 面板端口（仅首次生成配置时生效） |
| `--version v1.0.1` | 装指定版本，默认最新 |
| `--binary ./r1-toolbox-linux-amd64` | 离线安装，用本地包，不联网 |
| `--from-source` | 从源码编译（改代码 / ARM 主板） |
| `--fix-grub` | 移除 GRUB 里的关屏参数（会改引导配置） |
| `--force` | 跳过硬件检查（无屏环境） |

> **下载卡住**：GitHub 在国内时通时不通。在能上网的机器上取
> `https://github.com/mingyangbest/r1-frontscreen/releases/latest/download/r1-toolbox-linux-amd64`，
> `scp` 到 NAS，然后 `sudo ./install.sh --binary ./r1-toolbox-linux-amd64`。

<details>
<summary>从旧版升级 / 手动管理服务</summary>

- 升级直接重跑 `sudo ./install.sh`，二进制换新、配置不动、服务自动重启。
- 旧版 `install.sh` 把配置写在 `/etc/r1-control/config.yaml`，和程序读的不是一个地方。新脚本会迁移到 `/etc/r1-toolbox/config.yaml`，你设过的端口与风扇曲线不会丢。
- 旧单元是 `Type=simple` 的话，新单元会覆盖它。如果另外加过 drop-in，记得删掉重复的 `Type=` 和 `WatchdogSec=`：

```bash
sudo mkdir -p /etc/systemd/system/r1-toolbox.service.d
printf '[Service]\nType=notify\nWatchdogSec=30\n' | \
  sudo tee /etc/systemd/system/r1-toolbox.service.d/override.conf
sudo systemctl daemon-reload && sudo systemctl restart r1-toolbox
```

- 服务名、二进制名、配置路径沿用上游的 `r1-toolbox`，从上游或旧版升级不用迁移。

```bash
sudo systemctl status r1-toolbox      # 查看状态
sudo journalctl -u r1-toolbox -f      # 看日志
/usr/local/bin/r1-toolbox --version   # 查版本（v0.9.7+；更早的版本会启动第二个实例）
```

</details>

### 方式 B：从源码编译（改代码 / ARM 主板）

```bash
sudo ./install.sh --from-source
```

会装 Go 与 SDL2 开发库再 `make build`。R1 上首次编译约 1 到 3 分钟，ARM 主板只能走这条。没有 Go 环境也可以用 Docker 编译：

```bash
docker compose run --rm build      # 产物落在 ./build/r1-toolbox
```

### 方式 C：Docker 容器运行（实验性）

屏幕渲染需要特权设备访问，容器方式仅作尝试，不保证在所有固件上可用：

```bash
docker compose up -d screen   # privileged + /dev/fb0 + host 网络
```

遇到问题请回退方式 A。

### 方式 D：fpk 应用包（飞牛应用中心）

不用碰命令行：从 [Releases](https://github.com/mingyangbest/r1-frontscreen/releases) 下载 `r1-frontscreen-1.0.1.fpk`，在飞牛应用中心手动安装。包是自带的，SDL2 运行库和中文字体都在里面，纯净系统也能装。

也可以自己打包，配置在 `deploy/fnos/`：

```bash
cd deploy/fnos
VERSION=1.0.1 COMMIT=$(git rev-parse --short HEAD) ./build.sh
```

需要飞牛官方 `fnpack` 和 SDL2 开发文件。脚本会收集依赖库、字体，最后用 `fnpack build` 出包，产物约 17 MB。

> 应用中心不接受同版本覆盖：改了内容要先改 `deploy/fnos/manifest` 里的 `version` 再打包，否则装上去的还是旧包。

## Web 控制面板

启动后浏览器访问 `http://<设备IP>:8080`。面板用 `X-Token` 头鉴权，首次访问会提示输入一次，之后存在浏览器本地。

**怎么拿令牌**：在设备屏幕上滑到「系统」页，点一下「控制」卡里的「面板令牌」一行，屏上显示明文 10 秒。也可以直接读配置文件，路径随安装方式不同，见下面的 [配置](#配置) 一节。

令牌只接受 `X-Token` 头，不走 URL 参数，避免被访问日志抄走。连续 8 次错误会锁定该 IP 5 分钟。

能做什么：

| 分组 | 内容 |
|---|---|
| 屏幕 | 亮度、自动关屏时长、开屏关屏（关屏需点两次确认） |
| 显示 | 主题（墨 / 素 / 沙 / 青）、12 / 24 小时制、天气显隐、刷新节奏（2 / 3 / 5 秒），改完立刻生效 |
| 风扇 | CPU / 机箱风扇的模式与温控曲线、PWM 通道与温度源映射 |
| 灯效 | RGB 模式与颜色（R1 前屏未见系统级灯效通路，可能没有可见效果） |
| 硬件 | hwmon 扫描，列出本机风扇与温度传感器，换机器时用它确定通道 |

## 安全说明

面板是明文 HTTP，没有 TLS，请只在内网使用。令牌在网络上是明文传输，同一局域网内的嗅探者可以截获；拿到令牌就能控制这块屏幕，包括改亮度、关屏和调风扇。

**不要直接做端口映射到公网。** 确实要外网访问，请在前面加一层带 HTTPS 的反向代理，由反代终结 TLS，并且至少再加一道认证：

```
# Caddy 示例（自动申请证书）
screen.example.com {
    reverse_proxy 192.168.1.10:8080
    basicauth { admin <bcrypt-hash> }
}
```

**令牌没有轮换接口。** 泄露后手改配置文件并重启服务（路径与重启方式见 [配置](#配置)）：

```bash
# install.sh 装法
sudo sed -i 's/^\(\s*token:\).*/\1 换成新的8位令牌/' /etc/r1-toolbox/config.yaml
sudo systemctl restart r1-toolbox
```

面板不提供重启服务、执行命令、上传文件这类入口，也不要为了图方便给它加一个。

## 配置

配置与数据的位置取决于安装方式，别按错地方找：

| 装法 | 配置文件 | 数据（历史库 / 告警记录） | 重启 |
|---|---|---|---|
| `install.sh`（systemd） | `/etc/r1-toolbox/config.yaml` | `/var/lib/r1-toolbox/` | `sudo systemctl restart r1-toolbox` |
| fpk（应用中心） | `/vol1/@appconf/r1-frontscreen/config.yaml` | `/vol1/@appdata/r1-frontscreen/` | 应用中心里停止再启动，或 `sudo appcenter-cli stop r1-frontscreen && sudo appcenter-cli start r1-frontscreen` |

程序每 5 秒热重载，改完即生效。字段说明见 [`config.example.yaml`](config.example.yaml)。

天气数据来自[心知天气](https://www.seniverse.com/)免费接口，API Key 继承自上游项目（已随上游公开）。如失效，可自行注册并替换 `internal/netinfo/netinfo.go` 中的 `senKey`。

## 已知限制

- 侧边 RGB 灯条：上游代码保留了 RGB 模块，但在 R1 前屏硬件上没找到系统级软件控制通路（内核 LED、I2C、ACPI 都没有接口），灯效可能由屏体 MCU 自带
- 设备卡的盘位数来自机型映射表（当前只映射 `HS-AFS-R1` 到 4 盘位），不是硬件读数。Linux 没有暴露盘位数的通用接口。其余字段（机型名、CPU、内存类型与容量、fnOS 版本）全部为系统实读，读不到就整项省略，不填占位符
- 本程序为非官方项目，与海康威视、飞牛官方均无关联，刷写和安装的风险自负

## 更新日志

完整历史见 [CHANGELOG.md](CHANGELOG.md)，每个版本都有 git tag 和 [Release](https://github.com/mingyangbest/r1-frontscreen/releases)。最近几个版本：

- **v1.0.1**：修面板鉴权可被伪造头绕过、补服务端超时与请求体上限、磁盘扫描加互斥；风扇温度源失联不再按「机器是凉的」处理
- **v1.0.0**：分发形态改成 fpk 应用包，由应用中心托管启停，应用以非特权用户运行
- **v0.9.8**：安装脚本按配置文件里的端口做自检，不再误报「端口没响应」
- **v0.9.7**：安装改成下载预编译包，不装 Go 不编译；修掉配置写错目录、每次覆盖配置、`-version` 起第二个实例三个问题
- **v0.9.6**：面板新增磁盘占用分析，风扇映射改完立即生效

## 项目结构

```
cmd/main.go            程序入口
cmd/preview/           离屏渲染工具，把页面画成 PNG，不参与运行
internal/ui/           屏幕渲染：canvas / design（设计常量与阈值取色）/ textrender / theme / widgets / pages
internal/monitor/      CPU / 内存 / 网络 2 秒采样，磁盘 SMART / Docker 60 秒采样
internal/hardware/     背光、风扇曲线、触摸屏、hwmon 发现
internal/web/          Web 控制面板（gin + 内嵌前端，令牌鉴权）
internal/netinfo/      网络信息与天气（30 分钟刷新）
internal/config/       配置热重载（yaml.Node 键级合并，保留注释与键序）
web-ui/                Web 面板前端源码（Vue 3 + Vite）
```

## 开发

```bash
make build          # 或 docker compose run --rm build
go vet ./...
```

## 许可证

本项目继承上游的 GPL-3.0 + Commons Clause 许可证，见 [LICENSE](LICENSE)。任何分发或修改须同样以 GPL-3.0 开源，并保留原版权声明。
