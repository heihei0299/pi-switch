<div align="center">

# pi-switch

[![版本](https://img.shields.io/badge/version-20261009.0.0-blue.svg)](https://github.com/heihei0299/pi-switch/releases)
[![平台](https://img.shields.io/badge/platform-Linux%20amd64%20%7C%20arm64-lightgrey.svg)](https://github.com/heihei0299/pi-switch/releases)
[![Built with Go](https://img.shields.io/badge/built%20with-Go-00ADD8.svg)](https://go.dev/)
[![许可证](https://img.shields.io/badge/license-MIT-green.svg)](https://opensource.org/license/mit)

**面向 pi agent、以 WebUI 为主的控制面板。**

管理 provider 配置，并发布本地模型名网关；也可通过 CLI 和 TUI 在终端中操作。

[English](README.md) | [中文](#)

</div>

---

## 界面截图

<div align="center">

<img src="assets/webui-home.png" alt="pi-switch WebUI 首页" width="48%"/>
<img src="assets/webui-profiles.png" alt="pi-switch WebUI Profiles" width="48%"/>
<br/>
<img src="assets/webui-gateway.png" alt="pi-switch WebUI Gateway" width="48%"/>
<img src="assets/webui-stats.png" alt="pi-switch WebUI 统计" width="48%"/>

</div>

---

## 功能简介

- 管理 provider 配置、模型列表和各渠道暴露的模型。
- 在 Gateway 页面预览并发布模型到 Pi。
- 通过本地代理按模型名路由请求。
- 在 WebUI 查看请求与 token 用量，并管理 pi packages。
- WebUI、CLI 和 TUI 共用同一套 Go 核心。

架构与详细行为见 [WebUI 指南](./WEBUI_GUIDE.md)、[后端架构](./docs/architecture.md) 和[系统契约](./docs/system-contract.md)。

## 安装

### 通过 npm 安装

~~~bash
npm install -g @heihei0299/pi-switch

# 或通过 pi 安装
pi install npm:@heihei0299/pi-switch
~~~

### 从源码构建

需要 Node.js 23.6 或更高版本，以及 Go 1.24.2 或更高版本。

~~~bash
git clone https://github.com/heihei0299/pi-switch.git
cd pi-switch
npm install
npm run build
node bin/pi-switch.js webui start --daemon
~~~

新版本 CLI/npm 仅支持 Linux amd64 和 arm64；macOS、Windows 不再支持，历史版本保持不变。可在 [Releases](https://github.com/heihei0299/pi-switch/releases) 查看可下载版本。

## 快速开始

启动浏览器界面并打开 <http://127.0.0.1:43110>：

~~~bash
pi-switch webui start --daemon
~~~

运行 <code>pi-switch doctor</code> 检查环境，用 <code>pi-switch webui stop</code> 停止界面。也可以使用终端界面 <code>pi-switch tui</code> 或其他 <code>pi-switch</code> 命令。

### 在 Pi 中使用模型

1. 在 **Profiles** 中添加 provider 并暴露模型。
2. 在 **Gateway** 中检查变更，然后选择 **Apply to Pi**。
3. 在 **Proxy** 中启动代理，或运行 <code>pi-switch proxy start --daemon</code>。
4. 在 Pi 中，Responses 模型选择 <code>pi-switch-res</code>，Chat 模型选择 <code>pi-switch-chat</code>，然后选择已暴露的模型。

WebUI 默认绑定 loopback。绑定非 loopback 地址需要密码，详见[安全说明](./WEBUI_GUIDE.md#security)。

## 常见问题

**如何在 Pi 中切换模型？**

打开 <code>/model</code>，选择已发布的 <code>pi-switch-res</code> 或 <code>pi-switch-chat</code> provider，再选择已暴露的模型。

**数据存放在哪里？**

配置和本地请求数据存放在 <code>~/.pi-switch/</code>。Pi 的 provider 注册表位于 <code>~/.pi/agent/models.json</code>。

## 开发

需要 Node.js 23.6 或更高版本，以及 Go 1.24.2 或更高版本。

~~~bash
npm install
npm run build
bash scripts/test-limited.sh
npm run test:webui
~~~

## 致谢

- 感谢 [cc-switch](https://github.com/farion1231/cc-switch) 和 [cc-switch-cli](https://github.com/SaladDay/cc-switch-cli) 带来的配置管理与终端界面思路。
- 感谢 [LINUX DO](https://linux.do/) 社区的讨论与启发。

## 许可证

MIT
