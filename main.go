package main

import (
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Server struct {
	address     string
	port        int
	protocol    string
	listener    net.Listener
	timeout     int
	maxconn     int
	messagech   chan Message
	exitch      chan bool
	peermap     map[uuid.UUID]string
	certificate string
	key         string
}

type Message struct {
	payload []byte
}

func NewServer(listenAddr string, port int, proto string, timeout int, maxcon int, crt string, key string) *Server {
	return &Server{
		address:     listenAddr,
		port:        port,
		protocol:    proto,
		timeout:     timeout,
		maxconn:     maxcon,
		messagech:   make(chan Message, 2),
		exitch:      make(chan bool),
		peermap:     make(map[uuid.UUID]string),
		certificate: crt,
		key:         key,
	}
}

func (s *Server) Start() error {
	crt, err := tls.LoadX509KeyPair(s.certificate, s.key)
	if err != nil {
		return err
	}
	config := &tls.Config{Certificates: []tls.Certificate{crt}}
	lsn, err := tls.Listen(s.protocol, s.address+":"+strconv.Itoa(s.port), config)
	if err != nil {
		return err
	}
	s.listener = lsn
	go s.Listen()
	<-s.exitch
	close(s.messagech)
	close(s.exitch)
	lsn.Close()
	return nil
}

func (s *Server) Listen() {
	for {
		con, err := s.listener.Accept()
		if err != nil {
			log.Println("accept error: ", err)
			return
		}
		defer con.Close()
		con.SetDeadline(time.Now().Add(time.Second * time.Duration(s.timeout)))
		cid := uuid.New()
		log.Printf("connection accepted from: %s uuid: %s", con.RemoteAddr().String(), cid)
		s.peermap[cid] = con.RemoteAddr().String()
		go s.Read(con)
	}
}

func (s *Server) Read(con net.Conn) {
	defer con.Close()
	buf := make([]byte, 1024)
	for {
		n, err := con.Read(buf)
		if err != nil {
			log.Println("read error: ", err)
			if errors.Is(err, io.EOF) {
				log.Println("read error [io.EOF]: ", err)
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				log.Println("read error [io timeout]: ", err)
				return
			}
			/*
				if err, ok := err.(net.Error); ok && err.Timeout() {
					log.Println("read error [io timeout #2]: ", err)
					return
				}
			*/
			continue
		}
		msg := strings.TrimSpace(string(buf[:n]))
		if msg == "exit" {
			s.exitch <- true
		}
		s.messagech <- Message{payload: []byte(msg)}
		con.SetDeadline(time.Now().Add(time.Second * time.Duration(s.timeout)))
	}
}

func main() {
	s := NewServer("127.0.0.1", 8080, "tcp4", 5, 2, "server.crt", "server.key")
	go func() {
		for msg := range s.messagech {
			log.Printf("received %s\n", string(msg.payload))
		}
	}()
	log.Fatal(s.Start())
}
