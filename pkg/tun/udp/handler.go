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

type DatagramHandler interface {
	connpool.Handler
	NewDatagram(ctx context.Context, dg Datagram)
}

type handler struct {
	*connpool.Stream
	id        connpool.ConnID
	remove    func()
	toTun     chan<- ip.Packet
	fromTun   chan Datagram
	idleTimer *time.Timer
}

const ioChannelSize = 0x40
const idleDuration = time.Second

func (c *handler) NewDatagram(ctx context.Context, dg Datagram) {
	select {
	case <-ctx.Done():
	case c.fromTun <- dg:
	}
}

func (c *handler) Close(_ context.Context) {
}

func NewHandler(ctx context.Context, wg *sync.WaitGroup,
	stream *connpool.Stream, toTun chan<- ip.Packet, id connpool.ConnID, remove func()) DatagramHandler {
	h := &handler{
		Stream:  stream,
		id:      id,
		toTun:   toTun,
		remove:  remove,
		fromTun: make(chan Datagram, ioChannelSize),
	}
	go h.run(ctx, wg, h.writeLoop)
	return h
}

func (c *handler) HandleControl(_ context.Context, _ *connpool.ControlMessage) {
}

func (c *handler) HandleMessage(ctx context.Context, mdg *manager.ConnMessage) {
	pkt := NewDatagram(HeaderLen+len(mdg.Payload), c.id.Destination(), c.id.Source())
	ipHdr := pkt.IPHeader()
	ipHdr.SetChecksum()

	udpHdr := Header(ipHdr.Payload())
	udpHdr.SetSourcePort(c.id.DestinationPort())
	udpHdr.SetDestinationPort(c.id.SourcePort())
	udpHdr.SetPayloadLen(uint16(len(mdg.Payload)))
	copy(udpHdr.Payload(), mdg.Payload)
	udpHdr.SetChecksum(ipHdr)

	select {
	case <-ctx.Done():
		return
	case c.toTun <- pkt:
	}
}

func (c *handler) run(ctx context.Context, wg *sync.WaitGroup, writerLoop func(context.Context)) {
	defer wg.Done()

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	c.idleTimer = time.AfterFunc(idleDuration, func() {
		c.remove()
		cancel()
	})
	go writerLoop(ctx)
	<-ctx.Done()
}

func (c *handler) writeLoop(ctx context.Context) {
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
