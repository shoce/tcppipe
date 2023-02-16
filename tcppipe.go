/*
history:
2016-0203 v1

GoFmt GoBuildNull GoBuild GoRelease
*/

package main

import (
	"expvar"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func fatalUsage() {
	log.Fatal(`
	Creates tcp pipe for an ip address after a valid otp sent to the socket.
	Usage: tcppipe accept/dial addr1 accept/dial addr2
	Example: tcppipe dial 127.1:9022 dial 127.1:22
	Example: tcppipe accept 127.1:8022 accept 127.1:9022
	Env vars:
		Timeout [30s]- timeout for tcp connections and between dials
	`)
}

func allowAccept(addr string) (allow chan bool, connch chan *net.Conn, err error) {
	l, err := net.Listen("tcp4", addr)
	if err != nil {
		return
	}
	allow = make(chan bool)
	connch = make(chan *net.Conn)
	go func(allow chan bool, l net.Listener, connch chan *net.Conn) {
		for {
			<-allow
			l.(*net.TCPListener).SetDeadline(time.Now().Add(Timeout))
			conn, err := l.Accept()
			if err == nil {
				connch <- &conn
			} else {
				connch <- nil
			}
		}
	}(allow, l, connch)
	return
}

func allowDial(addr string) (allow chan bool, connch chan *net.Conn, err error) {
	allow = make(chan bool)
	connch = make(chan *net.Conn)
	go func(allow chan bool, addr string, connch chan *net.Conn) {
		for {
			<-allow
			conn, err := net.Dial("tcp4", addr)
			if err == nil {
				connch <- &conn
			} else {
				connch <- nil
			}
			time.Sleep(Timeout)
		}
	}(allow, addr, connch)
	return
}

func allowConn(cmd string, addr string) (allow chan bool, connch chan *net.Conn, err error) {
	switch cmd {
	case "accept":
		allow, connch, err = allowAccept(addr)
	case "dial":
		allow, connch, err = allowDial(addr)
	default:
		err = fmt.Errorf("Cannon parse command `%s`: should be accept/dial", cmd)
	}
	return
}

var (
	Timeout time.Duration

	expAllow1 *expvar.Int
	expAllow2 *expvar.Int
	expOpen1  *expvar.Int
	expOpen2  *expvar.Int
	expClose1 *expvar.Int
	expClose2 *expvar.Int
	expAddr1  *expvar.Map
	expAddr2  *expvar.Map
)

func main() {
	var err error

	if len(os.Args) != 5 {
		fatalUsage()
	}

	TimeoutString := os.Getenv("Timeout")
	if TimeoutString == "" {
		TimeoutString = "30s"
	}
	Timeout, err = time.ParseDuration(TimeoutString)
	if err != nil {
		log.Fatal(err)
	}

	cmd1, addr1 := os.Args[1], os.Args[2]
	al1, ch1, err := allowConn(cmd1, addr1)
	if err != nil {
		log.Fatal(err)
	}

	cmd2, addr2 := os.Args[3], os.Args[4]
	al2, ch2, err := allowConn(cmd2, addr2)
	if err != nil {
		log.Fatal(err)
	}

	expAllow1 = expvar.NewInt("Allow1")
	expAllow2 = expvar.NewInt("Allow2")
	expOpen1 = expvar.NewInt("Open1")
	expOpen2 = expvar.NewInt("Open2")
	expClose1 = expvar.NewInt("Close1")
	expClose2 = expvar.NewInt("Close2")
	expAddr1 = expvar.NewMap("Accept1")
	expAddr2 = expvar.NewMap("Accept2")

	for {
		al1 <- true
		expAllow1.Add(1)
		conn1 := <-ch1
		if conn1 == nil {
			continue
		}
		log.Printf("remote:%s local:%s ->", (*conn1).RemoteAddr(), (*conn1).LocalAddr())
		expOpen1.Add(1)
		expAddr1.Add((*conn1).RemoteAddr().String(), 1)

		go func(conn1 *net.Conn) {
			defer func() {
				(*conn1).Close()
				expClose1.Add(1)
			}()

			al2 <- true
			expAllow2.Add(1)
			conn2 := <-ch2
			if conn2 == nil {
				return
			}
			log.Printf("remote:%s local:%s -> local:%s remote:%s", (*conn1).RemoteAddr(), (*conn1).LocalAddr(), (*conn2).LocalAddr(), (*conn2).RemoteAddr())
			expOpen2.Add(1)
			expAddr2.Add((*conn2).RemoteAddr().String(), 1)
			defer func() {
				(*conn2).Close()
				expClose2.Add(1)
			}()

			tconn1 := timeoutConn{*conn1}
			tconn2 := timeoutConn{*conn2}
			go io.Copy(*conn2, tconn1)
			io.Copy(*conn1, tconn2)
		}(conn1)
	}
}

type timeoutConn struct {
	Conn net.Conn
}

func (c timeoutConn) Read(buf []byte) (int, error) {
	c.Conn.SetReadDeadline(time.Now().Add(Timeout))
	return c.Conn.Read(buf)
}

func (c timeoutConn) Write(buf []byte) (int, error) {
	c.Conn.SetWriteDeadline(time.Now().Add(Timeout))
	return c.Conn.Write(buf)
}
