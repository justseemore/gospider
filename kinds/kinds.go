package kinds

import (
	"sync"
)

type Set[T comparable] struct {
	data map[T]struct{}
	lock sync.RWMutex
}

// 新建客户端
func NewSet[T comparable](strs ...T) *Set[T] {
	list := &Set[T]{data: map[T]struct{}{}}
	for _, str := range strs {
		list.Add(str)
	}
	return list
}

// 添加元素
func (obj *Set[T]) Add(value T) {
	obj.lock.Lock()
	obj.data[value] = struct{}{}
	obj.lock.Unlock()
}

// 删除元素
func (obj *Set[T]) Del(value T) bool {
	obj.lock.Lock()
	delete(obj.data, value)
	obj.lock.Unlock()
	return false
}

// 判断元素是否存在
func (obj *Set[T]) Has(value T) bool {
	obj.lock.RLock()
	_, ok := obj.data[value]
	obj.lock.RUnlock()
	return ok
}

// 返回元素长度
func (obj *Set[T]) Len() int {
	return len(obj.data)
}

// 重置
func (obj *Set[T]) ReSet() {
	obj.data = make(map[T]struct{})
}

// 返回数组
func (obj *Set[T]) Array() []T {
	result := make([]T, obj.Len())
	var i int
	for val := range obj.data {
		result[i] = val
		i++
	}
	return result
}
func (obj *Set[T]) Map() map[T]struct{} {
	return obj.data
}
