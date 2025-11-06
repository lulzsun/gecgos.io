package gecgosio

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/pion/webrtc/v4"
)

// This function will try to prepare a WebRTC connection by first offering the SDP challenge to the potential client
// https://github.com/geckosio/geckos.io/blob/1d15c1ae8877b62f53fa026de2323c09202b07ab/packages/server/src/wrtc/connectionsManager.ts#L50
func (s *Server) CreateConnection(w http.ResponseWriter, r *http.Request) {
	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
			{URLs: []string{"stun:stun2.l.google.com:19302"}},
			{URLs: []string{"stun:stun3.l.google.com:19302"}},
			{URLs: []string{"stun:stun4.l.google.com:19302"}},
		},
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlanWithFallback,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	//Setup dataChannel to act like UDP with ordered messages (no retransmits)
	//with the DataChannelInit struct
	var udpPls webrtc.DataChannelInit
	var retransmits uint16 = 0

	//DataChannel will drop any messages older than
	//the most recent one received if ordered = true && retransmits = 0
	//This is nice so we can always assume client
	//side that the message received from the server
	//is the most recent update, and not have to
	//implement logic for handling old messages
	udpPls.Ordered = &s.Ordered
	udpPls.MaxRetransmits = &retransmits

	// Create a datachannel with label 'UDP' and options udpPls
	dataChannel, err := peerConnection.CreateDataChannel("geckos.io", &udpPls)
	if err != nil {
		panic(err)
	}

	peer := createPeer(s, dataChannel, peerConnection)

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		if peer == nil {
			return
		}

		if connectionState == 3 {
			s.Emit("connection", peer)
		} else if connectionState == 5 || connectionState == 6 || connectionState == 7 {
			peer.Disconnect()
		}
	})

	// When Pion gathers a new ICE Candidate send it to the client. This is how
	// ice trickle is implemented. Everytime we have a new candidate available we send
	// it as soon as it is ready. We don't wait to emit a Offer/Answer until they are
	// all available
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil || peer == nil {
			return
		}
		peer.AddCandidate(c.ToJSON())
	})

	// Register message/event handling
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		if peer == nil {
			return
		}

		var m map[string]interface{}
		// if err, these are probably just raw bytes
		if err := json.Unmarshal(msg.Data, &m); err != nil {
			peer.IEventEmitter.Emit("rawMessage", msg.Data)
			return
		}

		for event, data := range m {
			if dataJson, ok := data.(map[string]interface{}); ok {
				if _, ok := dataJson["RELIABLE"].(float64); ok {
					msg, _ := json.Marshal(dataJson["MESSAGE"])
					if dataJson["RELIABLE"] == 1 {
						// do reliable stuff (keep a buffer)
						//https://github.com/geckosio/geckos.io/blob/7cb3911c98e463e428ac42b1efcc5c1f552c94cf/packages/server/src/geckos/channel.ts
					}
					peer.IEventEmitter.Emit(event, string(msg))
					continue
				}
			}
			msg, _ := json.Marshal(data)
			peer.IEventEmitter.Emit(event, string(msg))
		}
	})

	// Create an offer to send to the browser
	offer, err := peer.peerConnection.CreateOffer(nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json := []byte(`{
		"userData": {},
		"id": "` + peer.Id + `",
		"localDescription": {
			"type": "offer",
			"sdp": ` + strconv.Quote(offer.SDP) + `
		}
	}`)
	w.Write(json)
}

// This function will try to accept the SDP challenge from a potential client
// https://github.com/geckosio/geckos.io/blob/6ad2535a8f26d6cce0e7af2c4cf7df311622b567/packages/server/src/httpServer/routes.ts#L38
func (s *Server) SetRemoteDescription(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var data map[string]interface{}
	err = json.Unmarshal([]byte(body), &data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	//fmt.Println(data["sdp"])
	//fmt.Println(data["type"])
	id := strings.Split(r.URL.Path, "/")[4]

	if data["type"] != "answer" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: data["sdp"].(string)}

	// Set the remote SessionDescription
	err = s.peerConnections[id].peerConnection.SetRemoteDescription(answer)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

// This function will send the client additional ice candidates to aid in connection
// https://github.com/geckosio/geckos.io/blob/6ad2535a8f26d6cce0e7af2c4cf7df311622b567/packages/server/src/httpServer/routes.ts#L68
func (s *Server) SendAdditionalCandidates(w http.ResponseWriter, r *http.Request) {
	id := strings.Split(r.URL.Path, "/")[4]
	match, _ := regexp.MatchString("[0-9a-zA-Z]{20}", id)

	if match == true && s.peerConnections[id] != nil {
		json, err := json.Marshal(s.peerConnections[id].additionalCandidates)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(json)
		return
	} else {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}
