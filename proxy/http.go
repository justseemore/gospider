package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

func (obj *Client) httpHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	var err error
	clientReq, err := client.readRequest(obj.RequestCallBack)
	if err != nil {
		return err
	}
	if strings.HasPrefix(clientReq.Host, "127.0.0.1") || strings.HasPrefix(clientReq.Host, "localhost") {
		if clientReq.URL.Port() == obj.port {
			return errors.New("loop addr error")
		}
	}
	if err = obj.verifyPwd(client, clientReq); err != nil {
		return err
	}
	proxyUrl, err := obj.dialer.GetProxy(ctx, nil)
	if err != nil {
		return err
	}
	var proxyServer net.Conn
	host := clientReq.Host
	addr := net.JoinHostPort(clientReq.URL.Hostname(), clientReq.URL.Port())
	if proxyServer, err = obj.dialer.DialContextForProxy(ctx, "tcp", client.option.schema, addr, host, proxyUrl); err != nil {
		return err
	}
	server := newProxyCon(ctx, proxyServer, bufio.NewReader(proxyServer), *client.option, false)
	defer server.Close()
	if clientReq.Method == http.MethodConnect {
		if _, err = client.Write([]byte(fmt.Sprintf("%s 200 Connection established\r\n\r\n", clientReq.Proto))); err != nil {
			return err
		}
	} else {
		tempBytes := bytes.NewBuffer(nil)
		if err = clientReq.Write(tempBytes); err != nil {
			return err
		}
		client.reader = bufio.NewReader(io.MultiReader(tempBytes, client.reader))
	}
	return obj.copyMain(ctx, client, server)
}
func (obj *Client) httpsHandle(ctx context.Context, client *ProxyConn) error {
	defer client.Close()
	tlsClient := tls.Server(client, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{obj.cert},
	})
	defer tlsClient.Close()
	return obj.httpHandle(ctx, newProxyCon(ctx, tlsClient, bufio.NewReader(tlsClient), *client.option, true))
}
