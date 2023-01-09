package requests

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"gitee.com/baixudong/gospider/tools"
)

type httpConn struct {
	rawConn            net.Conn
	proxyAuthorization string
}

func (obj *httpConn) Write(b []byte) (n int, err error) {
	if obj.proxyAuthorization == "" {
		return obj.rawConn.Write(b)
	}
	b = bytes.Replace(b, []byte("\r\n"), tools.StringToBytes(fmt.Sprintf("\r\nProxy-Authorization: Basic %s\r\n", obj.proxyAuthorization)), 1)
	obj.proxyAuthorization = ""
	return obj.rawConn.Write(b)
}
func (obj *httpConn) Read(b []byte) (n int, err error) {
	return obj.rawConn.Read(b)
}
func (obj *httpConn) Close() error {
	return obj.rawConn.Close()
}
func (obj *httpConn) LocalAddr() net.Addr {
	return obj.rawConn.LocalAddr()
}
func (obj *httpConn) RemoteAddr() net.Addr {
	return obj.rawConn.RemoteAddr()
}
func (obj *httpConn) SetDeadline(t time.Time) error {
	return obj.rawConn.SetDeadline(t)
}
func (obj *httpConn) SetReadDeadline(t time.Time) error {
	return obj.rawConn.SetReadDeadline(t)
}
func (obj *httpConn) SetWriteDeadline(t time.Time) error {
	return obj.rawConn.SetWriteDeadline(t)
}
