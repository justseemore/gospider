# 简介
### gospider 是一个golang 爬虫神器，拥有python到golang爬虫过渡的所有必需库。用于python爬虫从业者快速且无坑的过渡到golang
1. [请求库](../../tree/master/requests)：ja3指纹,http2 指纹,主流代理协议,类型自动转换,覆盖python requests 所有的功能
2. [并发库](../../tree/master/thread)：自实现高性能并发库
3. [执行js,py](../../tree/master/cmd): 通过管道调用js,python 中的方法
4. [操控浏览器的库](https://github.com/chromedp/chromedp)：建议直接用这个库，基于cdp 协议操控浏览器，干净，快速
# 依赖
* go1.20 (不要低于这个版本)
# 安装 (不要拉github的包,go包路径只能在gitee和github选一个,拉github包会出现路径问题)
```
go get -u gitee.com/baixudong/gospider
```

# [测试用例](../../tree/master/test) 

# 博客
1. [知乎](https://www.zhihu.com/people/xiao-bai-shu-87-3/posts)
2. [掘金](https://juejin.cn/user/4098624347452359/posts)
3. [csdn](https://blog.csdn.net/Mr_bai_404?type=blog)

![](im.jpg)