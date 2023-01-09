package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"gitee.com/baixudong/gospider/bar"
	"gitee.com/baixudong/gospider/bs4"
	"gitee.com/baixudong/gospider/tools"
	"github.com/tidwall/gjson"
	"nhooyr.io/websocket"
)

type Response struct {
	Response  *http.Response
	WebSocket *websocket.Conn
	cnl       context.CancelFunc
	content   []byte
	encoding  string
	disDecode bool
	disUnzip  bool
}

func (obj *Client) newResponse(r *http.Response, cnl context.CancelFunc, request_option RequestOption) (*Response, error) {
	response := &Response{Response: r, cnl: cnl}
	if request_option.DisRead { //是否预读
		return response, nil
	}
	if request_option.DisUnZip || r.Uncompressed { //是否解压
		response.disUnzip = true
	}
	response.disDecode = request_option.DisDecode      //是否解码
	return response, response.read(request_option.Bar) //读取内容
}
func (obj *Response) Location() (*url.URL, error) {
	return obj.Response.Location()
}
func (obj *Response) Cookies() []*http.Cookie {
	if obj.Response == nil {
		return []*http.Cookie{}
	}
	return obj.Response.Cookies()
}
func (obj *Response) StatusCode() int {
	if obj.Response == nil {
		return 0
	}
	return obj.Response.StatusCode
}
func (obj *Response) Url() *url.URL {
	if obj.Response == nil {
		return nil
	}
	return obj.Response.Request.URL
}
func (obj *Response) Headers() http.Header {
	return obj.Response.Header
}

func (obj *Response) Text(val ...string) string {
	if len(val) > 0 {
		obj.content = tools.StringToBytes(val[0])
	}
	return tools.BytesToString(obj.content)
}
func (obj *Response) Decode(encoding string) {
	if obj.encoding != encoding {
		obj.encoding = encoding
		obj.content = tools.Decode(obj.content, encoding)
	}
}
func (obj *Response) Map(path ...string) map[string]any {
	var data map[string]any
	if err := json.Unmarshal(obj.content, &data); err != nil {
		return nil
	}
	return data
}
func (obj *Response) Json(path ...string) gjson.Result {
	return tools.Any2json(obj.content, path...)
}
func (obj *Response) Content(val ...[]byte) []byte {
	if len(val) > 0 {
		obj.content = val[0]
	}
	return obj.content
}
func (obj *Response) Html() *bs4.Client {
	return bs4.NewClient(obj.Text(), obj.Url().String())
}
func (obj *Response) ContentType() string {
	return obj.Response.Header.Get("Content-Type")
}
func (obj *Response) ContentEncoding() string {
	return obj.Response.Header.Get("Content-Encoding")
}
func (obj *Response) ContentLength() int64 {
	return obj.Response.ContentLength
}

type barBody struct {
	body *bytes.Buffer
	bar  *bar.Client
}

func (obj *barBody) Write(con []byte) (int, error) {
	l, err := obj.body.Write(con)
	obj.bar.Print(int64(l))
	return l, err
}
func (obj *Response) barRead() (*bytes.Buffer, error) {
	barData := &barBody{
		bar:  bar.NewClient(obj.Response.ContentLength),
		body: bytes.NewBuffer(nil),
	}
	_, err := io.Copy(barData, obj.Response.Body)
	if err != nil {
		return nil, err
	}
	return barData.body, nil
}
func (obj *Response) verifyBytes() bool {
	return strings.Contains(obj.Headers().Get("Accept-Ranges"), "bytes")
}

func (obj *Response) read(bar bool) error { //读取body,对body 解压，解码操作
	defer obj.Close()
	var bBody *bytes.Buffer
	var err error
	if bar && obj.ContentLength() > 0 { //是否打印进度条,读取内容
		bBody, err = obj.barRead()
	} else {
		bBody = bytes.NewBuffer(nil)
		_, err = io.Copy(bBody, obj.Response.Body)
	}
	if err != nil {
		return errors.New("io.Copy error: " + err.Error())
	}
	if !obj.disUnzip {
		if bBody, err = tools.ZipDecode(bBody, obj.ContentEncoding()); err != nil {
			return errors.New("gzip NewReader error: " + err.Error())
		}
	}
	obj.content = bBody.Bytes()
	if !obj.disDecode && !obj.verifyBytes() {
		obj.content, obj.encoding = tools.Charset(obj.content, obj.ContentType())
	}
	return nil
}
func (obj *Response) Close() error {
	if obj.cnl != nil {
		defer obj.cnl()
	}
	if obj.WebSocket != nil {
		obj.WebSocket.Close(websocket.StatusInternalError, "close")
	}
	if obj.Response.Body != nil {
		io.Copy(io.Discard, obj.Response.Body)
		return obj.Response.Body.Close()
	}
	return nil
}
