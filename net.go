package main

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"net"
	"time"
)

// Reconnect automatically reconnects to the given host and provides a single bidirectional stream of packets.
//
// addr - the remote host.
// read - packets received from the remote host. closed when done.
// write - packets to send to the remote host. close this to exit.
// errors - net errors encountered. closed when done.
func Reconnect(addr string, read chan<- *res.Packet, write <-chan *res.Packet, errors chan<- error) {
	backOff := time.Second

	for {
		if func() bool {
			conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
			if err != nil {
				errors <- err
				time.Sleep(backOff)
				backOff *= 2
				return false
			} else {
				backOff = time.Second
			}
			defer conn.Close()

			readch, writech, errorsch := make(chan *res.Packet), make(chan *res.Packet), make(chan error, 2)
			defer close(writech)
			go Read(conn, readch, errorsch)
			go Write(conn, writech, errorsch)

			for {
				select {
				case p, ok := <-write:
					if !ok {
						return true
					}
					writech <- p

				case p, ok := <-readch:
					if !ok {
						readch = nil
						continue
					}
					read <- p

				case err := <-errorsch:
					errors <- err
					return false
				}
			}
		}() {
			close(read)
			close(errors)
			return
		}
	}
}

func Disconnect(read <-chan *res.Packet, write chan<- *res.Packet, errors <-chan error) {
	close(write)
	for {
		if read == nil && errors == nil {
			return
		}

		select {
		case _, ok := <-read:
			if !ok {
				read = nil
			}

		case _, ok := <-errors:
			if !ok {
				errors = nil
			}
		}
	}
}

func makeSlice(l uint64) (b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()
	return make([]byte, l), nil
}

func Read(conn net.Conn, packets chan<- *res.Packet, errors chan<- error) {
	defer close(packets)

	var l [64 / 8]byte
	for {
		_, err := io.ReadFull(conn, l[:])
		if err != nil {
			errors <- err
			return
		}

		b, err := makeSlice(binary.LittleEndian.Uint64(l[:]))
		if err != nil {
			errors <- err
			return
		}
		_, err = io.ReadFull(conn, b)
		if err != nil {
			errors <- err
			return
		}

		p := new(res.Packet)
		err = proto.Unmarshal(b, p)
		if err != nil {
			errors <- err
			return
		}

		packets <- p
	}
}

func Write(conn net.Conn, packets <-chan *res.Packet, errors chan<- error) {
	defer func() {
		for _ = range packets {
			// discard
		}
	}()

	var l [64 / 8]byte
	for p := range packets {
		b, err := proto.Marshal(p)
		if err != nil {
			errors <- err
			return
		}

		binary.LittleEndian.PutUint64(l[:], uint64(len(b)))

		n, err := conn.Write(l[:])
		if err == nil && n != len(l) {
			err = io.ErrShortWrite
		}
		if err != nil {
			errors <- err
			return
		}

		n, err = conn.Write(b)
		if err == nil && n != len(b) {
			err = io.ErrShortWrite
		}
		if err != nil {
			errors <- err
			return
		}
	}
}

func Send(ch chan<- *res.Packet, p *res.Packet) {
	ch <- p
}
