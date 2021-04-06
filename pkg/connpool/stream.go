package connpool

import (
	"context"
	"fmt"

	"github.com/datawire/dlib/dlog"

	"google.golang.org/grpc"

	"github.com/telepresenceio/telepresence/rpc/v2/manager"
)

type Stream struct {
	grpc.ClientStream
	pool *Pool
}

func NewStream(bidiStream grpc.ClientStream, pool *Pool) *Stream {
	return &Stream{ClientStream: bidiStream, pool: pool}
}

// ReadLoop reads replies from the stream and dispatches them to the correct connection
// based on the message id.
func (s *Stream) ReadLoop(ctx context.Context) error {
	for {
		cm := new(manager.ConnMessage)
		err := s.RecvMsg(cm)

		if err != nil {
			return fmt.Errorf("read from grpc.ClientStream failed: %s", err)
		}
		if IsControlMessage(cm) {
			ctrl, err := NewControlMessage(cm)
			if err != nil {
				dlog.Error(ctx, err)
				continue
			}

			dlog.Debugf(ctx, "<- MGR %s, code %s", ctrl.ID.ReplyString(), ctrl.Code)
			if conn, _ := s.pool.Get(ctx, ctrl.ID, nil); conn != nil {
				conn.HandleControl(ctx, ctrl)
			} else {
				dlog.Error(ctx, "control packet lost because no connection was active")
			}
			continue
		}
		id := ConnID(cm.ConnId)
		dlog.Debugf(ctx, "<- MGR %s, len %d", id.ReplyString(), len(cm.Payload))
		if conn, _ := s.pool.Get(ctx, id, nil); conn != nil {
			conn.HandleMessage(ctx, cm)
		}
	}
}
