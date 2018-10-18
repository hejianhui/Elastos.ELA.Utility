package restful

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// response represent the default RESTful response data structure.
type response struct {
	Result interface{} `json:"Result,omitempty"`
	Error  int         `json:"Error,omitempty"`
	Desc   string      `json:"Desc,omitempty"`
}

// Config is the configuration of the RESTful server.
type Config struct {
	Path      string
	ServePort uint16
	NetListen func(port uint16) (net.Listener, error)
	Response  func(result interface{}, err error) []byte
}

type Server struct {
	cfg    Config
	server *http.Server
	routes map[string]*Route
}

func (s *Server) write(w http.ResponseWriter, data []byte) {
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Add("content-type", "application/json;charset=utf-8")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

func (s *Server) response(w http.ResponseWriter, result interface{}, err error) {
	resp := response{
		Result: result,
		Error:  http.StatusOK,
	}

	if err != nil {
		resp.Error = http.StatusInternalServerError
		resp.Desc = err.Error()
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Fatal("HTTP Handle - json.Marshal: %v", err)
		return
	}
	s.write(w, data)
}

// RegisterAction register a RESTful handler with the given url and method. For
// a url with parameters, add /:param at the end of url like /api/block/:hash.
// multiple parameters are supported, like /api/account/:assetid/:address. The
// registered handler type must match the method, for example, GET method must use
// func(params util.Params) (interface{}, error) as handler.
func (s *Server) RegisterAction(method, url string, handler interface{}) error {
	// check url format.
	if !strings.HasPrefix(url, "/") {
		return fmt.Errorf(`resource url must begin with "/"`)
	}
	if strings.HasSuffix(url, "/") {
		return fmt.Errorf(`resource url must not end with "/"`)
	}

	// check if handler matches method require.
	if !matchHandler(method, handler) {
		return fmt.Errorf("handler %T not match %s method", handler, method)
	}

	// parse url to get route regex and parameters.
	regex, params := ParseUrl(url)

	// register route according to method.
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete:
	//	just do nothing.
	case http.MethodPost:
		if strings.Contains(url, ":") {
			return fmt.Errorf("post method do not allow path parameters")
		}

	default:
		return fmt.Errorf("method %s is not a valid restful method type", method)
	}

	path := regex.String()
	route := s.routes[path]
	if route == nil {
		route = NewRoute(regex)
	}

	route.SetHandler(method, handler)

	if len(params) > 0 {
		route.SetParams(method, params...)
	}

	s.routes[path] = route

	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, r := range s.routes {
		if r.matches(req) {
			result, err := r.handle(req)
			if s.cfg.Response != nil {
				data := s.cfg.Response(result, err)
				s.write(w, data)

			} else {
				s.response(w, result, err)

			}

			// break when handler finished.
			break
		}
	}

	// No route match
	http.NotFound(w, req)
}

func (s *Server) Start() error {
	if s.cfg.ServePort == 0 {
		return fmt.Errorf("restful ServePort not configured")
	}

	var err error
	var listener net.Listener
	if s.cfg.NetListen != nil {
		listener, err = s.cfg.NetListen(s.cfg.ServePort)
	} else {
		listener, err = net.Listen("tcp", fmt.Sprint(":", s.cfg.ServePort))
	}
	if err != nil {
		return err
	}

	if s.cfg.Path == "" {
		s.server = &http.Server{Handler: s}
	} else {
		http.Handle(s.cfg.Path, s)
		s.server = &http.Server{}
	}
	return s.server.Serve(listener)
}

func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}

	return fmt.Errorf("server not started")
}

// NewServer creates and return a RESTful server instance.
func NewServer(cfg *Config) *Server {
	return &Server{
		cfg:    *cfg,
		routes: make(map[string]*Route),
	}
}