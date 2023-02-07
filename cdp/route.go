package cdp

import (
	"context"
	"errors"
	"log"

	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/tools"
)

type RequestOption struct {
	Url      string            `json:"url"`
	Method   string            `json:"method"`
	PostData string            `json:"postData"`
	Headers  map[string]string `json:"headers"`
}
type RequestData struct {
	Url              string            `json:"url"`
	UrlFragment      string            `json:"urlFragment"`
	Method           string            `json:"method"`
	Headers          map[string]string `json:"headers"`
	PostData         string            `json:"postData"`
	HasPostData      bool              `json:"hasPostData"`
	MixedContentType string            `json:"mixedContentType"`
	InitialPriority  string            `json:"initialPriority"` //初始优先级
	ReferrerPolicy   string            `json:"referrerPolicy"`
	IsLinkPreload    bool              `json:"isLinkPreload"`   //是否通过链路预加载加载。
	PostDataEntries  []DataEntrie      `json:"postDataEntries"` //是否通过链路预加载加载。
}
type RouteData struct {
	RequestId    string      `json:"requestId"`
	Request      RequestData `json:"request"`
	FrameId      string      `json:"frameId"`
	ResourceType string      `json:"resourceType"`
	NetworkId    string      `json:"networkId"`

	ResponseErrorReason string   `json:"responseErrorReason"`
	ResponseStatusCode  int      `json:"responseStatusCode"`
	ResponseStatusText  string   `json:"responseStatusText"`
	ResponseHeaders     []Header `json:"responseHeaders"`
}

type Route struct {
	webSock  *WebSock
	recvData RouteData
}

func (obj *Route) NewRequestOption() RequestOption {
	return RequestOption{
		Url:      obj.Url(),
		Method:   obj.Method(),
		PostData: obj.PostData(),
		Headers:  obj.Headers(),
	}
}

func (obj *Route) ResourceType() string {
	return obj.recvData.ResourceType
}
func (obj *Route) Url() string {
	return obj.recvData.Request.Url
}
func (obj *Route) Method() string {
	return obj.recvData.Request.Method
}
func (obj *Route) PostData() string {
	return obj.recvData.Request.PostData
}
func (obj *Route) Headers() map[string]string {
	return obj.recvData.Request.Headers
}

func (obj *Route) Request(ctx context.Context, routeOption RequestOption, options ...requests.RequestOption) (FulData, error) {
	var option requests.RequestOption
	if len(options) > 0 {
		option = options[0]
	}
	if routeOption.PostData != "" {
		option.Bytes = tools.StringToBytes(routeOption.PostData)
	}
	option.Headers = routeOption.Headers
	resourceType := obj.ResourceType()
	if resourceType == "Document" || "resourceType" == "XHR" {
		option.TryNum = 1
	}
	var fulData FulData
	var err error
	routeKey := obj.webSock.db.keyMd5(routeOption, resourceType)
	if !obj.webSock.filterKeys.Has(routeKey) {
		if err = obj.webSock.db.get(routeKey, &fulData); err == nil {
			if resourceType == "Document" || "resourceType" == "XHR" {
				obj.webSock.filterKeys.Add(routeKey)
			}
			if fulData.StatusCode == 0 {
				log.Print(routeOption.Url, " == ", fulData.StatusCode, " == body length == ", len(fulData.Body))
				log.Panic(errors.New("错误的状态码get"))
			}
			return fulData, err
		}
	}
	rs, err := obj.webSock.reqCli.Request(ctx, routeOption.Method, routeOption.Url, option)
	if err != nil {
		return fulData, err
	}
	headers := []Header{}
	for kk, vvs := range rs.Headers() {
		for _, vv := range vvs {
			headers = append(headers, Header{
				Name:  kk,
				Value: vv,
			})
		}
	}
	fulData.StatusCode = rs.StatusCode()
	fulData.Body = rs.Text()
	fulData.Headers = headers
	fulData.ResponsePhrase = rs.Status()
	if fulData.StatusCode == 0 {
		log.Print(rs)
		log.Print(routeOption.Url, " == ", fulData.StatusCode, " == body length == ", len(fulData.Body))
		log.Panic(errors.New("错误的状态码put"))
	}
	obj.webSock.db.put(routeKey, fulData, 60*60)
	return fulData, nil
}

func (obj *Route) FulFill(ctx context.Context, fulData FulData) error {
	_, err := obj.webSock.FetchFulfillRequest(ctx, obj.recvData.RequestId, fulData)
	return err
}
func (obj *Route) Continue(ctx context.Context) error {
	_, err := obj.webSock.FetchContinueRequest(ctx, obj.recvData.RequestId)
	return err
}

// Failed, Aborted, TimedOut, AccessDenied, ConnectionClosed, ConnectionReset, ConnectionRefused, ConnectionAborted, ConnectionFailed, NameNotResolved, InternetDisconnected, AddressUnreachable, BlockedByClient, BlockedByResponse
func (obj *Route) Fail(ctx context.Context, errorReason string) error {
	_, err := obj.webSock.FetchFailRequest(ctx, obj.recvData.RequestId, errorReason)
	return err
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type FulData struct {
	StatusCode     int      `json:"responseCode"`
	Headers        []Header `json:"responseHeaders"`
	Body           string   `json:"body"`
	ResponsePhrase string   `json:"responsePhrase"`
}
