package gecgosio

import (
	"fmt"
	"strings"

	"github.com/pion/webrtc/v3"
	"github.com/rs/xid"
)

// The server's definition of a client
type Peer struct {
	Id                   string
	rooms                map[string]bool
	emitReliable         bool
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
		rooms:                make(map[string]bool),
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
	p.emitReliable = true
	p.interval = interval
	p.runs = runs
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
func (p *Peer) Emit(e string, msg ...string) {
	data := strings.Join(msg, ", ")

	startsWithCurly := strings.HasPrefix(data, "{")
	endsWithCurly := strings.HasSuffix(data, "}")
	jsonObjStr := startsWithCurly && endsWithCurly

	if !jsonObjStr {
		data = "\"" + data + "\""
	}
	if p.emitReliable {
		p.emitReliable = false
	}
	p.dataChannel.SendText(`{"` + e + `":` + data + `}`)
}

func (p *Peer) Join(ids ...string) {
	for _, id := range ids {
		if _, ok := p.server.rooms[id]; !ok {
			p.server.rooms[id] = make(Room)
		}
		p.server.rooms[id][p.Id] = p
		p.rooms[id] = true
	}
}

func (p *Peer) Leave(ids ...string) {
	for _, id := range ids {
		if _, ok := p.server.rooms[id]; ok {
			if _, ok := p.server.rooms[id][p.Id]; ok {
				delete(p.server.rooms[id], p.Id)
			}
			if _, ok := p.rooms[id]; ok {
				delete(p.rooms, id)
			}
		}
	}
}

// Returns a list of peers given roomIds, including sender
//
// If no roomIds are given, returns all peers from all rooms
// If the sender is not in a room, returns only sender
func (p *Peer) Room(roomIds ...string) Room {
	peers := Room{}
	peers[p.Id] = p

	if len(roomIds) == 0 {
		roomIds = make([]string, 0, len(p.rooms))
        for key := range p.rooms {
            roomIds = append(roomIds, key)
        }
	}

	for _, id := range roomIds {
		if _, ok := p.server.rooms[id]; ok {
			for _, p := range p.server.rooms[id] {
				peers[p.Id] = p
			}
		}
	}
	return peers
}

func (p *Peer) Disconnect() {
	if p.server.peerConnections[p.Id] != nil {
		p.Emit("disconnected", "disconnected")
		p.server.Emit("disconnected", *p)

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
