package requests

import (
	"context"
	"crypto/tls"

	"net/http"
	"net/url"
	"time"

	"gitee.com/baixudong/gospider/http2"
)

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
		TLSNextProto: map[string]func(authority string, c *tls.Conn) http.RoundTripper{
			"h2": http2.Upg{
				H2Ja3Spec:      session_option.H2Ja3Spec,
				DialTLSContext: dialCli.requestHttp2DialTlsContext,
			}.UpgradeFn,
		},
		Proxy: func(r *http.Request) (*url.URL, error) {
			ctxData := r.Context().Value(keyPrincipalID).(*reqCtxData)
			ctxData.url = r.URL
			if ctxData.disProxy || ctxData.ja3 { //关闭代理或ja3 走自实现代理
				return nil, nil
			}
			if ctxData.proxy != nil {
				ctxData.isCallback = true //官方代理实现
			}
			return ctxData.proxy, nil
		},
	}
}
