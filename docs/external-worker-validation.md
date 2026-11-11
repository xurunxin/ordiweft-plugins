# 外部 PNG worker 声明包验证

`ordiweft-examples/png-renderer/1.0.0`固定[Ordiweft 30c45ab14aa2867c61cd9660ebacfe38e8d12b82](https://github.com/xurunxin/Ordiweft/tree/30c45ab14aa2867c61cd9660ebacfe38e8d12b82/examples/plugin-worker/png-renderer/package)的五份原始文件，采用Apache-2.0。包只声明无素材输入的`render-png`动作，输出小型私有PNG候选；没有服务程序、URL、凭据、运行镜像或启动命令。

部署管理员另外管理受信服务，按原release绑定origin、实际实现SHA及可选服务端凭据环境引用。绑定与静态ready不能证明服务质量或隔离；激活另走宿主CAS。原提交意图先落账，丢响应/重启/换版只查询原任务，取消在终止确认前仍占资源容量。完整终态在released=false时也先封存，后续终止证明不能替换原结果。详见[宿主实际验收](https://github.com/xurunxin/Ordiweft/blob/30c45ab14aa2867c61cd9660ebacfe38e8d12b82/docs/evidence/plugin-external-worker-20261111.md)。

## 固定制品

| 项 | 实际值 |
| --- | --- |
| 宿主CLI源码 | `30c45ab14aa2867c61cd9660ebacfe38e8d12b82`，干净隔离checkout |
| CLI SHA-256 | `ca83b84b4973a9ec1f2c23e2d221c7c260693c918e069ad4bb4d65849b4ab591` |
| release digest | `a1ae43119cb449837086b892eb055615d987f840ce35663287f02fa83a3dfa13` |
| ZIP | 6290字节，SHA-256 `c4ab41a9ebf327f57eb2775988ee61cd57dfca33abebd007e7a74f3e820b559a` |
| 公开SDK worker程序 | Linux/amd64，SHA-256 `da1d1b24113b66243451f8767314cb690efe03095d97c76f5b9069cbad657a9f`，不包含在插件ZIP中 |

干净固定源码导出55文件公开Go SDK，只有`pkg/aigc`/`pkg/pluginhost`及其许可/模块文件；无宿主internal、Morpheus或第三方Go模块。精确已安装Go工具链离线构建两次，完整65文件发行闭包字节相同，含SDK许可、Go LICENSE/PATENTS及构建证明。此结论不授权镜像分发或生产运行。

该Linux程序实际在非root、只读bundle、network=none、cap-drop ALL、no-new-privileges、128MiB/1CPU/64 PID的独立合成容器运行，仅挂载自己的私有状态。一次POST得到真实32×16 PNG（113字节，SHA-256 `6d9d09dd8e2d137c52235ef655639398951177f82a1da07e0d2a7c386282a5fa`）；三次GET包含重启后的原响应与未知任务404，原请求/响应逐字节不变，requests.log仍只有一次提交。没有模型、供应商、宿主业务库或生产凭据。容器退出后已观察自身名称无残留；这不是宿主自动部署或多租户服务验收。

## 社区仓库检查

真实固定CLI打包16份候选；原15份的全部artifact元数据及ZIP逐字节保留。新增包`runtimeImage=null`，宿主端点与运行器配置不进入配方。拒绝worker运行镜像及端点的反例、真实CLI双次打包/提案、字节篡改与CLI摘要错误等5项测试PASS、0 SKIP（10.555s）。另生成单包unsigned候选提案；未签名、未发布、未安装、未激活或部署。

远端干净clone的同一配方验证与16份制品比较另行补记；完整O0–O8/M1–M2计划仍开放，更多worker输入/流式协议、付费预算和生产服务边界不在此样本中宣称完成。
