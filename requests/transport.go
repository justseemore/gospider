package requests

import (
	"context"
	"crypto/tls"

	"net/http"
	"net/url"
	"time"

	"gitee.com/baixudong/gospider/http2"
)

func newHttp2Transport(ctx context.Context, session_option ClientOption, dialCli *DialClient) *http2.Transport {
	return &http2.Transport{
		DisableCompression: session_option.DisCompression,
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		DialTLSContext:     dialCli.requestHttp2DialTlsContext,
		AllowHTTP:          true,
		ReadIdleTimeout:    time.Duration(session_option.IdleConnTimeout) * time.Second, //检测连接是否健康的间隔时间
		PingTimeout:        time.Second * time.Duration(session_option.TLSHandshakeTimeout),
		WriteByteTimeout:   time.Second * time.Duration(session_option.ResponseHeaderTimeout),
	}
}
func newHttpTransport(ctx context.Context, session_option ClientOption, dialCli *DialClient) http.Transport {
	return http.Transport{
		MaxIdleConns:        655350,
		MaxConnsPerHost:     655350,
		MaxIdleConnsPerHost: 655350,
		ProxyConnectHeader: http.Header{
			"User-Agent": []string{UserAgent},
		},
		TLSHandshakeTimeout:   time.Second * time.Duration(session_option.TLSHandshakeTimeout),
		ResponseHeaderTimeout: time.Second * time.Duration(session_option.ResponseHeaderTimeout),
		DisableKeepAlives:     session_option.DisAlive,
		DisableCompression:    session_option.DisCompression,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout:       time.Duration(session_option.IdleConnTimeout) * time.Second, //空闲连接在连接池中的超时时间
		DialContext:           dialCli.requestHttpDialContext,
		DialTLSContext:        dialCli.requestHttpDialTlsContext,
		ForceAttemptHTTP2:     true,
		Proxy: func(r *http.Request) (*url.URL, error) {
			ctxData := r.Context().Value(keyPrincipalID).(*reqCtxData)
			ctxData.url = r.URL

			if ctxData.disProxy || ctxData.proxy == nil { //关闭代理或没有代理，走自实现代理
				return nil, nil
			} else if ctxData.ja3 && ctxData.url.Scheme == "https" && ctxData.proxy.Scheme != "https" { //因为除了https 代理之外的其它代理无法走tlscontext 函数,使用自实现
				return nil, nil
			}
			//代理需要账号密码,发送http 请求，代理不是socks5 协议，要隐藏代理
			if ctxData.proxy.User != nil && ctxData.url.Scheme == "http" && ctxData.proxy.Scheme != "socks5" {
				ctxData.proxyUser, ctxData.proxy.User = ctxData.proxy.User, nil
			}
			ctxData.isCallback = true //官方代理实现
			return ctxData.proxy, nil
		},
	}
}

func cloneTransport(t *http2.Transport) *http2.Transport {
	return &http2.Transport{
		DisableCompression: t.DisableCompression,
		TLSClientConfig:    t.TLSClientConfig,
		DialTLSContext:     t.DialTLSContext,
		AllowHTTP:          t.AllowHTTP,
		ReadIdleTimeout:    t.ReadIdleTimeout, //空闲连接在连接池中的超时时间
		PingTimeout:        t.PingTimeout,
		WriteByteTimeout:   t.WriteByteTimeout,
	}
}
