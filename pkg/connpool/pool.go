package connpool

import (
	"context"
	"sync"

	"github.com/telepresenceio/telepresence/rpc/v2/manager"

	"github.com/datawire/dlib/dlog"
)

type Pool struct {
	handlers map[ConnID]Handler
	lock     sync.Mutex
}

type Handler interface {
	// Close closes the handle
	Close(context.Context)

	HandleControl(ctx context.Context, ctrl *ControlMessage)

	HandleMessage(ctx context.Context, message *manager.ConnMessage)
}

func NewPool() *Pool {
	return &Pool{handlers: make(map[ConnID]Handler)}
}

func (p *Pool) release(id ConnID) {
	p.lock.Lock()
	// TODO: Consider moving to freelist for reuse
	delete(p.handlers, id)
	p.lock.Unlock()
}

func (p *Pool) Get(ctx context.Context, id ConnID, createHandler func(ctx context.Context, release func()) (Handler, error)) (Handler, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	handler, ok := p.handlers[id]
	if ok || createHandler == nil {
		return handler, nil
	}

	var err error
	handler, err = createHandler(ctx, func() {
		p.release(id)
	})
	if err == nil {
		p.handlers[id] = handler
	}
	return handler, err
}

func (p *Pool) CloseAll(ctx context.Context) {
	p.lock.Lock()
	defer p.lock.Unlock()
	for id, handler := range p.handlers {
		dlog.Debugf(ctx, "Closing handler %s", id)
		handler.Close(ctx)
	}
}
