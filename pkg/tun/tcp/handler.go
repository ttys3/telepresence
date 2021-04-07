package tcp

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/telepresenceio/telepresence/v2/pkg/tun/ip"

	"github.com/telepresenceio/telepresence/rpc/v2/manager"

	"golang.org/x/sys/unix"

	"github.com/datawire/dlib/dlog"
	"github.com/datawire/dlib/dtime"
	"github.com/telepresenceio/telepresence/v2/pkg/connpool"
	"github.com/telepresenceio/telepresence/v2/pkg/tun/buffer"
)

type state int32

const (
	// simplified server-side tcp states
	stateSynReceived = state(iota)
	stateEstablished
	stateFinWait1
	stateFinWait2
	stateTimedWait
	stateIdle
)

func (s state) String() (txt string) {
	switch s {
	case stateIdle:
		txt = "IDLE"
	case stateSynReceived:
		txt = "SYN RECEIVED"
	case stateEstablished:
		txt = "ESTABLISHED"
	case stateFinWait1:
		txt = "FIN_WAIT_1"
	case stateFinWait2:
		txt = "FIN_WAIT_2"
	case stateTimedWait:
		txt = "TIMED WAIT"
	default:
		panic("unknown state")
	}
	return txt
}

const maxReceiveWindow = 65535
const maxSendWindow = 65535
const ioChannelSize = 0x40

type queueElement struct {
	sequence uint32
	retries  int32
	cTime    time.Time
	packet   Packet
	next     *queueElement
}

type quitReason int

const (
	pleaseContinue = quitReason(iota)
	quitByContext
	quitByReset
	quitByUs
	quitByPeer
	quitByBoth
)

type PacketHandler interface {
	connpool.Handler

	// HandlePacket handles a packet that was read from the TUN device
	HandlePacket(ctx context.Context, pkt Packet)
}

type handler struct {
	*connpool.Stream

	// id identifies this connection. It contains source and destination IPs and ports
	id connpool.ConnID

	// remove is the function that removes this instance from the pool
	remove func()

	// TUN channels
	ToTun   chan<- ip.Packet
	fromTun chan Packet
	fromMgr chan *manager.ConnMessage

	// the dispatcher signals its intent to close in dispatcherClosing. 0 == running, 1 == closing, 2 == closed
	dispatcherClosing *int32

	// Channel to use when sending messages to the traffic-manager
	toMgr chan Packet

	// queue where unacked elements are placed until they are acked
	ackWaitQueue     *queueElement
	ackWaitQueueLock sync.Mutex

	// oooQueue is where out-of-order packages are placed until they can be processed
	oooQueue *queueElement

	// synPacket is the initial syn packet received on a connect request. It is
	// dropped once the manager responds to the connect attempt
	synPacket Packet

	// wfState is the current workflow state
	wfState state

	// seq is the sequence that we provide in the packages we send to TUN
	seq uint32

	// lastAck is the last ackNumber that we received from TUN
	lastAck uint32

	// finalSeq is the ack sent with FIN when a connection is closing.
	finalSeq uint32

	// sendWnd and rcvWnd controls the size of the send and receive window
	sendWnd int32
	rcvWnd  int32
}

func NewHandler(ctx context.Context, wg *sync.WaitGroup,
	tcpStream *connpool.Stream, dispatcherClosing *int32, toTun chan<- ip.Packet, id connpool.ConnID, remove func()) PacketHandler {
	h := &handler{
		Stream:            tcpStream,
		id:                id,
		remove:            remove,
		ToTun:             toTun,
		fromMgr:           make(chan *manager.ConnMessage, ioChannelSize),
		dispatcherClosing: dispatcherClosing,
		fromTun:           make(chan Packet, ioChannelSize),
		toMgr:             make(chan Packet, ioChannelSize),
		sendWnd:           int32(maxSendWindow),
		rcvWnd:            int32(maxReceiveWindow),
		wfState:           stateIdle,
	}
	go h.run(ctx, wg)
	return h
}

func (h *handler) HandlePacket(ctx context.Context, pkt Packet) {
	select {
	case <-ctx.Done():
	case h.fromTun <- pkt:
	}
}

func (h *handler) HandleControl(ctx context.Context, ctrl *connpool.ControlMessage) {
	switch ctrl.Code {
	case connpool.ConnectOK:
		synPacket := h.synPacket
		h.synPacket = nil
		if synPacket != nil {
			defer synPacket.Release()
			h.sendSyn(ctx, synPacket)
		}
	case connpool.ConnectReject:
		synPacket := h.synPacket
		h.synPacket = nil
		if synPacket != nil {
			synPacket.Release()
		}
	}
}

func (h *handler) HandleMessage(ctx context.Context, cm *manager.ConnMessage) {
	select {
	case <-ctx.Done():
	case h.fromMgr <- cm:
	}
}

func (h *handler) Close(ctx context.Context) {
	if h.state() == stateEstablished {
		h.setState(ctx, stateFinWait1)
		h.sendFin(ctx, true)
	}
}

func (h *handler) run(ctx context.Context, wg *sync.WaitGroup) {
	defer func() {
		_ = h.sendConnControl(ctx, connpool.Disconnect)
		h.remove()
		wg.Done()
	}()
	go h.processResends(ctx)
	h.processPackets(ctx)
}

func (h *handler) adjustReceiveWindow() {
	queueSize := len(h.toMgr)
	windowSize := maxReceiveWindow
	if queueSize > ioChannelSize/4 {
		windowSize -= queueSize * (maxReceiveWindow / ioChannelSize)
	}
	h.setReceiveWindow(uint16(windowSize))
}

func (h *handler) sendToMgr(ctx context.Context, pkt Packet) bool {
	select {
	case h.toMgr <- pkt:
		h.adjustReceiveWindow()
		return true
	case <-ctx.Done():
		return false
	}
}

func (h *handler) sendToTun(ctx context.Context, pkt Packet) {
	select {
	case <-ctx.Done():
	case h.ToTun <- pkt:
	}
}

func (h *handler) newResponse(ipPlayloadLen int, withAck bool) Packet {
	pkt := NewPacket(ipPlayloadLen, h.id.Destination(), h.id.Source(), withAck)
	ipHdr := pkt.IPHeader()
	ipHdr.SetL4Protocol(unix.IPPROTO_TCP)
	ipHdr.SetChecksum()

	tcpHdr := Header(ipHdr.Payload())
	tcpHdr.SetDataOffset(5)
	tcpHdr.SetSourcePort(h.id.DestinationPort())
	tcpHdr.SetDestinationPort(h.id.SourcePort())
	tcpHdr.SetWindowSize(h.receiveWindow())
	return pkt
}

func (h *handler) sendAck(ctx context.Context) {
	pkt := h.newResponse(HeaderLen, false)
	tcpHdr := pkt.Header()
	tcpHdr.SetACK(true)
	tcpHdr.SetSequence(h.sequence())
	tcpHdr.SetAckNumber(h.sequenceLastAcked())
	tcpHdr.SetChecksum(pkt.IPHeader())
	h.sendToTun(ctx, pkt)
}

func (h *handler) sendFin(ctx context.Context, expectAck bool) {
	pkt := h.newResponse(HeaderLen, true)
	tcpHdr := pkt.Header()
	tcpHdr.SetFIN(true)
	tcpHdr.SetACK(true)
	tcpHdr.SetSequence(h.sequence())
	tcpHdr.SetAckNumber(h.sequenceLastAcked())
	tcpHdr.SetChecksum(pkt.IPHeader())
	if expectAck {
		h.pushToAckWait(1, pkt)
		h.finalSeq = h.sequence()
	}
	h.sendToTun(ctx, pkt)
}

func (h *handler) sendSyn(ctx context.Context, syn Packet) {
	synHdr := syn.Header()
	if !synHdr.SYN() {
		return
	}
	hl := HeaderLen
	if synHdr.ECE() {
		hl += 4
	}
	pkt := h.newResponse(hl, true)
	tcpHdr := pkt.Header()
	tcpHdr.SetSYN(true)
	tcpHdr.SetACK(true)
	tcpHdr.SetSequence(h.sequence())
	tcpHdr.SetAckNumber(synHdr.Sequence() + 1)
	if synHdr.ECE() {
		tcpHdr.SetDataOffset(6)
		opts := tcpHdr.OptionBytes()
		opts[0] = 2
		opts[1] = 4
		binary.BigEndian.PutUint16(opts[2:], uint16(buffer.DataPool.MTU-HeaderLen))
	}
	tcpHdr.SetChecksum(pkt.IPHeader())

	h.setSequenceLastAcked(tcpHdr.AckNumber())
	h.sendToTun(ctx, pkt)
	h.pushToAckWait(1, pkt)
}

// writeToTunLoop sends the packages read from the toMgr channel to the traffic-manager device
func (h *handler) writeToMgrLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case pkt := <-h.toMgr:
			h.adjustReceiveWindow()
			dlog.Debugf(ctx, "-> MGR %s", pkt)
			err := h.SendMsg(&manager.ConnMessage{ConnId: []byte(h.id), Payload: pkt.Header().Payload()})
			if err != nil {
				if ctx.Err() == nil && atomic.LoadInt32(h.dispatcherClosing) == 0 && h.state() < stateFinWait2 {
					dlog.Errorf(ctx, "failed to write to dispatcher's remote endpoint: %v", err)
				}
				return
			}
			pkt.Release()
		}
	}
}

// writeToTunLoop sends the packages read from the fromMgr channel to the TUN device
func (h *handler) writeToTunLoop(ctx context.Context) {
	for {
		window := h.sendWindow()
		if window == 0 {
			// The intended receiver is currently not accepting data. We must
			// wait for the window to increase.
			dlog.Debugf(ctx, "%s TCP window is zero", h.id)
			for window == 0 {
				dtime.SleepWithContext(ctx, 10*time.Microsecond)
				window = h.sendWindow()
			}
		}

		var cm *manager.ConnMessage
		select {
		case <-ctx.Done():
			return
		case cm = <-h.fromMgr:
		}
		n := len(cm.Payload)
		pkt := h.newResponse(HeaderLen+n, true)
		ipHdr := pkt.IPHeader()
		tcpHdr := pkt.Header()
		ipHdr.SetPayloadLen(HeaderLen + n)
		ipHdr.SetChecksum()

		tcpHdr.SetACK(true)
		tcpHdr.SetPSH(true)
		tcpHdr.SetSequence(h.sequence())
		tcpHdr.SetAckNumber(h.sequenceLastAcked())
		copy(tcpHdr.Payload(), cm.Payload)
		tcpHdr.SetChecksum(ipHdr)

		h.sendToTun(ctx, pkt)
		h.pushToAckWait(uint32(n), pkt)

		// Decrease the window size with the bytes that we just sent unless it's already updated
		// from a received package
		window -= window - uint16(n)
		atomic.CompareAndSwapInt32(&h.sendWnd, int32(window), int32(window))
	}
}

func (h *handler) sendConnControl(ctx context.Context, code connpool.ControlCode) error {
	pkt := connpool.ConnControl(h.id, code)
	dlog.Debugf(ctx, "-> MGR %s, code %s", h.id, code)
	if err := h.SendMsg(pkt); err != nil {
		return fmt.Errorf("failed to send control package: %v", err)
	}
	return nil
}

func (h *handler) idle(ctx context.Context, syn Packet) quitReason {
	h.synPacket = syn
	if err := h.sendConnControl(ctx, connpool.Connect); err != nil {
		dlog.Error(ctx, err)
		h.synPacket = nil
		h.sendToTun(ctx, syn.Reset())
		return quitByUs
	}
	h.setSequence(1)
	h.setState(ctx, stateSynReceived)
	return pleaseContinue
}

func (h *handler) synReceived(ctx context.Context, pkt Packet) quitReason {
	release := true
	defer func() {
		if release {
			pkt.Release()
		}
	}()

	tcpHdr := pkt.Header()
	if tcpHdr.RST() {
		return quitByReset
	}
	if !tcpHdr.ACK() {
		return pleaseContinue
	}

	h.ackReceived(tcpHdr.AckNumber())
	h.setState(ctx, stateEstablished)
	go h.writeToMgrLoop(ctx)
	go h.writeToTunLoop(ctx)

	pl := len(tcpHdr.Payload())
	h.setSequenceLastAcked(tcpHdr.Sequence() + uint32(pl))
	if pl != 0 {
		h.setSequence(h.sequence() + uint32(pl))
		h.pushToAckWait(uint32(pl), pkt)
		if !h.sendToMgr(ctx, pkt) {
			return quitByContext
		}
		release = false
	}
	return pleaseContinue
}

func (h *handler) handleReceived(ctx context.Context, pkt Packet) quitReason {
	state := h.state()
	release := true
	defer func() {
		if release {
			pkt.Release()
		}
	}()

	tcpHdr := pkt.Header()
	if tcpHdr.RST() {
		h.setState(ctx, stateIdle)
		return quitByReset
	}

	if !tcpHdr.ACK() {
		// Just ignore packages that has no ack
		return pleaseContinue
	}

	ackNbr := tcpHdr.AckNumber()
	h.ackReceived(ackNbr)
	if state == stateTimedWait {
		h.setState(ctx, stateIdle)
		return quitByPeer
	}

	sq := tcpHdr.Sequence()
	lastAck := h.sequenceLastAcked()
	switch {
	case sq == lastAck:
		if state == stateFinWait1 && ackNbr == h.finalSeq && !tcpHdr.FIN() {
			h.setState(ctx, stateFinWait2)
			return pleaseContinue
		}
	case sq > lastAck:
		// Oops. Package loss! Let sender know by sending an ACK so that we ack the receipt
		// and also tell the sender about our expected number
		h.sendAck(ctx)
		h.addOutOfOrderPackage(ctx, pkt)
		release = false
		return pleaseContinue
	default:
		// resend of already acknowledged package. Just ignore
		return pleaseContinue
	}
	if tcpHdr.RST() {
		return quitByReset
	}

	switch {
	case len(tcpHdr.Payload()) > 0:
		h.setSequenceLastAcked(lastAck + uint32(len(tcpHdr.Payload())))
		if !h.sendToMgr(ctx, pkt) {
			return quitByContext
		}
		release = false
	case tcpHdr.FIN():
		h.setSequenceLastAcked(lastAck + 1)
	default:
		// don't ack acks
		return pleaseContinue
	}
	h.sendAck(ctx)

	switch state {
	case stateEstablished:
		if tcpHdr.FIN() {
			h.sendFin(ctx, false)
			h.setState(ctx, stateTimedWait)
			return quitByPeer
		}
	case stateFinWait1:
		if tcpHdr.FIN() {
			h.setState(ctx, stateTimedWait)
			return quitByBoth
		}
		h.setState(ctx, stateFinWait2)
	case stateFinWait2:
		if tcpHdr.FIN() {
			return quitByUs
		}
	}
	return pleaseContinue
}

func (h *handler) processPackets(ctx context.Context) {
	for {
		select {
		case pkt := <-h.fromTun:
			if !h.processPacket(ctx, pkt) {
				return
			}
			for {
				continueProcessing, next := h.processNextOutOfOrderPackage(ctx)
				if !continueProcessing {
					return
				}
				if !next {
					break
				}
			}
		case <-ctx.Done():
			h.setState(ctx, stateIdle)
			return
		}
	}
}

func (h *handler) processPacket(ctx context.Context, pkt Packet) bool {
	h.setSendWindow(pkt.Header().WindowSize())
	var end quitReason
	switch h.state() {
	case stateIdle:
		end = h.idle(ctx, pkt)
	case stateSynReceived:
		end = h.synReceived(ctx, pkt)
	default:
		end = h.handleReceived(ctx, pkt)
	}
	switch end {
	case quitByReset, quitByContext:
		h.setState(ctx, stateIdle)
		return false
	case quitByUs, quitByPeer, quitByBoth:
		func() {
			ctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			h.processPackets(ctx)
		}()
		return false
	default:
		return true
	}
}

const initialResendDelay = 2
const maxResends = 7

func (h *handler) processResends(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			now := time.Now()
			h.ackWaitQueueLock.Lock()
			var prev *queueElement
			for el := h.ackWaitQueue; el != nil; {
				secs := initialResendDelay << el.retries // 2, 4, 8, 16, ...
				deadLine := el.cTime.Add(time.Duration(secs) * time.Second)
				if deadLine.Before(now) {
					el.retries++
					if el.retries > maxResends {
						dlog.Errorf(ctx, "package resent %d times, giving up", maxResends)
						// Drop from queue and point to next
						el = el.next
						if prev == nil {
							h.ackWaitQueue = el
						} else {
							prev.next = el
						}
						continue
					}
					dlog.Debugf(ctx, "Resending %s after having waited for %d seconds", el.packet, secs)
					h.sendToTun(ctx, el.packet)
				}
				prev = el
				el = el.next
			}
			h.ackWaitQueueLock.Unlock()
		}
	}
}

func (h *handler) pushToAckWait(seqAdd uint32, pkt Packet) {
	h.ackWaitQueueLock.Lock()
	h.ackWaitQueue = &queueElement{
		sequence: h.addSequence(seqAdd),
		cTime:    time.Now(),
		packet:   pkt,
		next:     h.ackWaitQueue,
	}
	h.ackWaitQueueLock.Unlock()
}

func (h *handler) ackReceived(seq uint32) {
	h.ackWaitQueueLock.Lock()
	var prev *queueElement
	for el := h.ackWaitQueue; el != nil && el.sequence <= seq; el = el.next {
		if prev != nil {
			prev.next = el.next
		} else {
			h.ackWaitQueue = el.next
		}
		prev = el
		el.packet.Release()
	}
	h.ackWaitQueueLock.Unlock()
}

func (h *handler) processNextOutOfOrderPackage(ctx context.Context) (bool, bool) {
	seq := h.sequenceLastAcked()
	var prev *queueElement
	for el := h.oooQueue; el != nil; el = el.next {
		if el.sequence == seq {
			if prev != nil {
				prev.next = el.next
			} else {
				h.oooQueue = el.next
			}
			dlog.Debugf(ctx, "Processing out-of-order package %s", el.packet)
			return h.processPacket(ctx, el.packet), true
		}
		prev = el
	}
	return true, false
}

func (h *handler) addOutOfOrderPackage(ctx context.Context, pkt Packet) {
	dlog.Debugf(ctx, "Keeping out-of-order package %s", pkt)
	h.oooQueue = &queueElement{
		sequence: pkt.Header().Sequence(),
		cTime:    time.Now(),
		packet:   pkt,
		next:     h.oooQueue,
	}
}

func (h *handler) state() state {
	return state(atomic.LoadInt32((*int32)(&h.wfState)))
}

func (h *handler) setState(_ context.Context, s state) {
	// dlog.Debugf(ctx, "state %s -> %s", c.state(), s)
	atomic.StoreInt32((*int32)(&h.wfState), int32(s))
}

// sequence is the sequence number of the packages that this client
// sends to the TUN device.
func (h *handler) sequence() uint32 {
	return atomic.LoadUint32(&h.seq)
}

func (h *handler) addSequence(v uint32) uint32 {
	return atomic.AddUint32(&h.seq, v)
}

func (h *handler) setSequence(v uint32) {
	atomic.StoreUint32(&h.seq, v)
}

// sequenceLastAcked is the last received sequence that this client has ACKed
func (h *handler) sequenceLastAcked() uint32 {
	return atomic.LoadUint32(&h.lastAck)
}

func (h *handler) setSequenceLastAcked(v uint32) {
	atomic.StoreUint32(&h.lastAck, v)
}

func (h *handler) sendWindow() uint16 {
	return uint16(atomic.LoadInt32(&h.sendWnd))
}

func (h *handler) setSendWindow(v uint16) {
	atomic.StoreInt32(&h.sendWnd, int32(v))
}

func (h *handler) receiveWindow() uint16 {
	return uint16(atomic.LoadInt32(&h.rcvWnd))
}

func (h *handler) setReceiveWindow(v uint16) {
	atomic.StoreInt32(&h.rcvWnd, int32(v))
}
