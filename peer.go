package gecgosio

import (
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"
	"github.com/rs/xid"
)

// The server's definition of a client
type Peer struct {
	Id                   string
	rooms                map[string]struct{}
	emitReliable         bool
	emitRaw              bool
	interval             int
	runs                 int
	server               *Server
	dataChannel          *webrtc.DataChannel
	peerConnection       *webrtc.PeerConnection
	additionalCandidates []webrtc.ICECandidateInit
	IEventEmitter
}

func createPeer(s *Server, dc *webrtc.DataChannel, pc *webrtc.PeerConnection) *Peer {
	p := &Peer{
		Id:                   xid.New().String(),
		rooms:                make(map[string]struct{}),
		server:               s,
		dataChannel:          dc,
		peerConnection:       pc,
		additionalCandidates: nil,
		IEventEmitter:        CreateEventEmitter(),
	}

	s.peerConnections[p.Id] = p
	return p
}

func (p *Peer) AddCandidate(can webrtc.ICECandidateInit) []webrtc.ICECandidateInit {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to add candidate")
		return []webrtc.ICECandidateInit{}
	}

	p.additionalCandidates = append(p.additionalCandidates, can)
	return p.additionalCandidates
}

// Will make the next Emit call to be reliable
//
// - interval: The interval between each message in ms
//
// - runs: How many times the message should be sent
//
// Example usage:
//
//	peer.Reliable(150, 10).Emit("ping", "hello") // reliable
//	peer.Emit("pong", "world") // no longer reliable
func (p *Peer) Reliable(interval int, runs int) *Peer {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to make reliable")
		return nil
	}

	p.emitReliable = true
	p.interval = interval
	p.runs = runs

	return p
}

// Will make the next emit call be able to send raw bytes
//
// Example usage:
//
//	peer.Raw().Emit(...)
func (p *Peer) Raw() *Peer {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to make reliable")
		return nil
	}

	p.emitRaw = true
	return p
}

// Send a message to peer
//
// - e: The event as a string
//
// - msg: The message as a string (optional)
//
// Example usage:
//
//	peer.Emit("x", "hello") // with message "hello"
//	peer.Emit("y", "hello", "world") // with message "hello, world"
//	peer.Emit("z") // with no message
func (p *Peer) Emit(data any, msg ...string) {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to emit")
		return
	}
	if p.dataChannel == nil {
		fmt.Println("error: peer dataChannel is nil, deleting peer")
		delete(p.server.peerConnections, p.Id)
		return
	}

	// Handle raw bytes
	if bytes, ok := data.([]byte); ok {
		p.dataChannel.Send(bytes)
		return
	}

	// Handle string event
	e, ok := data.(string)
	if !ok {
		fmt.Println("first argument must be string or []byte")
		return
	}

	msgData := strings.Join(msg, ", ")
	startsWithCurly := strings.HasPrefix(msgData, "{")
	endsWithCurly := strings.HasSuffix(msgData, "}")
	jsonObjStr := startsWithCurly && endsWithCurly
	if !jsonObjStr {
		msgData = "\"" + msgData + "\""
	}
	if p.emitReliable {
		p.emitReliable = false
	}
	if p.dataChannel != nil {
		p.dataChannel.SendText(`{"` + e + `":` + msgData + `}`)
	} else {
		fmt.Println("error: peer dataChannel is nil, deleting peer")
		delete(p.server.peerConnections, p.Id)
	}
}

func (p *Peer) Join(ids ...string) {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to join")
		return
	}

	for _, id := range ids {
		if _, ok := p.server.Rooms[id]; !ok {
			p.server.Rooms[id] = make(Room)
		}
		p.server.Rooms[id][p.Id] = p
		p.rooms[id] = struct{}{}
	}
}

func (p *Peer) Leave(ids ...string) {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to leave")
		return
	}

	for _, id := range ids {
		if _, ok := p.server.Rooms[id]; ok {
			if _, ok := p.server.Rooms[id][p.Id]; ok {
				delete(p.server.Rooms[id], p.Id)

				if len(p.server.Rooms[id]) <= 0 {
					delete(p.server.Rooms, id)
				}
			}
			if _, ok := p.rooms[id]; ok {
				delete(p.rooms, id)
			}
		}
	}
}

// Returns a list of roomIds given a peer
func (p *Peer) Rooms() []string {
	rooms := []string{}
	for room := range p.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// Returns a list of peers given roomIds, including sender.
//
// If no roomIds are given, returns all peers from all rooms the sender is in.
//
// If the sender is not in a room, returns only sender.
func (p *Peer) Room(roomIds ...string) Room {
	peers := Room{}

	if p == nil {
		fmt.Println("warning: peer is nil when trying to set room")
		return peers
	}

	peers[p.Id] = p

	if len(roomIds) == 0 {
		roomIds = make([]string, 0, len(p.rooms))
		for key := range p.rooms {
			roomIds = append(roomIds, key)
		}
	}

	for _, id := range roomIds {
		if _, ok := p.server.Rooms[id]; ok {
			for _, peer := range p.server.Rooms[id] {
				peers[peer.Id] = peer
			}
		}
	}
	return peers
}

// Returns a list of peers given roomIds, NOT including sender.
//
// If no roomIds are given, returns all peers from all rooms the sender is in.
//
// If the sender is not in a room, returns no peers.
func (p *Peer) Broadcast(roomIds ...string) Broadcast {
	peers := Broadcast{}

	if p == nil {
		fmt.Println("warning: peer is nil when trying to set broadcast")
		return peers
	}

	if len(roomIds) == 0 {
		roomIds = make([]string, 0, len(p.rooms))
		for key := range p.rooms {
			roomIds = append(roomIds, key)
		}
	}

	for _, id := range roomIds {
		if _, ok := p.server.Rooms[id]; ok {
			for _, peer := range p.server.Rooms[id] {
				if p.Id != peer.Id {
					peers[peer.Id] = peer
				}
			}
		}
	}
	return peers
}

func (p *Peer) Disconnect() {
	if p == nil {
		fmt.Println("warning: peer is nil when trying to disconnect")
		return
	}

	if p.server.peerConnections[p.Id] != nil {
		p.Emit("disconnected", "disconnected")
		p.server.Emit("disconnected", p)

		rooms := make([]string, 0, len(p.rooms))
		for room := range p.rooms {
			rooms = append(rooms, room)
		}

		p.Leave(rooms...)
		delete(p.server.peerConnections, p.Id)

		err := p.peerConnection.Close() //deletes all references to this peerconnection in mem and same for ICE agent (ICE agent releases the "closed" status)
		if err != nil {                 //https://www.w3.org/TR/webrtc/#dom-rtcpeerconnection-close
			fmt.Println(err)
		}
	}
}

func (p *Peer) OnRaw(f func([]byte)) {
	p.On("rawMessage", f)
}
