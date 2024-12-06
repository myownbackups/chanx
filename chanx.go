package chanx

import (
	"container/list"
	"context"
	"sync"
	"time"
)

type Client[T any] struct {
	pip   chan T
	buf   *list.List
	ctx   context.Context
	cnl   context.CancelFunc
	ctx2  context.Context
	cnl2  context.CancelFunc
	lock  sync.RWMutex
	note  chan struct{}
	timer *time.Timer
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
		note: make(chan struct{}),
	}
	go client.run()
	return client
}
func (obj *Client[T]) Add(val T) error {
	select {
	case <-obj.ctx.Done():
		return context.Cause(obj.ctx)
	case <-obj.ctx2.Done():
		return context.Cause(obj.ctx2)
	default:
		obj.push(val)
		select {
		case obj.note <- struct{}{}:
		default:
		}
		return nil
	}
}
func (obj *Client[T]) Chan() <-chan T {
	return obj.pip
}
func (obj *Client[T]) push(val T) {
	obj.lock.Lock()
	defer obj.lock.Unlock()
	obj.buf.PushBack(val)
}
func (obj *Client[T]) get() (T, bool) {
	obj.lock.Lock()
	defer obj.lock.Unlock()
	v, o := obj.buf.Remove(obj.buf.Front()).(T)
	return v, o
}
func (obj *Client[T]) Len() int {
	obj.lock.Lock()
	defer obj.lock.Unlock()
	return obj.buf.Len()
}

func (obj *Client[T]) send() error {
	for obj.buf.Len() > 0 {
		if remVal, ok := obj.get(); ok {
			select {
			case obj.pip <- remVal:
			case <-obj.ctx2.Done():
				return context.Cause(obj.ctx2)
			}
		}
	}
	return nil
}
func (obj *Client[T]) run() {
	defer obj.Close()
	for {
		if obj.timer == nil {
			obj.timer = time.NewTimer(time.Second * 5)
		} else {
			obj.timer.Reset(time.Second * 5)
		}
		select {
		case <-obj.ctx2.Done():
			return
		case <-obj.ctx.Done():
			obj.send()
			return
		case <-obj.note:
			if err := obj.send(); err != nil {
				return
			}
		case <-obj.timer.C:
			if err := obj.send(); err != nil {
				return
			}
		}
	}
}
func (obj *Client[T]) JoinClose() { //等待消费完毕后，关闭
	obj.cnl()
	<-obj.ctx2.Done()
}
func (obj *Client[T]) Close() { //立刻关闭
	obj.cnl()
	obj.cnl2()
	if obj.timer != nil {
		obj.timer.Stop()
	}
}
func (obj *Client[T]) Ctx() context.Context {
	return obj.ctx2
}
