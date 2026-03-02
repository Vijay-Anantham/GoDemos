# IO Multiplexing

[Inspiration](https://medium.com/@smafjal/io-multiplexing-in-go-14917eb4258f)

This is a comprehensive guide to building a high-performance network engine using Go and `kqueue`. I have structured this as a technical deep-dive suitable for a GitHub README or a "Guide to I/O Multiplexing."

---

# Understanding I/O Multiplexing: From Raw Sockets to High-Performance Engines

This document explores the architecture of high-performance networking by breaking down a Go implementation of a `kqueue` event loop.

## 1. The Core Intuition: The "Waiter" vs. The "Pager"

* **Blocking I/O (Traditional):** One waiter per table. The waiter stands idle until the customer orders. This scales poorly (1,000 tables = 1,000 waiters).
* **I/O Multiplexing (kqueue/epoll):** One waiter for the whole room. Every table has a "Pager." The waiter sits in a chair (the **Event Loop**) and naps until a pager vibrates. The waiter wakes up, handles the specific table, and returns to the chair.

---

## 2. Component Deep Dive

### A. The Listener & Non-Blocking Mode

To handle thousands of connections on one thread, we must tell the OS not to "freeze" our program when no data is present.

* **`setNonBlocking`**: This function uses `syscall.RawConn` to trigger a system call (`fcntl`) that sets the `O_NONBLOCK` flag.
* **`createNonBlockingListener`**: Uses a `net.ListenConfig` hook. It ensures that the "entrance door" (the listening socket) is non-blocking from the moment it is created.

### B. `syscall.Kevent`: The Notification Center

`Kevent` is the heart of the engine. It performs two roles:

1. **Registration:** You "subscribe" to interests (e.g., "Tell me when FD 5 is ready to read").
2. **Waiting:** The program enters a sleep state. The Kernel wakes it up only when a hardware interrupt (network packet) occurs. This keeps CPU usage at 0% during idle times.

### C. The "Receptionist" Logic

When `kqueue` identifies an event on the `listenerFD`, it signifies a new connection.

1. **Accept:** The OS picks the client from the "waiting room" (TCP backlog) and assigns a new unique FD.
2. **Register:** We immediately register this new FD with `kqueue` so we can track its future messages.

---

## 3. Advanced Implementation: The Write Buffer

In a production environment, you cannot assume `syscall.Write` will send all data at once. If the client's internet is slow, the kernel's outgoing buffer will fill up.

### The "Bucket" Pattern

1. **Store:** Instead of writing directly, append data to a per-client `outBuf` (the "bucket").
2. **Watch:** Register for `EVFILT_WRITE` events.
3. **Pour:** When the Kernel says space is available, write as much as possible.
4. **Unregister:** Crucially, once the bucket is empty, you must **unregister** the write interest to avoid a "Busy Loop" (where the CPU spins at 100% because the empty buffer is *always* ready for writing).

---

## 4. Resource Management & Cleanup

To avoid **Memory Leaks** and **File Descriptor Leaks**, every connection must be purged.

* **Polite Exit:** If `syscall.Read` returns `0`, the client has disconnected.
* **Ghosting:** Implement a "Janitor" goroutine that checks a `lastActive` timestamp on each client and closes sockets that have been silent for too long.

---

## 5. Architectural Comparison

| Feature | This Program | **Redis** | **Nginx** |
| --- | --- | --- | --- |
| **I/O Model** | `kqueue` (Single Loop) | `epoll`/`kqueue` | `epoll`/`kqueue` |
| **Concurrency** | Single-threaded | Single-threaded | Multi-process (Workers) |
| **State** | Simple Map | High-speed RAM store | HTTP State Machine |
| **Best For** | Learning/Low latency | Caching/Pub-Sub | Reverse Proxy/Static Web |

---

## 6. Full "Pro-Style" Reference Code

Below is the consolidated logic for a buffered, non-blocking broadcast server.

```go
package main

import (
	"fmt"
	"log"
	"syscall"
	"time"
)

type Client struct {
	fd         int
	outBuf     []byte
	lastActive time.Time
}

var allClients = make(map[int]*Client)

func main() {
	// 1. Create Kqueue
	kq, _ := syscall.Kqueue()

	// 2. Setup Listener (Simplified for brevity)
	// (Assume listenerFD is created and set to non-blocking)
	listenerFD := 3 

	// 3. Register Listener
	register(kq, listenerFD, syscall.EVFILT_READ)

	// 4. Start Janitor
	go janitor()

	// 5. Event Loop
	events := make([]syscall.Kevent_t, 100)
	for {
		n, _ := syscall.Kevent(kq, nil, events, nil)
		for i := 0; i < n; i++ {
			fd := int(events[i].Ident)

			if fd == listenerFD {
				// Handle New Connection
				connFD, _, _ := syscall.Accept(fd)
				syscall.SetNonblock(connFD, true)
				allClients[connFD] = &Client{fd: connFD, lastActive: time.Now()}
				register(kq, connFD, syscall.EVFILT_READ)
			} else if events[i].Filter == syscall.EVFILT_READ {
				// Handle Incoming Data
				handleRead(kq, fd)
			} else if events[i].Filter == syscall.EVFILT_WRITE {
				// Handle Outgoing Data (The Write Buffer)
				handleWrite(kq, fd)
			}
		}
	}
}

func register(kq, fd int, filter int16) {
	ev := syscall.Kevent_t{Ident: uint64(fd), Filter: filter, Flags: syscall.EV_ADD | syscall.EV_ENABLE}
	syscall.Kevent(kq, []syscall.Kevent_t{ev}, nil, nil)
}

func unregisterWrite(kq, fd int) {
	ev := syscall.Kevent_t{Ident: uint64(fd), Filter: syscall.EVFILT_WRITE, Flags: syscall.EV_DELETE}
	syscall.Kevent(kq, []syscall.Kevent_t{ev}, nil, nil)
}

func handleRead(kq, fd int) {
	client := allClients[fd]
	buf := make([]byte, 1024)
	n, err := syscall.Read(fd, buf)
	if n <= 0 || err != nil {
		cleanup(fd)
		return
	}
	client.lastActive = time.Now()
	// Logic to move data to outBuf of other clients...
}

func handleWrite(kq, fd int) {
	client := allClients[fd]
	if len(client.outBuf) == 0 {
		unregisterWrite(kq, fd)
		return
	}
	n, _ := syscall.Write(fd, client.outBuf)
	client.outBuf = client.outBuf[n:]
}

func cleanup(fd int) {
	delete(allClients, fd)
	syscall.Close(fd)
}

func janitor() { /* Loop and delete old clients */ }

```

---

### Final Takeaway

By manually managing File Descriptors and the Event Loop, you gain total control over memory and CPU. While Go's `net` package is usually sufficient, this "Reactor" pattern is the secret behind the fastest software on the planet.
