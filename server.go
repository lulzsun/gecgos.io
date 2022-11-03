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
	"time"

	//"reflect"
	//"encoding/binary"

	"github.com/pion/webrtc/v3"
	"github.com/rs/xid"
)

var api *webrtc.API
var connections = make(map[string]*Client)

type Client struct {
	peerConnection       *webrtc.PeerConnection
	additionalCandidates []webrtc.ICECandidateInit
}

func (c *Client) AddCandidate(can webrtc.ICECandidateInit) []webrtc.ICECandidateInit {
	c.additionalCandidates = append(c.additionalCandidates, can)
	return c.additionalCandidates
}

type Server struct {
	Ordered bool
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
		page := r.URL.Path
		if page == "/.wrtc/v2/connections/" {
			createConnection(w, r)
		} else if strings.Split(page, "/")[5] == "remote-description" {
			setRemoteDescription(w, r)
		} else if strings.Split(page, "/")[5] == "additional-candidates" {
			sendAdditionalCandidates(w, r)
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

// This function will try to prepare a WebRTC connection by first offering the SDP challenge to the potential client
// https://github.com/geckosio/geckos.io/blob/1d15c1ae8877b62f53fa026de2323c09202b07ab/packages/server/src/wrtc/connectionsManager.ts#L50
func createConnection(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Client attempting to connect from: ", r.RemoteAddr)
	id := xid.New().String()

	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	connections[id] = &Client{peerConnection, nil}

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
	var ordered = true

	udpPls.Ordered = &ordered
	udpPls.MaxRetransmits = &retransmits

	// Create a datachannel with label 'UDP' and options udpPls
	dataChannel, err := peerConnection.CreateDataChannel("UDP", &udpPls)
	if err != nil {
		panic(err)
	}

	//Create a reliable datachannel with label "TCP" for all other communications
	reliableChannel, err := peerConnection.CreateDataChannel("TCP", nil)
	if err != nil {
		panic(err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())

		//3 = ICEConnectionStateConnected
		if connectionState == 3 {
			fmt.Printf("connected!")
		} else if connectionState == 5 || connectionState == 6 || connectionState == 7 {
			fmt.Println("disconnected!")

			err := peerConnection.Close() //deletes all references to this peerconnection in mem and same for ICE agent (ICE agent releases the "closed" status)
			if err != nil {               //https://www.w3.org/TR/webrtc/#dom-rtcpeerconnection-close
				fmt.Println(err)
			}
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

		// outbound, marshalErr := json.Marshal(c.ToJSON())
		// if marshalErr != nil {
		// 	panic(marshalErr)
		// }

		fmt.Println("New ICE Candidate avaliable for " + id + ": " + c.ToJSON().Candidate)
		connections[id].AddCandidate(c.ToJSON())
	})

	// Register ordered channel opening handling
	dataChannel.OnOpen(func() {
		for {
			time.Sleep(time.Millisecond * 50) //50 milliseconds = 20 updates per second
			//20 milliseconds = ~60 updates per second

			//fmt.Println(UpdatesString)
			// Send the message as text so we can JSON.parse in javascript
			sendErr := dataChannel.SendText("hello")
			if sendErr != nil {
				fmt.Println("data send err", sendErr)
				break
			}
		}

	})

	// Register text message handling
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		fmt.Printf("Message from DataChannel '%s': '%s'\n", dataChannel.Label(), string(msg.Data))
	})

	// Register reliable channel opening handling
	reliableChannel.OnOpen(func() {
		sendErr := reliableChannel.SendText("hi")
		if sendErr != nil {
			panic(err)
		}
	})

	// Register message handling (Data all served as a bytes slice []byte)
	// for user controls
	reliableChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		fmt.Printf("Message from DataChannel '%s': '%s'\n", reliableChannel.Label(), string(msg.Data))
	})

	// Create an offer to send to the browser
	offer, err := connections[id].peerConnection.CreateOffer(nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	//fmt.Println(*peerConnection.LocalDescription())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json := []byte(`{
		"id": "` + id + `",
		"userData": "{}",
		"localDescription": {
			"type": "offer",
			"sdp": ` + strconv.Quote(offer.SDP) + `
		}
	}`)
	w.Write(json)
}

// This function will try to accept the SDP challenge from a potential client
// https://github.com/geckosio/geckos.io/blob/6ad2535a8f26d6cce0e7af2c4cf7df311622b567/packages/server/src/httpServer/routes.ts#L38
func setRemoteDescription(w http.ResponseWriter, r *http.Request) {
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
	err = connections[id].peerConnection.SetRemoteDescription(answer)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

// This function will send the client additional ice candidates to aid in connection
// https://github.com/geckosio/geckos.io/blob/6ad2535a8f26d6cce0e7af2c4cf7df311622b567/packages/server/src/httpServer/routes.ts#L68
func sendAdditionalCandidates(w http.ResponseWriter, r *http.Request) {
	id := strings.Split(r.URL.Path, "/")[4]
	match, _ := regexp.MatchString("[0-9a-zA-Z]{20}", id)

	if match == true {
		json, err := json.Marshal(connections[id].additionalCandidates)
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
