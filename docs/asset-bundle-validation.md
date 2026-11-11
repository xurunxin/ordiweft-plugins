# 原创片场素材包

`ordiweft-examples/director-models/1.0.1` 来自固定宿主提交 [ac78e2871220579e67c94892ac87e5c2f26c5e86](https://github.com/xurunxin/Ordiweft/tree/ac78e2871220579e67c94892ac87e5c2f26c5e86/examples/plugin-content/director-models)。它包含原创三角面、目录、完整release清单及Apache-2.0 LICENSE/NOTICE；没有抽取现有内置模型库、分发Morpheus或附加依赖。

`models/triangle.glb` 是424字节的自包含GLB，SHA-256为 `11783abc0a1e5d3d234ea90c61a48cb5a85f8bd3625f0e2cb94dbef30e73d892`。顶点为(0,0,0)、(1,0,0)、(0,1,0)，不含外部纹理、URL或脚本。

宿主先核对完整归档，再复用现有GLB和预览格式检查；成员在3D资产库预览原包的名称、版本、许可与署名，显式导入个人或团队模型库。导入事务保存原字节副本及许可，不自动放入场景。相同所有者/范围/GLB摘要复用原模型，新包不覆盖已有模型及场景。旧引用和原目录损坏后的副本读取沿用原资产权限。

宿主实际SQLite/PostgreSQL HTTP、1440px/390px指针和键盘、坏GLB检查反证与恢复结果见[固定源码验收](https://github.com/xurunxin/Ordiweft/blob/ac78e2871220579e67c94892ac87e5c2f26c5e86/docs/evidence/plugin-asset-bundle-20261111.md)。这些是宿主验收，不是本仓重新执行模型代码的记录。社区portable-pack只打包固定预构建字节，不执行GLB、插件、Go、npm或安装脚本。

此版本仅作为未发布候选，未创建信任根、签名或稳定目录。其他素材profile、预设、通用编译和完整平台迁移继续由宿主计划推进。

## 预构建分发检查

使用上述固定提交构建的CLI，SHA-256 `662eff2a931a482c12956b6c95273bc7fcf28cf11fb6a24e467526aee8d87185`。实际 `verify --root` / `pack --root ... --out ...` 检查7个完整样本文件，并与社区逐文件副本和独立打包ZIP比较，均相同；这些命令不执行模型。修正命令说明的样本1.0.1保留原GLB等字节，原1.0.0仍在宿主历史提交及其既有ZIP/证明中保留。

真实CLI门控下4项仓库测试全部PASS、0 SKIP（9.450s）：离线元数据、静态process镜像固定、实际打包/候选与不可变输出、源文件篡改及错误CLI摘要拒绝。十三份ZIP全部实际打包；原十二份ZIP、releaseDigest、来源和recipeDigest逐项保持相同。新素材包releaseDigest为 `c4e95ddffd21442a6fe4bdc49a953509e9af316677bd03467f4c0808b8b60d01`，ZIP 6831字节、SHA-256 `b1b191bc340ca243f69d716741e896e093481ef9f96e00981dce4876bf56a39b`。

`prepare` 只为这个新版本生成1条candidate状态的unsigned proposal，未生成稳定或签名目录，未安装到任何部署。CLI构建来源仍需维护者独立复核；证明中的工具提交和摘要不授予信任根或发行资格。
