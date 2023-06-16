# 版本
```
v0.10.0
```
# 修改指纹
## 添加函数
```go
type Upg struct {
	connPool *clientConnPool
	t        *Transport
}
type UpgOption struct {
	h2Ja3Spec             ja3.h2Ja3Spec
	DisableCompression    bool
	DialTLSContext        func(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error)
	IdleConnTimeout       int64
	TLSHandshakeTimeout   int64
	ResponseHeaderTimeout int64
}

func NewUpg(t1 *http.Transport, options ...UpgOption) *Upg {
	var option UpgOption
	if len(options) > 0 {
		option = options[0]
	}
	var headerTableSize uint32 = 65536
	var maxHeaderListSize uint32 = 262144
	var streamFlow uint32 = 6291456

	if option.h2Ja3Spec.InitialSetting != nil {
		for _, setting := range option.h2Ja3Spec.InitialSetting {
			switch setting.Id {
			case 1:
				headerTableSize = setting.Val
			case 6:
				maxHeaderListSize = setting.Val
			case 4:
				streamFlow = setting.Val
			}
		}
	} else {
		option.h2Ja3Spec.InitialSetting = []ja3.Setting{
			{Id: 1, Val: headerTableSize},
			{Id: 2, Val: 0},
			{Id: 3, Val: 1000},
			{Id: 4, Val: 6291456},
			{Id: 6, Val: maxHeaderListSize},
		}
	}
	if option.h2Ja3Spec.Priority.Exclusive == false && option.h2Ja3Spec.Priority.StreamDep == 0 && option.h2Ja3Spec.Priority.Weight == 0 {
		option.h2Ja3Spec.Priority = ja3.Priority{
			Exclusive: true,
			StreamDep: 0,
			Weight:    255,
		}
	}
	if option.h2Ja3Spec.OrderHeaders == nil {
		option.h2Ja3Spec.OrderHeaders = []string{":method", ":authority", ":scheme", ":path"}
	}
	if option.h2Ja3Spec.ConnFlow == 0 {
		option.h2Ja3Spec.ConnFlow = 15663105
	}

	if option.IdleConnTimeout == 0 {
		option.IdleConnTimeout = 30
	}
	if option.TLSHandshakeTimeout == 0 {
		option.TLSHandshakeTimeout = 15
	}
	if option.ResponseHeaderTimeout == 0 {
		option.ResponseHeaderTimeout = 30
	}

	connPool := new(clientConnPool)
	t2 := &Transport{
		t1:                        t1,
		h2Ja3Spec:                 option.h2Ja3Spec,
		streamFlow:                streamFlow,
		MaxDecoderHeaderTableSize: headerTableSize,   //1:initialHeaderTableSize,65536
		MaxEncoderHeaderTableSize: headerTableSize,   //1:initialHeaderTableSize,65536
		MaxHeaderListSize:         maxHeaderListSize, //6:MaxHeaderListSize,262144
		DisableCompression:        option.DisableCompression,

		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		DialTLSContext:   option.DialTLSContext,
		ReadIdleTimeout:  time.Duration(option.IdleConnTimeout) * time.Second, //检测连接是否健康的间隔时间
		PingTimeout:      time.Second * time.Duration(option.TLSHandshakeTimeout),
		WriteByteTimeout: time.Second * time.Duration(option.ResponseHeaderTimeout),

		ConnPool: noDialClientConnPool{connPool},
	}
	connPool.t = t2
	return &Upg{
		connPool: connPool,
		t:        t2,
	}
}
func (obj *Upg) CloseIdleConnections() {
	obj.t.CloseIdleConnections()
}
func (obj *Upg) UpgradeFn(authority string, c net.Conn) http.RoundTripper {
	addr := authorityAddr("https", authority)
	if used, err := obj.connPool.addConnIfNeeded(addr, obj.t, c); err != nil {
		defer c.Close()
		return erringRoundTripper{err}
	} else if !used {
		defer c.Close()
	}
	return obj.t
}
```

## 修改标头顺序
### 修改 enumerateHeaders 函数中的请求头顺序: 函数传参f 改为f2 ,函数内部重新定义函数f
```go
	func(f2 func(name, value string)) {
		//开头
		headers := http.Header{}
		f := func(name, value string) {
			headers.Add(name, value)
		}
		//中间代码不变
		//中间代码不变
		//中间代码不变
		//中间代码不变

		//结尾
		ll := kinds.NewSet[string]()
		for _, kk := range cc.t.h2Ja3Spec.OrderHeaders {
			for i := 0; i < 2; i++ {
				if i == 1 {
					kk = strings.Title(kk)
				}
				if vvs, ok := headers[kk]; ok {
					ll.Add(kk)
					for _, vv := range vvs {
						f2(kk, vv)
					}
					break
				}
			}
		}
		for kk, vvs := range headers {
			for _, vv := range vvs {
				if !ll.Has(kk) {
					f2(kk, vv)
				}
			}
		}
	}
```
## 修改streamFlow 值,修改addStreamLocked 函数中 cs.inflow.init 中 transportDefaultStreamFlow 变量为 int32(cc.t.streamFlow)

## 修改 newClientConn 函数中的   initialSettings 和 WriteWindowUpdate
```go
	initialSettings := make([]Setting, len(t.h2Ja3Spec.InitialSetting))
	for i, setting := range t.h2Ja3Spec.InitialSetting {
		initialSettings[i] = Setting{ID: SettingID(setting.Id), Val: setting.Val}
	}
	cc.fr.WriteSettings(initialSettings...)
	cc.fr.WriteWindowUpdate(0, t.h2Ja3Spec.ConnFlow)
	cc.inflow.init(int32(t.h2Ja3Spec.ConnFlow) + initialWindowSize)
```
## 修改ClientConn 的 writeHeaders 函数 的 first==true 时候的WriteHeaders 参数增加 Priority 值
```go
		if first {
			cc.fr.WriteHeaders(HeadersFrameParam{
				StreamID:      streamID,
				BlockFragment: chunk,
				EndStream:     endStream,
				EndHeaders:    endHeaders,
				Priority: PriorityParam{
					StreamDep: cc.t.h2Ja3Spec.Priority.StreamDep,
					Exclusive: cc.t.h2Ja3Spec.Priority.Exclusive,
					Weight:    cc.t.h2Ja3Spec.Priority.Weight,
				},
			})
			first = false
		} else {
			cc.fr.WriteContinuation(streamID, endHeaders, chunk)
		}
```