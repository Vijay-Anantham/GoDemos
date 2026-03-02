package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"syscall"
)

// initial setup for io multiplexing currently it takes in 1 client and handle connections.

func main() {
	addr := "0.0.0.0:8080"
	listener, err := NonBlockingListener(addr)
	if err != nil {
		log.Printf("Unexpected System Error %d", err)
		return
	}
	defer syscall.Close(listener)

	kq, err := syscall.Kqueue()
	if err != nil {
		log.Printf("Unexpected System Error %d", err)
		return
	}
	defer syscall.Close(kq)

	RegisterFD(kq, listener)
	fmt.Println("Server started on 0.0.0.0:8080")

	eventloop(kq, listener)
}

// Create a non blocking TCP Client using a net package in a unix based system it

func NonBlockingListener(addr string) (int, error) {
	// Set the listener config such as the control func for the file descriptor.
	lc := net.ListenConfig{
		Control: SetNonBlocking,
	}

	// func (lc *ListenConfig) Listen(ctx context.Context, network, address string) (Listener, error) Creates a Listener
	ln, err := lc.Listen(context.TODO(), "tcp", addr)
	if err != nil {
		return 0, err
	}

	// ensure checking the types
	tcpListener, ok := ln.(*net.TCPListener)
	if !ok {
		return 0, errors.New("The Lister is not a TCP Listener.")
	}
	// .File() actually creates a duplicate of the file descriptor and sets it back to blocking mode
	// by default (because Go assumes if you're asking for the File,
	// you want to handle it manually in a standard way).
	// This is why your code later calls syscall.SetNonblock again inside acceptConnection.
	fd, err := tcpListener.File()
	if err != nil {
		return 0, err
	}

	return int(fd.Fd()), nil
}

/*
This is a Callback to set the FD (File descriptor) to a non blocking read mode.
*/
func SetNonBlocking(network, address string, c syscall.RawConn) error {
	var err error
	err = c.Control(func(fd uintptr) { // protect the file descriptor such that it is not closed while we chaning the process.
		err = syscall.SetNonblock(int(fd), true) // When we read check the buffer and when its empty return error.
	})
	return err
}

// Now that we have a non blocking client we want to register it in our kqueue for optimal thread handling
// Mac and other BSP type OS used kqueue other linux uses poll and windows have to manage them with accept and poll
func RegisterFD(kq int, fd int) error {
	event := syscall.Kevent_t{
		Ident:  uint64(fd),
		Filter: syscall.EVFILT_READ, // Return signal for the read events
		Flags:  syscall.EV_ADD | syscall.EV_ENABLE,
	}

	_, err := syscall.Kevent(kq, []syscall.Kevent_t{event}, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func acceptConnections(listenerFd int) (int, error) {
	clientFD, _, err := syscall.Accept(listenerFd) // Returns clientfd, sockeradd, and err if any
	if err != nil {
		return 0, err
	}

	syscall.SetNonblock(clientFD, true)
	log.Printf("New client connected: FD %d\n", clientFD)

	return clientFD, nil
}

func handleClient(fd int) {
	buf := make([]byte, 1024)
	n, err := syscall.Read(fd, buf) // Gives in the container buf to the kernel and the kernel fetch the data from the network running in fd and return data into the buffer.
	if err != nil {
		log.Printf("Client FD %d is disconnected", fd)
		syscall.Close(fd)
		return
	}
	log.Printf("Received: %s\n", string(buf[:n]))

	message := fmt.Sprintf("System %s\n", string(buf[:n]))
	syscall.Write(fd, []byte(message)) // Fill in the buffer with the data and send it back to the netowrk to be send to the client.
}

func eventloop(kq int, listenerFD int) {
	events := make([]syscall.Kevent_t, 10)

	for {
		// Create a kevent with initial empty events array for it to fill in when a changes occur
		n, err := syscall.Kevent(kq, nil, events, nil)
		if err != nil {
			log.Printf("KEvent Error: %v", err)
			continue
		}

		for i := 0; i < n; i++ {
			fd := int(events[i].Ident)

			if fd == listenerFD {
				connFD, err := acceptConnections(listenerFD)
				if err != nil {
					log.Printf("Error accepting Connections: %v", err)
					continue
				}
				RegisterFD(kq, connFD)
			} else {
				handleClient(fd)
			}
		}
	}
}
