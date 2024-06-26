package gecgosio

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/pion/webrtc/v4"
)

var api *webrtc.API

type Server struct {
	Options
	peerConnections map[string]*Peer
	Rooms           map[string]Room
	IEventEmitter
}

type Options struct {
	Ordered bool
	// Only effective when using gecgos.io provided HTTP server
	Cors
	// Disables gecgos.io provided HTTP server
	DisableHttpServer bool
	// If defined, bind only to the given local address if it exists
	BindAddress string
	// Sets a list of external IP addresses of 1:1 (D)NAT and a candidate type for which the external IP address is used. 
	// This is useful when you host a server using Pion on an AWS EC2 instance which has a private address, behind a 1:1 DNAT with a public IP (e.g. Elastic IP).
	NAT1To1IPs []string
}

type Cors struct {
	Origin             string
	AllowAuthorization bool
}

// Instantiate and return a new Gecgos server
func Gecgos(opt *Options) *Server {
	s := &Server{
		Rooms:         make(map[string]Room),
		IEventEmitter: CreateEventEmitter(),
	}

	if opt != nil {
		s.Options = *opt
	}

	if i, err := net.ResolveIPAddr("ip4", s.BindAddress); err == nil {
		fmt.Println("Resolved IP address from BindAddress:", i.IP)
	} else {
		s.BindAddress = "0.0.0.0"
	}

	return s
}

// Make the server listen on a specific port
//
// If DisableHttpHandler is true, Listen() will no longer be blocking
func (s *Server) Listen(port int) error {
	// Listen on defined bind address and port, will be used for all WebRTC traffic
	udpListener, err := net.ListenPacket("udp4", fmt.Sprintf("%s:%d", s.BindAddress, port))
	if err != nil {
		panic(err)
	}

	s.peerConnections = make(map[string]*Peer)

	// Create a SettingEngine, this allows non-standard WebRTC behavior
	settingEngine := webrtc.SettingEngine{}

	//Our Public Candidate is declared here cause were not using a STUN server for discovery
	//and just hardcoding the open port, and port forwarding webrtc traffic on the router
	settingEngine.SetNAT1To1IPs(s.NAT1To1IPs, webrtc.ICECandidateTypeHost)

	// Configure our SettingEngine to use our UDPMux. By default a PeerConnection has
	// no global state. The API+SettingEngine allows the user to share state between them.
	// In this case we are sharing our listening port across many.
	settingEngine.SetICEUDPMux(webrtc.NewICEUDPMux(nil, udpListener))

	// Create a new API using our SettingEngine
	api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	if s.Options.DisableHttpServer {
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

func (s *Server) OnConnection(f func(c *Peer)) {
	s.On("connection", f)
}

func (s *Server) OnDisconnect(f func(c *Peer)) {
	s.On("disconnected", f)
}
