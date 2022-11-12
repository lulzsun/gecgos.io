package gecgosio

import (
	"fmt"

	"github.com/pion/webrtc/v3"
	"github.com/rs/xid"
)

// The server's definition of a client
type Peer struct {
	Id                   string
	server               *Server
	dataChannel          *webrtc.DataChannel
	peerConnection       *webrtc.PeerConnection
	additionalCandidates []webrtc.ICECandidateInit
	IEventEmitter
}

func createPeer(s *Server, dc *webrtc.DataChannel, pc *webrtc.PeerConnection) *Peer {
	p := &Peer{
		Id:                   xid.New().String(),
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
