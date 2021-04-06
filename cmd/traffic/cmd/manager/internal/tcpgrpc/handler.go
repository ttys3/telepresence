package tcpgrpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/datawire/dlib/dlog"
	rpc "github.com/telepresenceio/telepresence/rpc/v2/manager"
	"github.com/telepresenceio/telepresence/v2/pkg/connpool"
)

// The idleDuration controls how long a handler remains alive without reading or writing any messages
const idleDuration = 2 * time.Minute

// The Handler takes care of dispatching messages between gRPC and UDP connections
type Handler struct {
	id        connpool.ConnID
	ctx       context.Context
	cancel    context.CancelFunc
	release   func()
	server    rpc.Manager_ConnTunnelServer
	incoming  chan *rpc.ConnMessage
	conn      *net.TCPConn
	idleTimer *time.Timer
}

// NewHandler creates a new handler that dispatches messages in both directions between the given gRPC server
// and the destination identified by the given connID.
//
// The handler remains active until it's been idle for idleDuration, at which time it will automatically close
// and call the release function it got from the connpool.Pool to ensure that it gets properly released.
func NewHandler(ctx context.Context, connID connpool.ConnID, server rpc.Manager_ConnTunnelServer, release func()) (connpool.Handler, error) {
	handler := &Handler{
		id:       connID,
		server:   server,
		release:  release,
		incoming: make(chan *rpc.ConnMessage, 10),
	}

	// Set up the idle timer to close and release this handler when it's been idle for a while.
	handler.ctx, handler.cancel = context.WithCancel(ctx)
	handler.idleTimer = time.AfterFunc(idleDuration, func() {
		handler.release()
		handler.Close(handler.ctx)
	})
	return handler, nil
}

func (h *Handler) HandleControl(ctx context.Context, cm *connpool.ControlMessage) {
	var reply connpool.ControlCode
	switch cm.Code {
	case connpool.Connect:
		if h.conn != nil {
			break
		}
		destAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", cm.ID.Destination(), cm.ID.DestinationPort()))
		if err != nil {
			dlog.Errorf(ctx, "connection %s unable to resolve destination address: %v", cm.ID, err)
			reply = connpool.ConnectReject
			break
		}
		conn, err := net.DialTCP("tcp4", nil, destAddr)
		if err != nil {
			dlog.Errorf(ctx, "connection %s failed: %v", cm.ID, err)
			reply = connpool.ConnectReject
			break
		}
		h.conn = conn
		reply = connpool.ConnectOK
		go h.readLoop()
		go h.writeLoop()
	case connpool.Disconnect:
		if h.idleTimer.Stop() {
			h.release()
			h.Close(ctx)
		}
		reply = connpool.DisconnectOK
	default:
		dlog.Errorf(ctx, "unhandled connection control message: %s", cm)
		return
	}
	if err := h.sendTCD(reply); err != nil {
		dlog.Error(ctx, err)
	}
}

// Send a package to the underlying TCP connection
func (h *Handler) HandleMessage(ctx context.Context, dg *rpc.ConnMessage) {
	select {
	case <-ctx.Done():
		return
	case h.incoming <- dg:
	}
}

// Close will close the underlying TCP connection
func (h *Handler) Close(_ context.Context) {
	conn := h.conn
	h.conn = nil
	if conn != nil {
		_ = conn.Close()
	}
	h.cancel()
}

func (h *Handler) sendTCD(code connpool.ControlCode) error {
	err := h.server.Send(connpool.ConnControl(h.id, code))
	if err != nil {
		err = fmt.Errorf("failed to send control message: %v", err)
	}
	return err
}

func (h *Handler) readLoop() {
	b := make([]byte, 0x8000)
	for h.ctx.Err() == nil {
		n, err := h.conn.Read(b)
		if err != nil {
			h.idleTimer.Stop()
			if err = h.sendTCD(connpool.ReadClosed); err != nil {
				dlog.Error(h.ctx, err)
			}
			return
		}
		// dlog.Debugf(ctx, "%s read TCP package of size %d", uh.id, n)
		if !h.idleTimer.Reset(idleDuration) {
			// Timer had already fired. Prevent that it fires again. We're done here.
			h.idleTimer.Stop()
			return
		}
		if n > 0 {
			dlog.Debugf(h.ctx, "-> CLI %s, len %d", h.id.ReplyString(), n)
			if err = h.server.Send(&rpc.ConnMessage{ConnId: []byte(h.id), Payload: b[:n]}); err != nil {
				return
			}
		}
	}
}

func (h *Handler) writeLoop() {
	for {
		select {
		case <-h.ctx.Done():
			return
		case dg := <-h.incoming:
			dlog.Debugf(h.ctx, "<- CLI %s, len %d", h.id, len(dg.Payload))
			if !h.idleTimer.Reset(idleDuration) {
				// Timer had already fired. Prevent that it fires again. We're done here.
				h.idleTimer.Stop()
				return
			}

			pn := len(dg.Payload)
			for n := 0; n < pn; {
				wn, err := h.conn.Write(dg.Payload[n:])
				if err != nil && h.ctx.Err() == nil {
					dlog.Errorf(h.ctx, "%s failed to write TCP: %v", h.id, err)
				}
				n += wn
			}
		}
	}
}
