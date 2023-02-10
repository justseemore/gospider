package chanx

import (
	"container/list"
	"context"
	"sync"
	"time"
)

type Client[T any] struct {
	pip  chan T
	buf  *list.List
	ctx  context.Context
	cnl  context.CancelFunc
	ctx2 context.Context
	cnl2 context.CancelFunc
	lock sync.RWMutex
	len  int64
}

func NewClient[T any](preCtx context.Context) *Client[T] {
	if preCtx == nil {
		preCtx = context.TODO()
	}
	ctx, cnl := context.WithCancel(preCtx)
	ctx2, cnl2 := context.WithCancel(preCtx)
	client := &Client[T]{
		pip:  make(chan T),
		buf:  list.New(),
		ctx:  ctx,
		cnl:  cnl,
		ctx2: ctx2,
		cnl2: cnl2,
	}
	go client.run()
	return client
}
func (obj *Client[T]) Add(val T) error {
	select {
	case <-obj.ctx.Done():
		return obj.ctx.Err()
	case <-obj.ctx2.Done():
		return obj.ctx2.Err()
	default:
		obj.push(val)
		return nil
	}
}
func (obj *Client[T]) Chan() <-chan T {
	return obj.pip
}
func (obj *Client[T]) push(val T) {
	obj.lock.Lock()
	obj.buf.PushBack(val)
	obj.len++
	obj.lock.Unlock()
}
func (obj *Client[T]) get() any {
	obj.lock.Lock()
	val := obj.buf.Remove(obj.buf.Front())
	obj.len--
	obj.lock.Unlock()
	return val
}

func (obj *Client[T]) send() error {
	if obj.Len() <= 0 {
		time.Sleep(time.Second)
		return nil
	}
	if remVal := obj.get(); remVal != nil {
		select {
		case <-obj.ctx2.Done():
			return obj.ctx2.Err()
		case obj.pip <- remVal.(T):
			return nil
		}
	}
	return nil
}
func (obj *Client[T]) run() {
	defer close(obj.pip)
	defer obj.cnl2()
	for {
		select {
		case <-obj.ctx.Done():
			if err := obj.send(); err != nil {
				return
			}
		case <-obj.ctx2.Done():
			return
		default:
			if err := obj.send(); err != nil {
				return
			}
		}
	}
}
func (obj *Client[T]) Join() { //等待消费完毕后，关闭
	obj.cnl()
	<-obj.ctx2.Done()
}
func (obj *Client[T]) Close() { //立刻关闭
	obj.cnl()
	obj.cnl2()
}
func (obj *Client[T]) Done() <-chan struct{} {
	return obj.ctx2.Done()
}
func (obj *Client[T]) Len() int64 {
	return obj.len
}
