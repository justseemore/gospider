# 版本
```
v0.10.0
```
# 修改指纹
## initialSettings
```go
{
    "SETTINGS": {
      "1": "65536",
      "2": "0",
      "3": "1000",
      "4": "6291456",
      "6": "262144"
    },
    "WINDOW_UPDATE": "15663105",
    "HEADERS": [
      ":method",
      ":authority",
      ":scheme",
      ":path"
    ]
}
```
## 添加函数
```go
type Upg struct {
	H2Ja3Spec             ja3.H2Ja3Spec
	DisableCompression    bool
	DialTLSContext        func(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error)
	IdleConnTimeout       int64
	TLSHandshakeTimeout   int64
	ResponseHeaderTimeout int64
}

func (obj Upg) UpgradeFn(authority string, c *tls.Conn) http.RoundTripper {
	var headerTableSize uint32 = 65536
	var maxHeaderListSize uint32 = 262144
	if obj.H2Ja3Spec.InitialSetting != nil {
		for _, setting := range obj.H2Ja3Spec.InitialSetting {
			switch setting.Id {
			case 1:
				headerTableSize = setting.Val
			case 6:
				maxHeaderListSize = setting.Val
			}
		}
	} else {
		obj.H2Ja3Spec.InitialSetting = []ja3.Setting{
			{Id: 1, Val: headerTableSize},
			{Id: 2, Val: 0},
			{Id: 3, Val: 1000},
			{Id: 4, Val: 6291456},
			{Id: 6, Val: maxHeaderListSize},
		}
	}
	if obj.H2Ja3Spec.OrderHeaders == nil {
		obj.H2Ja3Spec.OrderHeaders = []string{":method", ":authority", ":scheme", ":path"}
	}
	if obj.H2Ja3Spec.ConnFlow == 0 {
		obj.H2Ja3Spec.ConnFlow = 15663105
	}

	if obj.IdleConnTimeout == 0 {
		obj.IdleConnTimeout = 30
	}
	if obj.TLSHandshakeTimeout == 0 {
		obj.TLSHandshakeTimeout = 15
	}
	if obj.ResponseHeaderTimeout == 0 {
		obj.ResponseHeaderTimeout = 30
	}

	connPool := new(clientConnPool)
	t2 := &Transport{
		H2Ja3Spec:                 obj.H2Ja3Spec,
		MaxDecoderHeaderTableSize: headerTableSize,   //1:initialHeaderTableSize,65536
		MaxEncoderHeaderTableSize: headerTableSize,   //1:initialHeaderTableSize,65536
		MaxHeaderListSize:         maxHeaderListSize, //6:MaxHeaderListSize,262144
		DisableCompression:        obj.DisableCompression,

		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		DialTLSContext:   obj.DialTLSContext,
		AllowHTTP:        true,
		ReadIdleTimeout:  time.Duration(obj.IdleConnTimeout) * time.Second, //检测连接是否健康的间隔时间
		PingTimeout:      time.Second * time.Duration(obj.TLSHandshakeTimeout),
		WriteByteTimeout: time.Second * time.Duration(obj.ResponseHeaderTimeout),

		ConnPool: noDialClientConnPool{connPool},
	}
	connPool.t = t2
	addr := authorityAddr("https", authority)
	if used, err := connPool.addConnIfNeeded(addr, t2, c); err != nil {
		defer c.Close()
		return erringRoundTripper{err}
	} else if !used {
		defer c.Close()
		return erringRoundTripper{errors.New("used")}
	}
	return t2
}
```

## 修改伪标头顺序
### 增加 newClientConn 函数中的 ClientConn 私有变量  orderHeaders
```go
	orderHeaders:          t.H2Ja3Spec.OrderHeaders,
```
### 修改 enumerateHeaders 函数中的请求头顺序为 m,a,s,p
```go
		if req.Method != "CONNECT" {
			ll := kinds.NewSet(":method", ":authority", ":scheme", ":path")
			for _, h := range cc.orderHeaders {
				switch h {
				case ":method":
					f(":method", m)
					ll.Del(h)
				case ":authority":
					f(":authority", host)
					ll.Del(h)
				case ":scheme":
					f(":scheme", req.URL.Scheme)
					ll.Del(h)
				case ":path":
					f(":path", path)
					ll.Del(h)
				}
			}
			for _, h := range ll.Array() {
				switch h {
				case ":method":
					f(":method", m)
					ll.Del(h)
				case ":authority":
					f(":authority", host)
					ll.Del(h)
				case ":scheme":
					f(":scheme", req.URL.Scheme)
					ll.Del(h)
				case ":path":
					f(":path", path)
					ll.Del(h)
				}
			}
		} else {
			ll := kinds.NewSet(":method", ":authority")
			for _, h := range cc.orderHeaders {
				switch h {
				case ":method":
					f(":method", m)
					ll.Del(h)
				case ":authority":
					f(":authority", host)
					ll.Del(h)
				}
			}
			for _, h := range ll.Array() {
				switch h {
				case ":method":
					f(":method", m)
					ll.Del(h)
				case ":authority":
					f(":authority", host)
					ll.Del(h)
				}
			}
		}
```

## 修改streamFlow
### 增加 newClientConn 函数中的 ClientConn 私有变量  streamFlow
```go
	var streamFlow uint32 = 6291456
	for _, setting := range t.H2Ja3Spec.InitialSetting {
		if setting.Id == 4 {
			streamFlow = setting.Val
		}
	}

	streamFlow:            streamFlow,

```
### 修改 addStreamLocked 函数中 transportDefaultStreamFlow 变量为 int32(cc.streamFlow)

## 修改 newClientConn 函数中的   initialSettings 和 WriteWindowUpdate
```go
	initialSettings := make([]Setting, len(t.H2Ja3Spec.InitialSetting))
	for i, setting := range t.H2Ja3Spec.InitialSetting {
		initialSettings[i] = Setting{ID: SettingID(setting.Id), Val: setting.Val}
	}
	cc.fr.WriteSettings(initialSettings...)
	cc.fr.WriteWindowUpdate(0, t.H2Ja3Spec.ConnFlow)
	cc.inflow.init(int32(t.H2Ja3Spec.ConnFlow) + initialWindowSize)
```