# UTF-8原文档换行归一化

原创Apache-2.0样本，使用公开SDK0.1.5的有界readAsset/stageOutput，不另写下载器、分块编码或重试。输入必须为一个role=source、kind=document、实际MIME=text/plain的原资产；宿主冻结原SHA/大小/MIME并逐次检查当前读取权限。将CRLF/CR显式转换为LF，输出私有文本文件候选及原源SHA、行数、字符数，不覆盖输入或自动写画布。

资源声明必需1个输入/输出、最多1MiB源文档、4MiB结果和60秒。参数为空闭合对象。独立fixture提供合成13字节原文和11字节预期结果；源与结果摘要不同。具体算法只处理UTF-8文本，不是PDF/Office/OCR转换器；SDK能够表达其他文档并不授予转换能力。

管理员可暂存/运行独立合成验证，通过后仍需显式启用。成员按画布范围plugin-actions API选择当前可读的原assetId，原任务done后读取私有附件。下载再次检查原输入权限；撤权、取消、unknown不重执行。没有网络、密钥、通用路径、业务资产写入或文档采用权限。
