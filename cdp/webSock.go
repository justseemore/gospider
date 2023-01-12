package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"gitee.com/baixudong/gospider/kinds"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"

	"go.uber.org/atomic"
)

type commend struct {
	Id     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}
type event struct {
	Ctx      context.Context
	Cnl      context.CancelFunc
	RecvData chan RecvData
}
type RecvError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type RecvData struct {
	Id     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
	Result map[string]any `json:"result"`
	Error  RecvError      `json:"error"`
}

type WebSock struct {
	db        *DbClient
	ids       sync.Map
	methods   sync.Map
	conn      *websocket.Conn
	ctx       context.Context
	cnl       context.CancelFunc
	id        atomic.Int64
	RouteFunc func(context.Context, *Route)
	reqCli    *requests.Client
	sync.Mutex
	filterKeys *kinds.Set[[16]byte]
}

type DataEntrie struct {
	Bytes string `json:"bytes"`
}

func (obj *WebSock) Done() <-chan struct{} {
	return obj.ctx.Done()
}
func (obj *WebSock) routeMain() error {
	event := obj.RegMethod(obj.ctx, "Fetch.requestPaused")
	pool := thread.NewClient(obj.ctx, 65535)
	defer obj.Close()
	defer pool.Close()
	defer event.Cnl()
	for {
		select {
		case <-obj.Done():
			return errors.New("websocks closed")
		case <-event.Ctx.Done():
			return errors.New("event closed")
		case recvData := <-event.RecvData:
			routeData := RouteData{}
			temData, err := json.Marshal(recvData.Params)
			if err == nil && json.Unmarshal(temData, &routeData) == nil {
				route := &Route{
					webSock:  obj,
					recvData: routeData,
				}
				if obj.RouteFunc != nil {
					if _, err := pool.Write(&thread.Task{
						Func: obj.RouteFunc,
						Args: []any{route},
					}); err != nil {
						return err
					}
				} else {
					if _, err := pool.Write(&thread.Task{
						Func: route.Continue,
					}); err != nil {
						return err
					}
				}
			}
		}
	}
}

func (obj *WebSock) recv(ctx context.Context, rd RecvData) error {
	cmdDataAny, ok := obj.ids.Load(rd.Id)
	if ok {
		cmdData := cmdDataAny.(*event)
		select {
		case <-obj.Done():
			return errors.New("websocks closed")
		case <-ctx.Done():
			return ctx.Err()
		case <-cmdData.Ctx.Done():
			obj.ids.Delete(rd.Id)
		case cmdData.RecvData <- rd:
		}
	}
	cmdDataAny, ok = obj.methods.Load(rd.Method)
	if ok {
		cmdData := cmdDataAny.(*event)
		select {
		case <-obj.Done():
			return errors.New("websocks closed")
		case <-ctx.Done():
			return ctx.Err()
		case <-cmdData.Ctx.Done():
			obj.methods.Delete(rd.Method)
		case cmdData.RecvData <- rd:
		}
	}
	return nil
}
func (obj *WebSock) recvMain() error {
	defer obj.Close()
	pool := thread.NewClient(obj.ctx, 65535)
	defer pool.Close()
	for {
		select {
		case <-obj.ctx.Done():
			return obj.ctx.Err()
		default:
			rd := RecvData{}
			if err := wsjson.Read(obj.ctx, obj.conn, &rd); err != nil {
				return err
			}
			if rd.Id == 0 {
				rd.Id = obj.id.Add(1)
			}
			if _, err := pool.Write(&thread.Task{
				Func: obj.recv,
				Args: []any{rd},
			}); err != nil {
				return err
			}
		}
	}
}

func NewWebSock(preCtx context.Context, ws, href, proxy string, getProxy func() (string, error), db *DbClient) (*WebSock, error) {
	reqOption := requests.ClientOption{Proxy: proxy}
	if getProxy != nil {
		reqOption.GetProxy = func(ctx context.Context, url *url.URL) (string, error) { return getProxy() }
	}
	reqOption.DisCookie = true
	reqCli, err := requests.NewClient(preCtx, reqOption)
	if err != nil {
		return nil, err
	}
	reqCli.RedirectNum = -1
	reqCli.DisDecode = true
	response, err := reqCli.Request(preCtx, "get", ws, requests.RequestOption{DisProxy: true})
	if err != nil {
		return nil, err
	}
	response.WebSocket().SetReadLimit(1024 * 1024 * 1024) //1G
	cli := &WebSock{
		conn:       response.WebSocket(),
		db:         db,
		reqCli:     reqCli,
		filterKeys: kinds.NewSet[[16]byte](),
	}
	cli.ctx, cli.cnl = context.WithCancel(preCtx)
	go cli.recvMain()
	go cli.routeMain()
	return cli, err
}
func (obj *WebSock) Close() error {
	obj.cnl()
	obj.reqCli.Close()
	return obj.conn.Close(websocket.StatusInternalError, "close")
}

func (obj *WebSock) regId(preCtx context.Context, ids ...int64) *event {
	data := new(event)
	data.Ctx, data.Cnl = context.WithCancel(preCtx)
	data.RecvData = make(chan RecvData)
	for _, id := range ids {
		obj.ids.Store(id, data)
	}
	return data
}
func (obj *WebSock) RegMethod(preCtx context.Context, methods ...string) *event {
	data := new(event)
	data.Ctx, data.Cnl = context.WithCancel(preCtx)
	data.RecvData = make(chan RecvData)
	for _, method := range methods {
		obj.methods.Store(method, data)
	}
	return data
}
func (obj *WebSock) send(ctx context.Context, cmd commend) (RecvData, error) {
	var cnl context.CancelFunc
	if ctx == nil {
		ctx, cnl = context.WithTimeout(obj.ctx, time.Second*60)
		defer cnl()
	}
	select {
	case <-obj.Done(): //webSocks 关闭
		return RecvData{}, errors.New("websocks closed")
	case <-ctx.Done():
		return RecvData{}, obj.ctx.Err()
	default:
		cmd.Id = obj.id.Add(1)
		idEvent := obj.regId(ctx, cmd.Id)
		defer idEvent.Cnl()
		if err := wsjson.Write(ctx, obj.conn, cmd); err != nil {
			return RecvData{}, err
		}
		select {
		case <-obj.Done():
			return RecvData{}, errors.New("websocks closed")
		case <-ctx.Done():
			return RecvData{}, ctx.Err()
		case idRecvData := <-idEvent.RecvData:
			if idRecvData.Error.Message != "" {
				return idRecvData, errors.New(idRecvData.Error.Message)
			}
			return idRecvData, nil
		}
	}
}
