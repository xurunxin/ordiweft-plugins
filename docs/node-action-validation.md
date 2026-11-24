# 文本分析节点预构建验证

新增`ordiweft-examples/text-analysis-node/1.0.0`，固定[Ordiweft bd2e589](https://github.com/xurunxin/Ordiweft/commit/bd2e58919594d0c9aafabc357cedc95d2ef5039e)。原处理器与表单节点共包，只有同包单动作、无连线端口的节点可安装；按钮用原节点同名字段草稿预填共享处理器表单，显式运行前可独立试参，不自动保存或执行节点。

宿主SQLite和本地PostgreSQL实际通过：公开CLI打包、安装/激活、真实隔离执行、完整结果与资源释放、更新不改原节点、旧版本新调用拒绝而原命令仍读相同结果。桌面指针与390px键盘浏览器通过，原任务运行中激活1.1.0，旧/新表单schema和结果字段说明分别保留；服务端只观察到两次显式动作POST，没有生成或Agent调用。原动作关系校验的反证失败后恢复通过。完整范围、早期失败与间歇Docker不可用限制见[宿主证据](https://github.com/xurunxin/Ordiweft/blob/bd2e58919594d0c9aafabc357cedc95d2ef5039e/docs/evidence/plugin-node-actions-20261111.md)。

从上述固定源码以`-trimpath -buildvcs=false`构建CLI，SHA-256为`17abf086774814237052f14b53c3cbdbd87641646c82ef73d4ec9dd28205f114`。十五包实际build与真实CLI测试4 PASS / 0 SKIP（11.008秒）；此前十四份配方、来源、release和ZIP字节保持不变。新增九个源码文件与固定宿主源码相同，社区ZIP与独立从固定宿主样本pack的ZIP逐字节相同：11974字节，SHA-256 `9130185f4156b77ce7f825f866b7e7006f0ce02da8c003eaaded209dfa5f8ad4`，release digest `6b8cf1759660429c82e5a6df3c2c099ae2e0354b4f3bb006dfa51fb5b3b9ca9f`。

需要支持同包处理器节点的宿主和配方固定的隔离镜像；旧宿主拒绝安装，不忽略动作限制。社区portable-pack只打包预构建源码字节，没有执行插件、编译、安装依赖、访问模型或分发Morpheus。仅准备新节点的unsigned candidate提案；不生成根、签名、stable或发布目录，不安装、激活或部署任何真实服务。
