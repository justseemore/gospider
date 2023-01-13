package pool

import (
	"context"
	"errors"

	"gitee.com/baixudong/gospider/chanx"
)

type Client[T any] struct {
	new     func(context.Context) (T, error)
	clear   func(context.Context, T) bool
	datas   *chanx.Client[T]
	maxSize int
	ctx     context.Context
	cnl     context.CancelFunc
	Err     error
}
type ClientOption[T any] struct {
	New     func(context.Context) (T, error)
	Clear   func(context.Context, T) bool
	MaxSize int
}

func NewClient[T any](preCtx context.Context, option ClientOption[T]) (*Client[T], error) {
	if preCtx == nil {
		preCtx = context.TODO()
	}
	if option.MaxSize < 1 {
		return nil, errors.New("size must > 0")
	}
	if option.New == nil {
		return nil, errors.New("not found new func")
	}
	ctx, cnl := context.WithCancel(preCtx)
	client := &Client[T]{
		new:     option.New,
		clear:   option.Clear,
		maxSize: option.MaxSize,
		datas:   chanx.NewClient[T](ctx),
		ctx:     ctx,
		cnl:     cnl,
	}
	for i := 0; i < option.MaxSize; i++ {
		go client.add()
	}
	return client, nil
}
func (obj *Client[T]) add() {
	if obj.Err != nil {
		return
	}
	if val, err := obj.new(obj.ctx); err == nil {
		obj.datas.Add(val)
	} else {
		obj.cnl()
		obj.Err = err
	}
}

func (obj *Client[T]) Get(ctx context.Context) (T, bool, error) {
	var t T
	if obj.Err != nil {
		return t, false, obj.Err
	}
	if ctx == nil {
		ctx = obj.ctx
	}
	for {
		select {
		case <-obj.ctx.Done():
			return t, false, obj.ctx.Err()
		case <-ctx.Done():
			return t, false, ctx.Err()
		case data := <-obj.datas.Chan():
			go obj.add()
			if obj.clear == nil || !obj.clear(ctx, data) {
				return data, true, nil
			}
		default:
			return t, false, nil
		}
	}
}
