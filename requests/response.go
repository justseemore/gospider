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
	"gitee.com/baixudong/gospider/websocket"
	"github.com/tidwall/gjson"
)

type Response struct {
	response  *http.Response
	webSocket *websocket.Conn
	cnl       context.CancelFunc
	content   []byte
	encoding  string
	disDecode bool
	disUnzip  bool
}

func (obj *Client) newResponse(r *http.Response, cnl context.CancelFunc, request_option RequestOption) (*Response, error) {
	response := &Response{response: r, cnl: cnl}
	if request_option.DisRead { //是否预读
		return response, nil
	}
	if request_option.DisUnZip || r.Uncompressed { //是否解压
		response.disUnzip = true
	}
	response.disDecode = request_option.DisDecode      //是否解码
	return response, response.read(request_option.Bar) //读取内容
}
func (obj *Response) Response() *http.Response {
	return obj.response
}
func (obj *Response) WebSocket() *websocket.Conn {
	return obj.webSocket
}
func (obj *Response) Location() (*url.URL, error) {
	return obj.response.Location()
}
func (obj *Response) Cookies() []*http.Cookie {
	if obj.response == nil {
		return []*http.Cookie{}
	}
	return obj.response.Cookies()
}
func (obj *Response) StatusCode() int {
	return obj.response.StatusCode
}
func (obj *Response) Status() string {
	return obj.response.Status
}
func (obj *Response) Url() *url.URL {
	if obj.response == nil {
		return nil
	}
	return obj.response.Request.URL
}
func (obj *Response) Headers() http.Header {
	return obj.response.Header
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
func (obj *Response) Text(val ...string) string {
	if len(val) > 0 {
		obj.content = tools.StringToBytes(val[0])
	}
	return tools.BytesToString(obj.content)
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
	return obj.response.Header.Get("Content-Type")
}
func (obj *Response) ContentEncoding() string {
	return obj.response.Header.Get("Content-Encoding")
}
func (obj *Response) ContentLength() int64 {
	return obj.response.ContentLength
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
		bar:  bar.NewClient(obj.response.ContentLength),
		body: bytes.NewBuffer(nil),
	}
	err := tools.CopyWitchContext(obj.response.Request.Context(), barData, obj.response.Body)
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
		err = tools.CopyWitchContext(obj.response.Request.Context(), bBody, obj.response.Body)
	}
	if err != nil {
		return errors.New("io.Copy error: " + err.Error())
	}
	if !obj.disUnzip {
		if bBody, err = tools.ZipDecode(bBody, obj.ContentEncoding()); err != nil {
			return errors.New("gzip NewReader error: " + err.Error())
		}
	}
	if !obj.disDecode && !obj.verifyBytes() {
		if content, encoding, err := tools.Charset(bBody.Bytes(), obj.ContentType()); err == nil {
			obj.content, obj.encoding = content, encoding
		} else {
			obj.content = bBody.Bytes()
		}
	} else {
		obj.content = bBody.Bytes()
	}
	return nil
}
func (obj *Response) Close() error {
	if obj.cnl != nil {
		defer obj.cnl()
	}
	if obj.webSocket != nil {
		obj.webSocket.Close("close")
	}
	if obj.response.Body != nil {
		io.Copy(io.Discard, obj.response.Body)
		return obj.response.Body.Close()
	}
	return nil
}
