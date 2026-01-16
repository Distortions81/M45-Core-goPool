# Performance (Operator Notes)

This is a practical, operator-friendly reference for “how much can this box
handle?” using simple benchmarks. Most of it focuses on **CPU**; there’s also a
small **network bandwidth** section at the end for gigabit vs 10 gig ballparks.

These numbers are meant as a *ballpark*, not a guarantee. Real deployments will
hit other limits too (file descriptors, memory, kernel/network overhead, TLS,
disk logging, etc.).

## Reference machine (for the numbers below)

- CPU: AMD Ryzen 9 7950X 16-Core Processor
- OS/Arch: linux/amd64
- Go: go1.24.11

## Quick capacity picture (CPU-only, network ignored)

We assume **15 shares/min per worker** for rough planning.

- **Share handling headroom is huge on this CPU.** Even including submit
  parsing/validation, we measured about **~1.16M shares/sec** of CPU throughput.
  At 15 shares/min/worker (0.25 shares/sec/worker), that’s a *theoretical*
  **~4.6M workers at 100% CPU** for share checking alone.
- **What will limit “snappy dashboard” first is status rebuilding**, because it
  scales with the number of connected miners and is paid periodically (and on
  the first request after the cache expires).

In other words: for “web UI feels fast”, plan around the dashboard/status work,
not around share hashing.

## What uses CPU (and what it means)

- **Every share submitted by miners**: parse the message, validate it, do the
  proof-of-work checks, update stats, and send a response. More shares/min per
  worker means more CPU per worker.
- **Keeping the dashboard fresh**: the server rebuilds a status snapshot by
  scanning connections. With more connected miners, this takes longer.
- **Serving the web/API**: turning that snapshot into JSON responses costs some
  CPU, but it’s usually smaller than the snapshot rebuild itself.

## “Low-latency max workers” (dashboard rebuild budget)

If you want the UI to feel responsive, a simple rule of thumb is:

“How many connected workers can we scan/rebuild in **X ms**?”

On the reference 7950X, measured rebuild budgets are roughly:

- ~`2.7k` workers @ `5ms` (≈ `1.9k` @ ~70% CPU)
- ~`5.3k` workers @ `10ms` (≈ `3.7k` @ ~70% CPU)
- ~`8.0k` workers @ `15ms` (≈ `5.6k` @ ~70% CPU)
- ~`16.0k` workers @ `30ms` (≈ `11.2k` @ ~70% CPU)
- ~`32.1k` workers @ `60ms` (≈ `22.5k` @ ~70% CPU)

Notes:

- The status snapshot is cached and only rebuilt about once per
  `defaultRefreshInterval` (currently 10s), so most requests are “cheap reads”.
  The “spike” happens on rebuild.
- At **15 shares/min per worker**, share CPU is not the limiting factor at these
  worker counts (e.g. `10k workers` ≈ `2.5k shares/sec`, far below the measured
  share-processing throughput).

## Putting it together (realistic CPU-only ballparks)

There are two different “max worker” stories:

- **Average CPU load** (amortized over time): combines shares + periodic status
  rebuilds using their refresh intervals.
- **Worst-case latency** (spikes): how long a status rebuild takes when it runs.

### Average CPU load (70% CPU target)

Using the reference benchmarks:

- Share handling (15 shares/min/worker) costs ~`858 ns` per share.
  - Per worker: `0.25 shares/sec * 858 ns ≈ 215 ns/sec` of CPU time.
  - At 70% CPU: ballpark **~3.2M workers** for share processing alone.
- Status rebuild cost is ~`~1.9 µs/worker` per rebuild and happens every ~`10s`.
  - Per worker: `1.9 µs / 10s ≈ 190 ns/sec` of CPU time.
  - At 70% CPU: ballpark **~3.7M connected workers** for rebuild CPU alone.

If you combine those two costs (shares + rebuild CPU), the CPU-only “math max”
lands around **~1.7M connected workers at ~70% CPU** on this 7950X.

This number is intentionally conservative and still ignores real-world limits
like memory, goroutines, open sockets, TLS, and the kernel/network stack.

### Worst-case latency (UI “snappiness”)

Even if the *average* CPU is fine, very large worker counts can cause the
dashboard rebuild to take tens of milliseconds when it runs. For a UI that
“feels instant”, the rebuild budgets in the section above are the more useful
guide (e.g. ~`5k` workers @ `10ms`).

## Re-running these numbers on your hardware

Run the two benchmarks:

```bash
go test -run '^$' -bench 'BenchmarkHandleSubmitAndProcessAcceptedShare$' -benchmem
go test -run '^$' -bench 'BenchmarkBuildStatusData$' -benchmem
```

If you want a CPU profile (to see what’s taking time):

```bash
go test -run '^$' -bench 'BenchmarkHandleSubmitAndProcessAcceptedShare$' -cpuprofile cpu_submit.out
go tool pprof -top ./goPool.test cpu_submit.out
```

If you want a portable CPU profile suitable for PGO builds (and an easy SVG you
can open in a browser):

- `default.pgo` (CPU profile output): [default.pgo](default.pgo)
- `profile.svg` (Graphviz render): [profile.svg](profile.svg)

Generate `default.pgo` with the `-profile` flag (writes a 60s CPU profile), then
render it with:

```bash
./scripts/profile-graph.sh default.pgo profile.svg
```

## Saved workers dashboard (how many people can watch?)

The saved workers page refreshes every **5 seconds** and (usually) checks a
small list of saved workers. In the UI and DB we cap this at **64 saved workers
per user**; the common case is much smaller (e.g. 15).

On the reference 7950X, with a realistic “15 saved workers” list (10 online / 5
offline), the pool can serve roughly:

- ~`26k` saved-workers refreshes per second (CPU-only)
- That’s ~`130k` concurrent viewers refreshing every 5 seconds
- At ~70% CPU: ~`91k` concurrent viewers (CPU-only)

In practice, network/TLS overhead and whatever else the machine is doing will
reduce this, but the main takeaway is that “saved workers page viewers” are not
usually a CPU bottleneck compared to managing the miners themselves.

## Network ballparks (gigabit vs 10 gig)

These are **bandwidth-only** estimates (not CPU), assuming:

- **15 shares/min per worker** (0.25 shares/sec)
- Stratum traffic is “typical” (shares + responses + occasional `mining.notify`)
- We aim to use ~**70%** of link capacity to avoid living at the edge

As a conservative rule of thumb, plan for **~1 KB/sec per worker** of total
traffic (up + down). On this assumption:

- **1 Gbit**: ~`87k` workers (70% of 1 Gbit)
- **10 Gbit**: ~`875k` workers (70% of 10 Gbit)

If your miners/pool send more frequent or larger `mining.notify` messages, or
you’re using TLS everywhere, a safer “heavy traffic” assumption is **~2 KB/sec
per worker**, which halves the numbers:

- **1 Gbit (heavy)**: ~`44k` workers
- **10 Gbit (heavy)**: ~`438k` workers

In practice, long before you hit these bandwidth limits you may hit other real
world limits: file descriptors, kernel packet-per-second overhead, memory, and
the CPU/UI limits earlier in this document.

## Stratum latency optimizations

goPool includes several optimizations to minimize share response latency, which
is particularly important for miners that display "ping" measurements (like
Bitaxe/ESP-Miner). These optimizations reduce the round-trip time from when a
miner sends `mining.submit` to when it receives the pool's response.

### TCP tuning (automatic)

- **TCP_NODELAY**: Disables Nagle's algorithm on all stratum connections. This
  prevents the kernel from buffering small packets (like stratum responses)
  waiting for more data, eliminating up to 40ms of artificial delay.

- **TCP keepalive** (30-second period): Enables OS-level connection probing to
  detect dead connections faster than the default (often 2+ hours). This does
  NOT affect idle miners—they can go minutes without submitting shares. It only
  detects truly dead connections (crashed miner, network failure).

### Sync share processing (default: enabled)

By default, `sync_share_processing = true` processes share submissions
synchronously in the miner's read goroutine instead of queueing to a worker
pool. This eliminates queue latency at the cost of blocking the read loop
during hash computation (~50-100µs on modern CPUs).

**When to disable**: For very high-traffic pools (thousands of miners), set
`sync_share_processing = false` in your config to use the async worker pool,
which provides better throughput under heavy load.

```toml
# In config.toml - only disable for high-traffic pools
sync_share_processing = false
```

### Optimistic share response (default: enabled)

When `optimistic_share_response = true` (the default), the pool sends the "accepted" response
**immediately** after basic parameter validation, before full share verification
completes. This minimizes displayed latency (ping) by responding in ~1-2ms
instead of 20-40ms.

**Trade-offs**:
- Invalid shares are still rejected (not counted toward rewards), but the miner
  has already received "accepted" feedback
- The miner's displayed ping will be much lower
- Validation still happens; only the response timing changes

```toml
# In config.toml - disable if you prefer response after validation
optimistic_share_response = false
```

**Note**: This is safe for solo pools since invalid shares simply don't count.
The miner already submitted the work; whether validation happens before or
after the response doesn't change the outcome.

### Message size optimizations

- **Incremental job IDs**: Job IDs use short incremental base36 encoding
  (`"1"`, `"2"`, ... `"a"`, `"b"`, ... `"10"`...) instead of timestamps. First
  ~1300 jobs fit in 1-2 characters, saving 8-17 bytes per `mining.notify`.

- **Pre-serialized accept responses**: Share accept responses
  (`{"id":N,"result":true,"error":null}`) are formatted directly for integer
  IDs, bypassing JSON marshaling overhead.

- **Smaller read buffer** (8KB): More cache-friendly for typical stratum
  messages (<2KB). The buffer grows automatically for larger messages.

### Expected latency breakdown

On LAN with ~8ms ICMP ping, typical share response latency is **20-40ms**:

| Component | Time |
|-----------|------|
| Network RTT (TCP) | ~10-15ms |
| JSON parse (request) | ~0.1ms |
| Share validation + hash | ~0.05-0.1ms |
| JSON format (response) | ~0.05ms |
| Miner-side processing | ~5-15ms |

The miner's own processing time (ESP32 JSON parsing, timing measurement
overhead) contributes significantly to the displayed "ping" value. The pool's
actual processing time is typically <1ms.

### Further latency notes

- **ICMP vs TCP**: ICMP ping measures raw network latency. TCP adds connection
  state overhead, so expect TCP round-trips to be 20-50% higher than ICMP.

- **Miner firmware**: The displayed "ping" includes the miner's own processing
  time. Different firmware versions may report different values for the same
  pool latency.

- **TLS overhead**: If using stratum TLS (`stratum_tls_listen`), expect ~1-2ms
  additional latency for encryption/decryption.
