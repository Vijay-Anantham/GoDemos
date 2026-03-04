package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func SSEHandler(w http.ResponseWriter, r *http.Request) {
	// setting header
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache") // Prevent browser proxt to prevent caching older data
	w.Header().Set("Connection", "keep-alive")  // Tells TLS Layer to keep connection alive [TLS Sockets stay open instead of closing after first set of pockets are sent]
	// w.Header().Set("X-Accel-Buffering", "no")   // Tells nginx proxies to directly respond and not to add to the buffer before sending out responses

	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all request no cors restrictions.

	w.WriteHeader(http.StatusOK) // This is to avoid the error in curl where the curl expect a status code

	rc := http.NewResponseController(w)
	err := rc.Flush()
	if err != nil {
		log.Printf("Could not flush initial headers: %v", err)
		return
	}

	clientGone := r.Context().Done()

	t := time.NewTicker(time.Second)
	defer t.Stop() // graceful shutdown of timer after the server stops
	for {
		select {
		case <-clientGone:
			fmt.Println("Client Closed")
			return
		case <-t.C:
			data := fmt.Sprintf("data: The time is %s\n\n", time.Now().Format(time.TimeOnly))
			_, err := fmt.Fprint(w, data)
			if err != nil {
				log.Printf(" [IOWriter] Unhandled error in server: %v", err)
				return
			}

			err = rc.Flush() // Makes sure the response is sent immediately
			if err != nil {
				log.Printf(" [Flushing] Unhandled error in server: %v", err)
				return
			}
		}
	}
}

func main() {
	http.HandleFunc("/events", SSEHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Printf("error creating sse server: %v", err)
		return
	}
}

// Pro Tip: To prevent network timeouts, servers often send a "heartbeat" (an empty SSE comment) every 15–30 seconds to tell the network, "I'm still alive!"
// res.write(': heartbeat\n\n');
