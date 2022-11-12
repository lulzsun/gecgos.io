package gecgosio

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/pion/webrtc/v3"
)

var api *webrtc.API

type Server struct {
	Options
	peerConnections map[string]*Peer
	IEventEmitter
}

type Options struct {
	Ordered bool
	Cors
}

type Cors struct {
	Origin string
	//AllowAuthorization bool
}

// Instantiate and return a new Gecgos server
func Gecgos(opt *Options) *Server {
	s := &Server{
		IEventEmitter: CreateEventEmitter(),
	}

	if opt != nil {
		s.Options = *opt
	}

	return s
}

// Make the server listen on a specific port
func (s *Server) Listen(port int) error {
	// Listen on UDP Port 80, will be used for all WebRTC traffic
	udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IP{0, 0, 0, 0},
		Port: port,
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Gecgos.io signaling server is running on port at %d\n", port)
	s.peerConnections = make(map[string]*Peer)

	// Create a SettingEngine, this allows non-standard WebRTC behavior
	settingEngine := webrtc.SettingEngine{}

	//Our Public Candidate is declared here cause were not using a STUN server for discovery
	//and just hardcoding the open port, and port forwarding webrtc traffic on the router
	settingEngine.SetNAT1To1IPs([]string{}, webrtc.ICECandidateTypeHost)

	// Configure our SettingEngine to use our UDPMux. By default a PeerConnection has
	// no global state. The API+SettingEngine allows the user to share state between them.
	// In this case we are sharing our listening port across many.
	settingEngine.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))

	// Create a new API using our SettingEngine
	api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	// mimic geckos.io http routes
	// https://github.com/geckosio/geckos.io/blob/6ad2535a8f26d6cce0e7af2c4cf7df311622b567/packages/server/src/httpServer/routes.ts
	http.HandleFunc("/.wrtc/v2/connections/", func(w http.ResponseWriter, r *http.Request) {
		if s.Cors.Origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.Cors.Origin)
		}

		page := r.URL.Path
		if page == "/.wrtc/v2/connections/" {
			s.createConnection(w, r)
		} else if strings.Split(page, "/")[5] == "remote-description" {
			s.setRemoteDescription(w, r)
		} else if strings.Split(page, "/")[5] == "additional-candidates" {
			s.sendAdditionalCandidates(w, r)
		} else {
			fmt.Println(page)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	err = http.ListenAndServe("localhost:"+strconv.Itoa(port), nil) //Http server blocks
	if err != nil {
		panic(err)
	}
	return err
}

func (s *Server) OnConnection(f func(c Peer)) {
	s.On("connection", f)
}

func (s *Server) OnDisconnect(f func(c Peer)) {
	s.On("disconnection", f)
}

// This function will try to prepare a WebRTC connection by first offering the SDP challenge to the potential client
// https://github.com/geckosio/geckos.io/blob/1d15c1ae8877b62f53fa026de2323c09202b07ab/packages/server/src/wrtc/connectionsManager.ts#L50
func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Client attempting to connect from: ", r.RemoteAddr)

	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})
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
		if connectionState == 3 {
			s.Emit("connection", *peer)
		} else if connectionState == 5 || connectionState == 6 || connectionState == 7 {
			peer.Disconnect()
		}
	})

	// When Pion gathers a new ICE Candidate send it to the client. This is how
	// ice trickle is implemented. Everytime we have a new candidate available we send
	// it as soon as it is ready. We don't wait to emit a Offer/Answer until they are
	// all available
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		peer.AddCandidate(c.ToJSON())
	})

	// Register message/event handling
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		m := map[string]string{}
		err := json.Unmarshal(msg.Data, &m)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		for event, data := range m {
			peer.IEventEmitter.Emit(event, data)
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
func (s *Server) setRemoteDescription(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) sendAdditionalCandidates(w http.ResponseWriter, r *http.Request) {
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
