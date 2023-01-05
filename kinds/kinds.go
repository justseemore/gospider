package kinds

import (
	"sync"
	"sync/atomic"
)

type Set[T comparable] struct {
	data sync.Map
	l    atomic.Int64
}

// 新建客户端
func NewSet[T comparable](strs ...T) *Set[T] {
	list := new(Set[T])
	for _, str := range strs {
		list.Add(str)
	}
	return list
}

// 添加元素
func (obj *Set[T]) Add(value T) bool {
	_, ok := obj.data.LoadOrStore(value, struct{}{})
	if !ok {
		obj.l.Add(1)
	}
	return !ok
}

// 删除元素
func (obj *Set[T]) Rem(value T) bool {
	obj.l.Add(-1)
	obj.data.Delete(value)
	return false
}

// 判断元素是否存在
func (obj *Set[T]) Has(value T) bool {
	_, ok := obj.data.Load(value)
	return ok
}

// 返回元素长度
func (obj *Set[T]) Len() int64 {
	return obj.l.Load()
}

// 重置
func (obj *Set[T]) ReSet() {
	obj.data = sync.Map{}
	obj.l = atomic.Int64{}
}

// 返回数组
func (obj *Set[T]) Array() []T {
	result := make([]T, obj.Len())
	var i int64
	obj.data.Range(func(key, value any) bool {
		result[i] = key.(T)
		i++
		return true
	})
	return result
}
