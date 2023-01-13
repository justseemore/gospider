package pool

import (
	"context"
	"errors"

	"gitee.com/baixudong/gospider/chanx"
)

type Client[T any] struct {
	new     func() T
	clear   func(T) bool
	datas   *chanx.Client[T]
	maxSize int
}
type ClientOption[T any] struct {
	New     func() T
	Clear   func(T) bool
	MaxSize int
}

func NewClient[T any](ctx context.Context, option ClientOption[T]) (*Client[T], error) {
	if option.MaxSize < 1 {
		return nil, errors.New("size must > 0")
	}
	if option.New == nil {
		return nil, errors.New("not found new func")
	}
	client := &Client[T]{
		new:     option.New,
		clear:   option.Clear,
		maxSize: option.MaxSize,
		datas:   chanx.NewClient[T](ctx),
	}
	for i := 0; i < option.MaxSize; i++ {
		go client.add()
	}
	return client, nil
}
func (obj *Client[T]) add() {
	obj.datas.Add(obj.new())
}

func (obj *Client[T]) Get(ctx context.Context) (T, bool, error) {
	var t T
	for {
		select {
		case <-ctx.Done():
			return t, false, ctx.Err()
		case data := <-obj.datas.Chan():
			go obj.add()
			if obj.clear == nil || !obj.clear(data) {
				return data, true, nil
			}
		default:
			return t, false, nil
		}
	}
}
