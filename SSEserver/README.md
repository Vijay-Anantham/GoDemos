Professional architecture and security guide for Server-Sent Events (SSE). This document covers the "Why," "How to Scale," and "Production Hardening" of an SSE-based system.

---

# 🚀 Server-Sent Events (SSE) Architecture Guide

## 1. The Architectural Decision: Why SSE?

Choosing SSE over alternatives like WebSockets or Long Polling depends on your **data flow direction** and **infrastructure constraints**.

### When to choose SSE:

* **One-Way Traffic:** Your server pushes updates, but the client rarely (or never) sends data back over that same channel (e.g., Dashboards, AI text generation/streaming, Notifications).
* **Infrastructure Simplicity:** You want to use standard HTTP/HTTPS and avoid the complexity of managing a separate WebSocket (`ws://`) protocol.
* **Native Reliability:** You need automatic reconnection and "message catching up" (Last-Event-ID) without writing custom client-side retry logic.
* **Low Battery/Resource Usage:** SSE is generally lighter on mobile devices than a full-duplex WebSocket connection.

### When to avoid SSE:

* **Bidirectional Real-time:** If you're building a multiplayer game or a collaborative editor (e.g., Google Docs), WebSockets are superior.
* **Binary Data:** SSE is text-based (UTF-8). If you need to stream raw binary packets, use WebSockets.

---

## 2. Horizontal Scaling Strategy

A single server can handle thousands of connections, but for millions, you must scale horizontally.

### The "Silo" Problem

If **User A** is connected to **Server 1**, and an event occurs on **Server 3**, Server 3 has no way to "talk" to User A's socket.

### The Solution: Redis Pub/Sub Backplane

1. **Subscription:** When a client connects to *any* server, that server subscribes to a specific Redis channel (e.g., `user:123` or `broadcast:news`).
2. **Publishing:** When your backend logic triggers an update, it publishes a message to Redis.
3. **Distribution:** Redis broadcasts that message to all server instances. Only the server that holds the active connection for that user picks it up and writes it to the HTTP response.

> **Architecture Note:** Use **HTTP/2** in production. It allows multiple SSE streams to share a single TCP connection, bypassing the browser's 6-connection limit per domain.

---

## 3. Production Security & Hardening

Standard HTTP security rules apply, but SSE has unique "long-lived" vulnerabilities.

### A. Authentication (JWT vs. Cookies)

* **JWT in Headers:** Standard `Authorization: Bearer <token>` works perfectly for the initial request.
* **Token Expiration:** Since an SSE connection can stay open for hours, a token might expire *while* the connection is active.
* **Best Practice:** Check the token on the initial connection. If it expires, let the connection continue, but force a reconnection/re-auth if the client drops.



### B. Cross-Site Request Forgery (CSRF)

* Because SSE is a `GET` request, it is susceptible to CSRF if you rely solely on session cookies.
* **Defense:** Always verify the `Origin` and `Referer` headers. Better yet, use a custom header (like `X-Requested-With`) or a JWT in the `Authorization` header, which browsers won't send automatically for malicious cross-site requests.

### C. Resource Protection (Rate Limiting)

* **Connection Limits:** Limit the number of SSE connections per User ID or IP address to prevent "Connection Exhaustion" attacks.
* **Heartbeats:** Implement a "Keep-Alive" heartbeat (a simple `:` comment) every 15–30 seconds. This prevents aggressive corporate firewalls and load balancers from killing "idle" connections.

### D. Data Privacy

* **The "Broadcaster" Trap:** Ensure your Redis channels are scoped correctly. Never publish sensitive user data to a global `all_users` channel. Create per-user channels (e.g., `user:{uuid}:private`).

---

## 4. Operational Checklist for Production

| Task | Why? |
| --- | --- |
| **Enable Gzip/Brotli** | Drastically reduces bandwidth for text-heavy JSON streams. |
| **Set `WriteTimeout: 0**` | Prevents the Go/Node server from killing the connection after 30s. |
| **Configure Nginx** | Set `proxy_buffering off` and `proxy_read_timeout 24h`. |
| **Implement `retry**` | Send `retry: 5000\n\n` to tell clients how long to wait before reconnecting. |

---

Would you like me to provide a **Redis-powered Go snippet** to show exactly how the horizontal scaling part looks in code?