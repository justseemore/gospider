# 简介
python到golang爬虫过渡的所有必需库.
1. 请求库：支持ja3,http2 协议,各种主流代理协议,覆盖python requests 所有的功能
2. 浏览器：使用原生cdp 操控浏览器,无任何三方依赖，快速，高度自定义
3. 代理库：对数据抓包拦截修改,聚合代理池为隧道代理,作为网关拦截爬虫请求,作为代理突破ja3反爬
4. 并发库：自实现高性能并发库
5. 执行js: 通过管道调用js 方法
6. 执行python: 通过管道调用python 方法
7. 更多功能...
# 依赖
* go1.20 (不要低于这个版本)
# 安装 (不要拉github的包,go包路径只能在gitee和github选一个,拉github包会出现路径问题)
```
go get -u gitee.com/baixudong/gospider
```
# 教程
1. [知乎](https://www.zhihu.com/people/xiao-bai-shu-87-3/posts)
2. [掘金](https://juejin.cn/user/4098624347452359/posts)
3. [csdn](https://blog.csdn.net/Mr_bai_404?type=blog)

![](im.jpg)