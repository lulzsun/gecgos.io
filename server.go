package gecgosio

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/pion/webrtc/v3"
)

var api *webrtc.API

type Server struct {
	Options
	peerConnections map[string]*Peer
	rooms           map[string]Room
	IEventEmitter
}

type Options struct {
	Ordered bool
	Cors
	CustomHttpHandler bool
}

type Cors struct {
	Origin             string
	AllowAuthorization bool
}

// Instantiate and return a new Gecgos server
func Gecgos(opt *Options) *Server {
	s := &Server{
		rooms:         make(map[string]Room),
		IEventEmitter: CreateEventEmitter(),
	}

	if opt != nil {
		s.Options = *opt
	}

	return s
}

// Make the server listen on a specific port
//
// If CustomHttpHandler is true, Listen() will not be blocking
func (s *Server) Listen(port int) error {
	// Listen on UDP Port 80, will be used for all WebRTC traffic
	udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IP{0, 0, 0, 0},
		Port: port,
	})
	if err != nil {
		panic(err)
	}

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

	if s.Options.CustomHttpHandler {
		return nil
	}

	corsHandler := func(h http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if s.Cors.Origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", s.Cors.Origin)
			}

			w.Header().Set("Access-Control-Request-Method", "*")
			w.Header().Set("Access-Control-Request-Headers", "X-Requested-With, Accept, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")

			if s.Cors.AllowAuthorization {
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}

			// handle preflight cors request
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			h.ServeHTTP(w, r)
		}
	}

	http.HandleFunc("/.wrtc/v2/connections", corsHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.CreateConnection(w, r)
	})))

	// mimic geckos.io http routes
	// https://github.com/geckosio/geckos.io/blob/6ad2535a8f26d6cce0e7af2c4cf7df311622b567/packages/server/src/httpServer/routes.ts
	http.HandleFunc("/.wrtc/v2/connections/", corsHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Path
		if page == "/.wrtc/v2/connections/" {
			s.CreateConnection(w, r)
		} else if strings.Split(page, "/")[5] == "remote-description" {
			s.SetRemoteDescription(w, r)
		} else if strings.Split(page, "/")[5] == "additional-candidates" {
			s.SendAdditionalCandidates(w, r)
		} else {
			fmt.Println(page)
			w.WriteHeader(http.StatusNotFound)
		}
	})))

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
