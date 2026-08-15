# How I Built an L7 Load Balancer in Go: Algorithms, Failover, and the Complexity of Shared State

*Load balancing looks simple, until you realize that at any instant a hundred requests are reading the same state while a health checker changes it. Building one in Go taught me where the real complexity lives.*

I built an L7 load balancer in Go. Not because Nginx, HAProxy, or Envoy are bad — they're excellent. I did it because a load balancer is a great lens for a problem all network software shares: **many goroutines reading and changing the same data at the same time**.

Every request has to be sent to a healthy backend. But the list of backends is never static. Backends fail, recover, get different weights, and are swapped in and out — all while requests keep arriving at full speed.

The interesting part is not the proxying. It's the state.

Go's model of goroutines, channels, and the standard library is enough to build real network software with almost no dependencies. This article is about what happens when the problem stops being *request composition* and becomes *state coordination*.

## What this POC demonstrates

- Five load balancing algorithms behind a single `Balancer` interface — and when to pick each one.
- A thread-safe backend pool, where request goroutines, the health checker, and the admin API all touch the same state safely.
- A failure model: health checks, automatic failover, and self-recovery.
- Hot configuration reload: changing backends and algorithms without restarting the process.
- Why the corner cases — a backend with weight zero, a canceled context, a nil request — are where a design reveals itself.
- A 98% code coverage gate that forces you to make the code testable from the start.

Let's walk through it.

## Architecture overview

Here is the component diagram.

[INSERT IMAGE] Captura: diagrama de componentes (cliente → HTTP listener → middleware chain → load balancer core → backend pool → backends, con health manager y metrics a los lados).

*Fig: component diagram of the balancer.*

The layout follows Go's standard conventions: `cmd/` holds the entrypoint, `internal/` holds the packages. Every package has one job. The entrypoint is deliberately thin — it loads config, wires components, and delegates.

[INSERT IMAGE] Captura: cmd/gobalancer/main.go (wiring del balancer, health checker, middleware chain y graceful shutdown).

*Fig: main.go — wiring the balancer, health checker, middleware chain, and graceful shutdown.*

Before anything else, there is one detail worth pointing out: a small wrapper that protects the active algorithm with a `sync.RWMutex`.

[INSERT IMAGE] Captura: la struct `safeBalancer` y su método `NextBackend` en cmd/gobalancer/main.go.

*Fig: safeBalancer — protecting the active algorithm with an RWMutex.*

Requests read the algorithm with `RLock`, and a config reload replaces it with `Lock`. This is the small piece that makes dynamic reconfiguration possible. It's also the first hint of the article's theme: the interesting code in a balancer is the code that controls access to state that changes.

## One interface, five strategies

The heart of the design is a single interface.

[INSERT IMAGE] Captura: internal/balancer/balancer.go — la interfaz `Balancer`.

*Fig: balancer.go — the `Balancer` interface every algorithm implements.*

Every algorithm — round robin, least connections, weighted round robin, random, and IP hash — implements it. A factory maps the configured name to the right implementation at startup, and stops with a clear error if the name is unknown.

[INSERT IMAGE] Captura: internal/balancer/balancer.go — la función `NewBalancer` con el switch de algoritmos.

*Fig: balancer.go — the factory that maps config names to algorithms.*

No reflection, no code generation. Adding a new algorithm means writing one struct that implements `NextBackend` and adding a case to the factory. The rest of the code never changes.

Now the interesting part: not *how* the algorithms work, but *when you would pick each one*.

- **Round Robin** — cycles through backends in order. Best for backends with similar capacity. Simplest thing that works, but it ignores load: a slow backend still gets its turn.
- **Least Connections** — sends the request to the backend with the fewest active connections. Best when requests take very different amounts of time (think streaming or uploads). It needs to count connections, which adds a little tracking.
- **Weighted Round Robin** — distributes traffic proportionally to capacity. Best for mixed hardware, like a 16-core server next to a 4-core one. The downside: weights are static, so they can't react to changing capacity.
- **Random** — picks a backend at random. Dead simple, and statistically balanced at scale. In short windows, bursts can skew the distribution.
- **IP Hash** — hashes the client IP to pick a backend. Best for session affinity (the same client always hits the same backend) without storing any session state. The cost: when a backend dies, all its clients get moved to other backends at once.

The last one deserves a bit more. Session affinity is normally done with sticky cookies or a session store. IP hash gives you most of the benefit with zero shared state: the same client always maps to the same backend, so that backend can safely cache per-client data in memory.

[INSERT IMAGE] Captura: internal/balancer/ip_hash.go — método `NextBackend` de `IPHash`.

*Fig: ip_hash.go — hashing the client IP for session affinity.*

FNV-32a is a deliberately boring hash: fast, predictable, and good enough for distribution. This is not the place for a cryptographic hash.

## The weighted round-robin corner case

The weighted algorithm is where the design gets genuinely interesting. This is the *smooth* weighted round robin: instead of sending requests in bursts (backend-1 gets five in a row, then backend-2 gets three...), it mixes them so the proportions hold at every moment.

[INSERT IMAGE] Captura: internal/balancer/weighted.go — método `NextBackend` de `WeightedRoundRobin`.

*Fig: weighted.go — the smooth weighted round robin algorithm.*

Look at this line: `if weight <= 0 { weight = 1 }`. A weight of zero is a config error — or a deliberate "take this backend out of rotation" signal. The algorithm keeps working instead of dividing by zero or crashing. This is one of those details that separates a solid implementation from a textbook one. Your config will be wrong sometimes; the code has to survive it. The test suite covers exactly this case: a backend with weight zero is never selected, and the balancer does not crash.

## Shared state: the real subject of this article

Here is the core. A `Backend` is not a passive data holder. It carries counters that are updated by request goroutines, health checks, and proxy errors — at the same time.

[INSERT IMAGE] Captura: internal/backend/backend.go — la struct `Backend` con sus campos atómicos.

*Fig: backend.go — the `Backend` struct with atomic counters.*

Notice the rule of thumb: **the counters on the hot path are atomic, the rare state changes use a mutex.**

An atomic counter is a value that many goroutines can update safely at the same time, without blocking each other. It's the right tool for the counters every request touches:

- `ActiveConnections` is increased with `atomic.AddInt64` on every proxied request, and decreased when it ends. No lock on the path every request goes through.
- `Failures` and `Successes` are atomic too — the health checker updates them from a background goroutine while requests read them.
- `Status` changes are rare and need a full write, so they take a `sync.RWMutex`.

This is the classic pattern: lock-free reads on the hot path, locked writes for rare changes. It's exactly what a load balancer needs — the path of a request should never wait on a mutex if it can be avoided.

The pool around these backends uses the same idea on a bigger scale.

[INSERT IMAGE] Captura: internal/backend/pool.go — la struct `BackendPool` y su método `GetBackends`.

*Fig: pool.go — the thread-safe backend pool.*

Every request calls `GetHealthyBackends()` — a read — so it takes an `RLock` and never blocks other requests. A config reload, which replaces the whole list, takes the exclusive `Lock`. Returning a *copy* of the list instead of the internal slice is a safety measure: callers can't accidentally change the pool's internal state.

If you build concurrent software in Go, this pattern — atomics for counters, `RWMutex` for structure, copies across boundaries — is the one to learn well. And you can keep it honest by running `go test -race`, the race detector that ships with Go.

## The failure model: health checks, failover, self-recovery

A backend is an entity that can go down. The health checker exists to notice, and the rest of the system exists to route around the damage.

The checker runs in a background goroutine. On every tick, it launches one goroutine per backend.

[INSERT IMAGE] Captura: internal/health/checker.go — método `CheckAll`.

*Fig: checker.go — one health check goroutine per backend.*

Concurrent health checks in three lines. No thread pool, no executor — the Go runtime schedules one goroutine per backend, each doing a single HTTP request with a timeout.

The failure logic lives in the `Backend` itself.

[INSERT IMAGE] Captura: internal/backend/backend.go — método `RecordFailure`.

*Fig: backend.go — marking a backend unhealthy after N failures.*

The threshold model — mark a backend unhealthy only after *N failures in a row* — exists to absorb small glitches. One failed check should not take a healthy server out of rotation. And recovery is eager: the first successful check resets the counter and flips the status back to HEALTHY, with a log line so you can see it happen.

[INSERT IMAGE] Captura: internal/health/checker.go — la lógica de recuperación en `CheckBackend`.

*Fig: checker.go — detecting that a backend recovered and marking it HEALTHY.*

Every algorithm calls `GetHealthyBackends()` before choosing, so unhealthy backends simply stop getting traffic. That's failover without a separate failover feature — the health state *is* the routing decision.

This is the moment worth appreciating: the correctness of the balancer is not in any single algorithm. It's in one simple guarantee: *a request is only ever sent to a backend that was healthy at the moment of selection*. Everything else — least connections, IP hash, weights — is optimization on top of that guarantee.

## Degradation: when every backend is down

What happens when there is no healthy backend? The proxy gets `ErrNoHealthyBackends` and returns a 503 instead of crashing.

[INSERT IMAGE] Captura: internal/proxy/proxy.go — la rama de error en `ServeHTTP` que devuelve 503.

*Fig: proxy.go — returning 503 when no backend is healthy.*

Failing gracefully is a feature. The system stays up, returns a meaningful status code, and records the error in the metrics. This is the kind of behavior that separates real infrastructure from a script.

There's a second failure path worth mentioning: when a proxied request fails in the middle, the proxy marks that backend as failed.

[INSERT IMAGE] Captura: internal/proxy/proxy.go — el `errorHandler` del ReverseProxy.

*Fig: proxy.go — marking a backend failed after a proxy error.*

So the balancer gets health information from two sources: the passive one (requests that fail) and the active one (periodic checks). Production load balancers do exactly this — passive detection is fast, active detection is systematic.

## Changing the menu without closing the restaurant

The admin API exposes a `POST /api/reload` endpoint. It reads the YAML config again, builds a *new* backend pool and a *new* balancer, and swaps them in one step.

[INSERT IMAGE] Captura: cmd/gobalancer/main.go — el callback `onReload`.

*Fig: main.go — the atomic config swap on reload.*

The key property: **the new state is fully built before anything is swapped**. There is no moment where the pool is half-old, half-new. The new pool and balancer are constructed completely; only then is the pointer replaced and the balancer swapped under the `safeBalancer` lock. Requests already in flight finish against the old configuration; new requests see the new one. This is a small version of the "build new, swap atomically" pattern behind zero-downtime deployments.

The admin API is small but useful, and it doubles as the observability surface:

- `GET /health` — is the balancer alive
- `GET /api/backends` — per backend: status, connections, latency, failures, successes
- `GET /api/stats` — healthy and unhealthy totals
- `POST /api/reload` — hot config swap
- `GET /metrics` — Prometheus-compatible metrics

## The middleware chain

Before the balancer ever sees a request, it passes through a global chain of middleware: Recovery → RequestID → Logger → Rate Limiter → mux. In Go, a middleware is just a function type, and composing them takes a handful of lines.

[INSERT IMAGE] Captura: internal/middleware/middleware.go — el tipo `Middleware` y la función `Chain`.

*Fig: middleware.go — composing the middleware chain.*

[INSERT IMAGE] Captura: cmd/gobalancer/main.go — la construcción de la cadena de middleware con `middleware.Chain(...)`.

*Fig: main.go — wiring the middleware chain.*

The rate limiter here uses a token bucket: it refills `rate` tokens per second, caps them at `capacity`, and keeps one bucket per client IP. A background goroutine cleans up stale buckets so the map never grows without bound — the same "background goroutine as a janitor" pattern, applied with slightly different trade-offs than a sliding window.

## Testability is a design driver, not an afterthought

The project enforces a **98% coverage threshold** in the build pipeline (`MIN_COVERAGE = 98.0` in the Rakefile), and meeting it forced some genuinely good design decisions. To test things in a predictable way, the code had to grow small hooks:

- The rate limiter accepts a configurable cleanup interval, so tests can trigger the eviction without waiting five minutes.
- The request ID generator wraps `rand.Read` in a package-level variable, so tests can inject a failing entropy source and check the fallback path.
- The health checker runs on a `context.Context`, so tests can cancel it and verify `Start` returns right away instead of leaking a goroutine.

Each of these is a tiny indirection that costs nothing in production and unlocks predictable tests. This is the strongest argument I know for coverage gates: they push you to *design for testability* instead of bolting tests on afterwards.

The corner-case tests are where the coverage shows its real value:

- A weighted round-robin with a zero-weight backend still returns a valid backend.
- A health checker whose context is canceled exits cleanly (no leaked goroutines).
- A nil request to `GetClientIP` returns a sensible default instead of panicking.
- A failed entropy read produces `req-unknown`, not an empty ID or a crash.

None of these are glamorous. All of them are the difference between software that survives unexpected real-world conditions and software that doesn't.

## The numbers that matter

This POC doesn't ship micro-benchmarks — the focus here is on *behavior under failure*, not raw throughput. What I can share is the resource profile, which is what really matters for infrastructure:

- **Goroutines (Go):** about 2.5 KB stack each. 10,000 of them use about 25 MB. Creation takes under 1 µs. Context switch about 200 ns.
- **Threads (Java/C++):** about 1 MB stack each. 10,000 of them use about 10 GB. Creation around 100 µs. Context switch around 10 µs.

And the deployment profile:

- **GoBalancer (Go):** ~15 MB image, starts in under 100 ms, ~8 MB idle memory.
- **nginx:alpine:** ~25 MB image, ~2 s startup, ~5 MB idle.
- **Python + Flask:** ~100 MB image, ~3 s startup, ~30 MB idle.
- **Java + Spring:** ~300 MB image, ~5 s startup, ~100 MB idle.

And the dependency budget says more than any benchmark: **one external dependency** (`yaml.v3`). The HTTP server, the reverse proxy, the transport, the graceful shutdown, the connection pool — all from the standard library.

## Why this POC matters

Building a load balancer reveals what really matters in network infrastructure:

- **The real challenge is shared state, and Go gives you exactly the building blocks for it** — atomics for the counters, `RWMutex` for the structure, goroutines for spreading work, channels for lifecycle. You reach for the right tool because it's *in the standard library*, not because a framework handed it to you.
- **Correctness under failure is a design choice, not a feature.** Health thresholds, atomic swaps, graceful degradation, and copies for safety are all decisions you make *before* something goes wrong. None of them need a big framework — they need discipline and a language that doesn't fight you.
- **A single `Balancer` interface carries the whole algorithm story.** Five strategies, one contract, zero changes to callers. Adding a sixth is an afternoon's work.
- **Coverage gates push you to build testable code.** The 98% threshold didn't just test the code; it shaped the code.
- **Deployment stays boring.** One binary, one ~15 MB image, cold start in milliseconds. For software that runs at the edge of every network, boring is a feature.

## What's missing

This is a POC, and it's honest about its scope:

- **No TLS termination.** Put Nginx, Traefik, or a cloud load balancer in front of it.
- **In-memory state.** The backend pool, rate-limit buckets, and metrics are lost on restart. Production would use Redis for distributed rate limiting and shared state.
- **No service discovery.** Backends come from static YAML. No DNS, Consul, or Kubernetes endpoints.
- **Single instance.** No clustering, no shared health consensus across nodes.
- **No formal benchmarks.** This POC is verified by behavior (tests, failover demo) rather than micro-benchmarks. Adding `Benchmark*` functions would be a natural next step.
- **No load-shedding under overload** beyond the rate limiter — no active-connection ceilings or circuit breakers.
- **RWMutex write-preferring behavior under load.** The `BackendPool` uses `sync.RWMutex` to protect the backend list. Reads (`RLock`) don't block each other, but when a config reload takes the exclusive `Lock`, new readers are queued behind it. Under high request throughput, this can cause p99 latency spikes during reloads. A production-grade alternative would be `atomic.Pointer` for lock-free reads with copy-on-write semantics, at the cost of extra memory per swap.

Each of these is a deliberate scope decision, and each is a natural extension point. The architecture doesn't have to be torn down to add any of them — which is, I think, the point.

## Conclusion

Building this load balancer taught me that network software is *stateful*, and the hard problem is keeping that state correct and consistent while a hundred goroutines touch it at once. A short `Chain` function composes the middleware; atomics keep the hot path lock-free; `RWMutex` guards the structure; goroutines spread the health checks; and a single `Balancer` interface keeps the whole algorithm story from leaking into the rest of the system.

The answer, in Go, was never a framework. It was the standard library: atomics for the hot path, `RWMutex` for the structure, goroutines for spreading work, `context.Context` for the lifecycle. And a `Balancer` interface that kept the entire algorithm story — five strategies, one contract — from ever leaking into the rest of the system.

If you're building network software, stop reaching for the framework first. Reach for the building blocks, make the failure modes explicit, and let coverage gates push you into designs that are testable by construction. Go makes all three feel like the default — and that, more than any benchmark, is why I keep choosing it for this kind of software.
