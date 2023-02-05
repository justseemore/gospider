# 功能概述

- 基于net/http 二次封装
- cookies 开关，连接池开关，http2
- 自实现http,socks5代理
- 自动解压缩,解码
- dns缓存
- 类型自动转化
- 尝试重试，请求回调

# 发送http请求

```golang
package main

import (
    "log"

    "gitee.com/baixudong/gospider/requests"
)

func main() {
    reqCli, err := requests.NewClient(nil) //创建请求客户端
    if err != nil {
        log.Panic(err)
    }
    response, err := reqCli.Request(nil, "get", "http://myip.top") //发送get请求
    if err != nil {
        log.Panic(err)
    }
    log.Print(response.Text())    //获取内容,解析为字符串
    log.Print(response.Content()) //获取内容,解析为字节
    log.Print(response.Json())    //获取json,解析为gjson
    log.Print(response.Html())    //获取内容,解析为html
    log.Print(response.Cookies()) //获取cookies
}

```

# 发送websocket 请求

```golang
package main

import (
	"context"
	"log"

	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/websocket"
)

func main() {
	reqCli, err := requests.NewClient(nil) //创建请求客户端
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "ws://82.157.123.54:9010/ajaxchattest", requests.RequestOption{Headers: map[string]string{
		"Origin": "http://coolaf.com",
	}}) //发送websocket请求
	if err != nil {
		log.Panic(err)
	}
	defer response.Close()
	wsCli := response.WebSocket()
	if err = wsCli.WriteMsg(context.TODO(), websocket.MessageText, []byte("测试")); err != nil { //发送txt 消息
		log.Panic(err)
	}
	msgType, con, err := wsCli.ReadMsg(context.TODO()) //接收消息
	if err != nil {
		log.Panic(err)
	}
	log.Print(msgType)     //消息类型
	log.Print(string(con)) //消息内容
}
```
