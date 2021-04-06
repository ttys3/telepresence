package udp

import (
	"context"
	"sync"
	"time"

	"github.com/telepresenceio/telepresence/v2/pkg/tun/ip"

	"github.com/telepresenceio/telepresence/rpc/v2/manager"

	"github.com/telepresenceio/telepresence/v2/pkg/connpool"

	"github.com/datawire/dlib/dlog"
)

type Handler struct {
	*connpool.Stream
	id        connpool.ConnID
	remove    func()
	toTun     chan<- ip.Packet
	fromTun   chan Datagram
	idleTimer *time.Timer
}

func (c *Handler) Close(_ context.Context) {
}

const ioChannelSize = 0x40
const idleDuration = time.Second

func (c *Handler) Run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	c.idleTimer = time.AfterFunc(idleDuration, func() {
		c.remove()
		cancel()
	})
	go c.writerLoop(ctx)
	<-ctx.Done()
}

func (c *Handler) NewDatagram(ctx context.Context, dg Datagram) {
	select {
	case <-ctx.Done():
	case c.fromTun <- dg:
	}
}

func NewHandler(stream *connpool.Stream, toTun chan<- ip.Packet, id connpool.ConnID, remove func()) *Handler {
	return &Handler{
		Stream:  stream,
		id:      id,
		toTun:   toTun,
		remove:  remove,
		fromTun: make(chan Datagram, ioChannelSize),
	}
}

func (c *Handler) HandleControl(_ context.Context, _ *connpool.ControlMessage) {

}

func (c *Handler) HandleMessage(ctx context.Context, mdg *manager.ConnMessage) {
	pkt := NewDatagram(HeaderLen+len(mdg.Payload), c.id.Destination(), c.id.Source())
	ipHdr := pkt.IPHeader()
	ipHdr.SetChecksum()

	udpHdr := Header(ipHdr.Payload())
	udpHdr.SetSourcePort(c.id.DestinationPort())
	udpHdr.SetDestinationPort(c.id.SourcePort())
	udpHdr.SetPayloadLen(uint16(len(mdg.Payload)))
	copy(udpHdr.Payload(), mdg.Payload)
	udpHdr.SetChecksum(ipHdr)

	dlog.Debugf(ctx, "<- MGR %s", pkt)
	select {
	case <-ctx.Done():
		return
	case c.toTun <- pkt:
	}
}

func (c *Handler) writerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case dg := <-c.fromTun:
			c.idleTimer.Reset(idleDuration)

			udpHdr := dg.Header()
			dlog.Debugf(ctx, "-> MGR %s", dg)
			err := c.SendMsg(&manager.ConnMessage{ConnId: []byte(c.id), Payload: udpHdr.Payload()})
			dg.SoftRelease()
			if err != nil {
				if ctx.Err() == nil {
					dlog.Error(ctx, err)
				}
				return
			}
		}
	}
}
