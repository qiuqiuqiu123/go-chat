package server

import (
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/gobwas/ws"
	"github.com/sirupsen/logrus"
)

// Server is a websocket Server
type Server struct {
	once    sync.Once
	id      string
	address string
	sync.Mutex
	// 会话列表
	users map[string]net.Conn
}

// NewServer NewServer
func NewServer(id, address string) *Server {
	return newServer(id, address)
}

func newServer(id, address string) *Server {
	return &Server{
		id:      id,
		address: address,
		users:   make(map[string]net.Conn, 100),
	}
}

// Start server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	log := logrus.WithFields(logrus.Fields{
		"module": "Server",
		"listen": s.address,
		"id":     s.id,
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// step1. 升级
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			conn.Close()
			return
		}
		//step2. 读取userId
		userId := r.URL.Query().Get("USERID")
		if userId == "" {
			conn.Close()
			return
		}
		//step3. 添加到会话管理中
		old, ok := s.addUser(userId, conn)
		if ok {
			// 断开旧的连接
			old.Close()
		}
		log.Infof("user %s in", userId)

		go func(user string, conn net.Conn) {
			//step4. 读取消息
			err := s.readloop(user, conn)
			if err != nil {
				log.Error(err)
			}
			conn.Close()
			//step5. 连接断开，删除用户
			s.delUser(user)

			log.Infof("connection of %s closed", user)
		}(userId, conn)

	})
	log.Infoln("started")
	return http.ListenAndServe(s.address, mux)
}

func (s *Server) addUser(userId string, conn net.Conn) (net.Conn, bool) {
	s.Lock()
	defer s.Unlock()
	old, ok := s.users[userId] //返回旧的连接
	s.users[userId] = conn     //缓存
	return old, ok
}

func (s *Server) delUser(userId string) {
	s.Lock()
	defer s.Unlock()
	delete(s.users, userId)
}

// Shutdown Shutdown
func (s *Server) Shutdown() {
	s.once.Do(func() {
		s.Lock()
		defer s.Unlock()
		for _, conn := range s.users {
			conn.Close()
		}
	})
}

func (s *Server) readloop(userId string, conn net.Conn) error {
	for {
		frame, err := ws.ReadFrame(conn)
		if err != nil {
			return err
		}
		if frame.Header.OpCode == ws.OpClose {
			return errors.New("remote side close the conn")
		}

		if frame.Header.Masked {
			ws.Cipher(frame.Payload, frame.Header.Mask, 0)
		}
		// 接收文本帧内容
		if frame.Header.OpCode == ws.OpText {
			s.handle(userId, string(frame.Payload))
		}
	}
}

func (s *Server) handle(userId string, message string) {
	logrus.Infof("user %s recieve message : %s", userId, message)
	s.Lock()
	defer s.Unlock()
	for u, conn := range s.users {
		if u == userId {
			// 不发给自己
			continue
		}
		err := s.writeText(conn, message)
		if err != nil {
			logrus.Errorf("write to %s fail,error : %s", userId, err)
		}
	}
}

func (s *Server) writeText(conn net.Conn, message string) error {
	f := ws.NewTextFrame([]byte(message))
	return ws.WriteFrame(conn, f)
}
