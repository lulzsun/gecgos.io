package gecgosio

import "github.com/pion/webrtc/v3"

type Peer struct {
	Id                   string
	dataChannel          *webrtc.DataChannel
	peerConnection       *webrtc.PeerConnection
	additionalCandidates []webrtc.ICECandidateInit
	IEventEmitter
}

func (p *Peer) AddCandidate(can webrtc.ICECandidateInit) []webrtc.ICECandidateInit {
	p.additionalCandidates = append(p.additionalCandidates, can)
	return p.additionalCandidates
}

func (p *Peer) Emit(e string, msg string) {
	p.dataChannel.SendText(`{"` + e + `":"` + msg + `"}`)
}
