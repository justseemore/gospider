# 功能概述
* 浏览器渲染页面功能,快速的进行渲染页面并返回内容

# 快速开始
## 快速生成基于命令行的程序  (使用go buld 自行编译哈)
### 源码
```go
func main() {
	client, err := splash.NewFlagClient(nil)
	if err != nil {
		log.Panic(err)
	}
	log.Panic(client.Run())
}
```
### 查看命令行参数使用  -h
```shell
  -browserNum int
        浏览器的数量 (default 1)       
  -headless
        是否开启无头模式 (default true)
  -host string
        host
  -log string
        日志文件
  -pageNum int
        标签页最大打开数量 (default 100)
  -port int
        port (default 8210)
  -proxy string
        代理
```
### api文档
```
https://console-docs.apipost.cn/preview/3832c58d674caa85/fca62d9e59519ccb
```