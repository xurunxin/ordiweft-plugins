# ComfyUI 工作流适配器

此模块维护 ComfyUI API JSON 编译、接入鉴权、素材上传、进度、单次提交和原任务读取。Ordiweft 以固定 Go 模块版本链接；只有安装并激活 ComfyUI 渠道包后，才会公布工作流目录与开放接单。

工作流连接配置包含固定 HTTP(S) 地址和可选的凭据环境变量名。直接接入可使用 HTTP；承载 Bearer 和 X-API-Key 的连接使用 HTTPS。适配器只查询节点定义进行导入预览。