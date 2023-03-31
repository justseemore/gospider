package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/db"
	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/websocket"

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
	option     WebSockOption
	db         *db.Client[FulData]
	ids        map[int64]*event
	methods    map[string]*event
	methodLock sync.RWMutex
	idLock     sync.RWMutex
	conn       *websocket.Conn
	ctx        context.Context
	cnl        context.CancelCauseFunc
	id         atomic.Int64
	pageId     string
	RouteFunc  func(context.Context, *Route)
	reqCli     *requests.Client
	lock       sync.Mutex

	PageStarId int64
	pageEndId  int64
	PageDone   chan struct{}
}

type DataEntrie struct {
	Bytes string `json:"bytes"`
}

func (obj *WebSock) Done() <-chan struct{} {
	return obj.ctx.Done()
}
func (obj *WebSock) routeMain() (err error) {
	event := obj.RegMethod(obj.ctx, "Fetch.requestPaused")
	pool := thread.NewClient(obj.ctx, 65535)
	defer obj.Close(err)
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
						Func: route._continue,
					}); err != nil {
						return err
					}
				}
			}
		}
	}
}
func (obj *WebSock) PageStop() bool {
	return obj.pageEndId >= obj.PageStarId
}
func (obj *WebSock) recv(ctx context.Context, rd RecvData) error {
	switch rd.Method {
	case "Page.frameStartedLoading":
		if obj.pageId == rd.Params["frameId"].(string) {
			if rd.Id > obj.PageStarId {
				obj.PageStarId = rd.Id
			}
		}
	case "Page.frameStoppedLoading":
		if obj.pageId == rd.Params["frameId"].(string) {
			if rd.Id > obj.pageEndId {
				obj.pageEndId = rd.Id
			}
		}
		if obj.PageStop() {
			select {
			case obj.PageDone <- struct{}{}:
			default:
			}
		}
	}
	obj.idLock.RLock()
	cmdData, ok := obj.ids[rd.Id]
	obj.idLock.RUnlock()
	if ok {
		select {
		case <-obj.Done():
			return errors.New("websocks closed")
		case <-ctx.Done():
			return ctx.Err()
		case <-cmdData.Ctx.Done():
			obj.idLock.Lock()
			delete(obj.ids, rd.Id)
			obj.idLock.Unlock()
		case cmdData.RecvData <- rd:
		}
	}
	obj.methodLock.RLock()
	cmdData, ok = obj.methods[rd.Method]
	obj.methodLock.RUnlock()
	if ok {
		select {
		case <-obj.Done():
			return errors.New("websocks closed")
		case <-ctx.Done():
			return ctx.Err()
		case <-cmdData.Ctx.Done():
			obj.methodLock.Lock()
			delete(obj.methods, rd.Method)
			obj.methodLock.Unlock()
		case cmdData.RecvData <- rd:
		}
	}
	return nil
}
func (obj *WebSock) recvMain() (err error) {
	defer obj.Close(err)
	pool := thread.NewClient(obj.ctx, 65535)
	defer pool.Close()
	for {
		select {
		case <-obj.ctx.Done():
			return obj.ctx.Err()
		default:
			rd := RecvData{}
			if err := obj.conn.RecvJson(obj.ctx, &rd); err != nil {
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

type WebSockOption struct {
	Proxy        string
	DisDataCache bool //关闭数据缓存
	Ja3Spec      ja3.ClientHelloSpec
	Ja3          bool
}

func NewWebSock(preCtx context.Context, globalReqCli *requests.Client, ws string, option WebSockOption, db *db.Client[FulData], pageId string) (*WebSock, error) {
	response, err := globalReqCli.Request(preCtx, "get", ws, requests.RequestOption{DisProxy: true})
	if err != nil {
		return nil, err
	}
	response.WebSocket().SetReadLimit(1024 * 1024 * 1024) //1G
	cli := &WebSock{
		ids:     make(map[int64]*event),
		methods: make(map[string]*event),
		conn:    response.WebSocket(),
		db:      db,
		reqCli:  globalReqCli,
		option:  option,
		pageId:  pageId,
	}
	cli.ctx, cli.cnl = context.WithCancelCause(preCtx)
	go cli.recvMain()
	go cli.routeMain()
	return cli, err
}
func (obj *WebSock) Close(err error) error {
	obj.cnl(err)
	return obj.conn.Close("close")
}

func (obj *WebSock) regId(preCtx context.Context, ids ...int64) *event {
	data := new(event)
	data.Ctx, data.Cnl = context.WithCancel(preCtx)
	data.RecvData = make(chan RecvData)
	for _, id := range ids {
		obj.idLock.Lock()
		obj.ids[id] = data
		obj.idLock.Unlock()
	}
	return data
}
func (obj *WebSock) RegMethod(preCtx context.Context, methods ...string) *event {
	data := new(event)
	data.Ctx, data.Cnl = context.WithCancel(preCtx)
	data.RecvData = make(chan RecvData)
	for _, method := range methods {
		obj.methodLock.Lock()
		obj.methods[method] = data
		obj.methodLock.Unlock()
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
	case <-obj.Done():
		return RecvData{}, context.Cause(obj.ctx)
	case <-ctx.Done():
		return RecvData{}, obj.ctx.Err()
	default:
		cmd.Id = obj.id.Add(1)
		idEvent := obj.regId(ctx, cmd.Id)
		defer idEvent.Cnl()
		if err := obj.conn.SendJson(ctx, cmd); err != nil {
			return RecvData{}, err
		}
		select {
		case <-obj.Done():
			return RecvData{}, context.Cause(obj.ctx)
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
