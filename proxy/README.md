# 功能概述
* 快如闪电的正向代理
* 支持http,https,socks五
* 支持隧道代理的开发
* 支持白名单，用户名密码
* 支持ja3 指纹代理,使客户端隐藏自身ja3指纹
* 支持链式代理，设置下游代理
* 支持http,https,websocket,http2 抓包
* 让不支持http2协议的客户端访问http2网站,例如：python 中requests 不支持http2协议,使其支持http2协议

#  一个端口同时实现 http,https,socks五 代理
```go
func main() {
	proCli, err := proxy.NewClient(nil, proxy.ClientOption{
		Port:    7006,
	})
	if err != nil {
		log.Panic(err)
	}
	proCli.DisVerify = true//关闭白名单验证和密码验证，在没有白名单和密码的情况下如果不关闭，用不了
	log.Print(proCli.Addr())
	log.Panic(proCli.Run())
}
```
# 设置白名单
```go
func main() {
	proCli, err := proxy.NewClient(nil, proxy.ClientOption{
		Port:    7006,
        IpWhite: []net.IP{
			net.IPv4(192, 168, 1, 11),
		},
	})
	if err != nil {
		log.Panic(err)
	}
	log.Print(proCli.Addr())
	log.Panic(proCli.Run())
}
```
# 设置账号密码
```go
func main() {
	proCli, err := proxy.NewClient(nil, proxy.ClientOption{
		Port:    7006,
       	Usr:     "admin",
		Pwd:     "password",
	})
	if err != nil {
		log.Panic(err)
	}
	log.Print(proCli.Addr())
	log.Panic(proCli.Run())
}
```








