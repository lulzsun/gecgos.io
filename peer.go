package gecgosio

import (
	"fmt"

	"github.com/pion/webrtc/v3"
	"github.com/rs/xid"
)

// The server's definition of a client
type Peer struct {
	Id                   string
	rooms                map[string]bool
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

func (p *Peer) Emit(e string, msg string) {
	p.dataChannel.SendText(`{"` + e + `":"` + msg + `"}`)
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

func (p *Peer) To(roomIds ...string) Room {
	peers := Room{}
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
		p.server.Emit("disconnection", *p)
		delete(p.server.peerConnections, p.Id)

		err := p.peerConnection.Close() //deletes all references to this peerconnection in mem and same for ICE agent (ICE agent releases the "closed" status)
		if err != nil {                 //https://www.w3.org/TR/webrtc/#dom-rtcpeerconnection-close
			fmt.Println(err)
		}
	}
}
