package requests

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/http2"
)

func newHttp2Transport(ctx context.Context, session_option ClientOption, dialCli *dialClient) *http2.Transport {
	return &http2.Transport{
		DisableCompression: session_option.DisCompression,
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		DialTLSContext:     dialCli.dialTlsContext2,
		AllowHTTP:          true,
		ReadIdleTimeout:    time.Duration(session_option.IdleConnTimeout) * time.Second, //检测连接是否健康的间隔时间
		PingTimeout:        time.Second * time.Duration(session_option.TLSHandshakeTimeout),
		WriteByteTimeout:   time.Second * time.Duration(session_option.ResponseHeaderTimeout),
	}
}
func newHttpTransport(ctx context.Context, session_option ClientOption, dialCli *dialClient) http.Transport {
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
		DialContext:           dialCli.dialContext,
		DialTLSContext:        dialCli.dialTlsContext,
		ForceAttemptHTTP2:     true,
		Proxy: func(r *http.Request) (*url.URL, error) {
			ctxData := r.Context().Value(keyPrincipalID).(*reqCtxData)
			ctxData.url = r.URL
			if (ctxData.ja3 && ctxData.url.Scheme == "https") || ctxData.disProxy { //如果是ja3或者关闭代理，则走自实现代理
				return nil, nil
			}
			if ctxData.proxy != nil && ctxData.proxy.User != nil && ctxData.proxy.Scheme == "http" && ctxData.url.Scheme == "http" {
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
