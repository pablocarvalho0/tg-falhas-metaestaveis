# Experiment 2 — Methodology

**Server queue policies against the retry loop** · Lab Exame CSC-27 / CE-288 · ITA · November 2026

Companion documents: [PLAN.md](PLAN.md) (the design as planned) · [results.md](results.md) (the results). Experiment 1, which this one follows up, is the first LabExame bench and report (delivered separately).

## 1. Background and question

Experiment 1 showed a **metastable failure**. A service with a fixed queue, whose clients retry after a timeout, gets stuck after a transient database delay whenever the load exceeds 1/(N + 1) of its capacity, where N is the number of retries per request. Once stuck, the service stays stuck after the cause is gone: its workers stay fully busy on requests whose clients have already given up, and every client retry adds more of them. Neither exponential backoff with jitter nor dropping abandoned requests at dequeue fixed it.

Experiment 2 asks:

> Which **server queue policy** lets the service return to normal after a transient database delay, at loads where a fixed queue gets stuck? What does each policy cost when nothing is wrong? Does the answer depend on whether clients retry after a **fixed delay** or with **exponential backoff and jitter**?

## 2. Design

A full factorial design:

| Factor | Levels |
|---|---|
| Server queue policy | 1 fixed queue (baseline) · 2 fixed time bound · 3 per-request deadline · 4 adaptive LIFO |
| Client retry policy | fixed delay · backoff with jitter |
| Load (% of measured capacity μ) | 20 · 50 · 70 · 90 |
| Repetitions | 3 |

4 × 2 × 4 × 3 = **96 runs**. 20 % is below the frontier measured in experiment 1 (25 % for 3 retries), so even the baseline should recover there. 50, 70 and 90 % are above it, where the baseline is expected to get stuck.

## 3. The bench

Everything runs in Docker containers, with Docker Compose, in a 2-CPU, 2 GB Linux virtual machine (Colima) on a laptop.

| Component | Role |
|---|---|
| **Service** (Go) | 8 workers (C) and a queue of 64 slots (Q). Each request runs a Postgres query that takes T = 40 ms. The queue policy can be switched while the service is running, through `POST /admin/policy`. Work in progress is never cancelled. |
| **Load generator** (k6) | Open loop: new requests arrive at a fixed rate that does not depend on responses. Two client classes, and timeout and retry in the client (section 5). |
| **Toxiproxy** | Sits between the service and Postgres. The fault enters here: +200 ms on every database response for 20 s, without touching the service code. |
| **Postgres 17** | A real database. |
| **Prometheus** | Scrapes the service every 1 s and receives the client's metrics from k6 every 1 s. |
| **Orchestrator** (Go) | Calibrates, sets the queue policy, starts k6, turns the fault on and off at scheduled times, reads the series from Prometheus and writes a per-second CSV and a manifest for every run. |
| **Analysis** (Python) | Produces every table and figure from the CSVs. |

The unit tests (17 for the service, 23 for the orchestrator) and `go vet` run inside the image builds; an image is only created if they pass.

## 4. Server queue policies

All four policies use the **same physical queue (Q = 64)**, the same 8 workers and the same work. Q is the memory and admission capacity of the service, not the policy. A policy only decides which request a worker takes next and which ones are dropped. A dropped request is answered immediately with 503, if its client is still connected.

```
worker loop:
    item, drop = pick()            # under the queue lock
    if drop: reply 503; continue
    process(item)
```

### 4.1 Fixed queue (baseline)

```
arrival: if queue is full: reply 503 ("full"); else append
pick():  take the oldest; never drop
```

### 4.2 Fixed time bound

```
arrival: if queue is full: reply 503 ("full"); else append(enqueued_at = now)
pick():  item = oldest
         drop ("expired") if now − item.enqueued_at > W
```

W is the longest a request may wait and still finish within the client's timeout: W = timeout − Ŝ − margin. A fixed bound must use a single timeout for everyone, so it uses the **mean** timeout: W = 150 − 42 − 10 = **98 ms**.

### 4.3 Per-request deadline, checked at dequeue

```
arrival: if queue is full: reply 503 ("full"); else append(deadline = now + X-Budget-Ms)
pick():  item = oldest
         drop ("deadline") if now + Ŝ + margin > item.deadline
```

Each attempt carries its own timeout in the `X-Budget-Ms` header, so the server applies "max wait = timeout − service time − margin" with **each request's own** timeout. Ŝ = 42 ms (section 7) and the margin is 10 ms.

### 4.4 Adaptive LIFO

```
arrival: if queue is full: drop the OLDEST ("evicted"), then append the new one
pick():  overloaded = (now − last_time_queue_was_empty) > 100 ms
         if overloaded:
             drop ("stale") every item at the old end with now − enqueued_at > W
             item = newest
         else:
             item = oldest
         drop ("expired") if now − item.enqueued_at > W
```

- **Order.** FIFO normally; LIFO while overloaded, so fresh requests, which can still finish in time, are served first.
- **Overload detector.** The queue has not been empty for more than 100 ms (about 2.5 × the service time). A burst empties within that window; a standing queue never does. Switching back requires the queue to actually empty, which gives natural hysteresis. The detector's state is exported as a metric. A second detector (CoDel-style: the age of the oldest queued request stays above 5 ms for a whole 100 ms window) is implemented as a fallback. The pilots showed the first one works, so the campaign uses it.
- **Admission.** If LIFO rejected arrivals when the queue is full, stale requests that LIFO never takes would fill the 64 slots, and every fresh arrival would get a 503. So LIFO evicts the oldest request instead, and while overloaded it also purges expired requests from the old end. Policies 1–3 serve the oldest first, so their stale requests are at the head and are cleared as soon as a worker is free. They keep plain rejection when full.
- **W.** The same as policy 2 (98 ms), so that, apart from the admission rule LIFO requires, 2 and 4 differ only in the order of service.

### 4.5 What each comparison isolates

| Comparison | Isolates |
|---|---|
| 1 → 2 | adding a time bound |
| 2 → 3 | one fixed bound vs each request's own deadline |
| 2 → 4 | FIFO vs LIFO order under overload |

## 5. Clients

### 5.1 Two timeout classes

If every client had the same timeout, policy 3 would compute exactly the bound that policy 2 hard-codes, and the two policies would be identical by construction. Every run therefore mixes two classes:

| Class | Share of new requests | Timeout per attempt | Longest useful wait (timeout − Ŝ − margin) |
|---|---|---|---|
| impatient | 50 % | 100 ms | 48 ms |
| patient | 50 % | 200 ms | 148 ms |

The mean timeout is 150 ms, as in experiment 1. With W = 98 ms, policy 2 lets impatient requests wait about twice as long as they can afford, and drops patient requests before they need to be dropped. Each attempt sends `X-Budget-Ms` (its timeout) and `X-Client-Class`, so the server can count drops and wasted work per class.

### 5.2 Retry policies

Both retry policies retry at most **N = 3** times per request, and treat a timeout and a 503 the same way.

| Policy | Wait before retry k |
|---|---|
| fixed delay | 50 ms |
| backoff + jitter | uniform in [0, min(1000, 100 · 2^(k−1))] ms |

50 ms is the expected wait before backoff's first retry, so both policies start equally patient. They differ in that backoff grows and is random.

### 5.3 Open-loop arrivals with common random numbers

k6's `constant-arrival-rate` executor starts new requests at a fixed rate, independent of responses. Each new request's arrival offset (uniform within 1 s, to approximate a Poisson process) and its class come from a hash of (seed, global iteration number), not from `Math.random`. The seed depends only on (load, repetition). So **all 8 configurations of a cell see exactly the same sequence of new requests**; only the retries differ, because they depend on what the server did.

## 6. A run

Each run starts with the service idle and its policy set by the orchestrator through the admin endpoint, with no restart. Time is counted from the first request that reaches the service.

| Phase | Duration | What happens |
|---|---|---|
| warm-up | 10 s | load at the target rate |
| steady state | 30 s | measurement of normal operation, and of each policy's cost when nothing is wrong |
| fault | 20 s | Toxiproxy adds 200 ms to every database response; capacity drops to about 33 req/s |
| recovery | 90 s | fault removed; does the service come back on its own? |

Each run takes about 2 min 40 s including overhead. Within each repetition, the 32 configurations run in a shuffled order with a fixed seed (27), so slow drift of the machine cannot line up with one policy or load.

## 7. Calibration: measuring capacity μ and service time Ŝ

Calibration runs once, with the fixed queue and no fault. It is three 30 s windows at 300 req/s, about 1.5 times nominal capacity, with a 5 s timeout and no retries, so the 8 workers are busy all the time and nothing is abandoned. Each window is read directly from the service's `/metrics` at both ends:

- **μ**: requests completed per second.
- **Ŝ**: mean service time (sum ÷ count from the service's histogram), weighted by the number of requests in each window. The p50, p95 and p99 come from the same histogram.

Ŝ is accepted only if two checks pass:

1. **Repeatability:** the three windows agree within ±5 %.
2. **Consistency:** Ŝ explains the counted throughput. By Little's law at saturation, μ must equal (busy workers) ÷ Ŝ, within ±3 %. If it didn't, the measured time would not be what limits throughput.

The orchestrator then uses the measured values instead of the nominal ones: Ŝ goes to policy 3, and W = mean timeout − Ŝ − margin goes to policies 2 and 4. Both are sent to the service with every policy switch. Each run also records the mean service time during its own steady state, to check Ŝ at the real loads.

**Measured values:**

| | Window 1 | Window 2 | Window 3 | Result |
|---|---:|---:|---:|---|
| μ (req/s) | 190.66 | 190.34 | 190.33 | **190.45**, max deviation 0.11 % |
| Ŝ (ms) | 42.00 | 42.05 | 42.02 | **42.03**, max deviation 0.07 % |
| μ by Little's law (req/s) | 190.41 | 190.07 | 190.36 | at most 0.14 % from the count |
| service time p95 (ms) | 44.8 | 44.8 | 44.8 | |
| requests measured | 5,720 | 5,720 | 5,712 | |

Parameters used: **Ŝ = 42 ms, margin = 10 ms, W = 98 ms, overload window = 100 ms**. The mean service time during steady state stayed between 40.6 and 43.5 ms at every load, so Ŝ holds across the loads tested.

## 8. Metrics

Every metric is computed per second, from counters interpolated on the real sample timestamps. It is reported overall and per client class.

| Metric | Definition |
|---|---|
| **fraction answered in time** | requests answered within their timeout ÷ new requests; the primary outcome |
| **recovery time** | seconds from the end of the fault until the fraction answered in time, averaged over the next 5 s, reaches 95 % of its steady-state value |
| **outcome** | from the fraction answered over the last 30 s, relative to steady state: recovered (≥ 95 %), partial, stuck (≤ 50 %), or **no steady state** if the service was already answering < 95 % before the fault (a spontaneous collapse) |
| **wasted work** | requests the service processed after their client had given up (req/s) |
| **amplification** | attempts reaching the service ÷ new requests |
| **drops by reason** | full · evicted · expired · deadline · stale |
| **needless drops** | drops (other than "full") of requests that could still have finished in time: now + Ŝ + margin ≤ deadline |
| **steady-state drop rate** | drops per second before the fault: the cost of a policy when nothing is wrong |
| **LIFO mode on** | fraction of each phase with the overload detector active (policy 4) |
| **p99** | 99th percentile of the time in the service (queue + processing), from a histogram |

## 9. Measurement guards

A run is discarded and repeated automatically (up to 4 attempts) in three cases:

- it lasted longer than scheduled (the host was suspended);
- Prometheus is missing samples;
- the environment froze for more than 50 ms.

Freezes are measured by orchestrator threads pinned to each CPU, which sleep 10 ms in a loop and record how far each sleep overshoots. The criterion is independent of the run's result. The campaign runs on AC power with `caffeinate -dimsu`, so the Mac neither sleeps nor turns off the display. On battery, or with the display off, macOS slows the VM down: the pilot on battery measured μ = 153 req/s and Ŝ = 52 ms.

Other checks reported for each run: zero iterations dropped by k6 (so the load stays open), no client errors other than timeouts and 503s, no service errors talking to the database, and Little's law in steady state.

## 10. Procedure as executed

1. Unit tests in the image builds.
2. **Pilot** (battery, morning of 21 Nov): each policy at 70 % with the fixed-delay client. Policy 1 got stuck and 2–4 recovered. The LIFO detector was on for 100 % of the fault and off in steady state and recovery, so the CoDel fallback was not needed. Because the pilot ran on battery, the calibration was redone on AC power, and Ŝ and W were derived from calibration instead of being hard-coded.
3. **Calibration** on AC power (section 7).
4. **Repetition 1** (32 runs, 12:25–13:57 UTC). It was interrupted once when the session ended, and resumed by reusing the runs already saved. One run was discarded by the freeze guard (76 ms) and repeated.
5. **Repetitions 2 and 3** (64 runs, started 13:57 UTC). They reuse repetition 1, so `resultados/principal/` holds all 96 runs once they finish. results.md currently reports repetition 1.
6. Analysis: `./bancada.sh analise --varredura principal`.

## 11. Reproducing

```sh
cd exame_2
caffeinate -dimsu ./experimentos.sh                  # calibration + 96 runs + analysis, about 4 h 30, on AC power
./bancada.sh varredura -nome principal -reps 1       # only the first repetition (32 runs)
./bancada.sh analise --varredura principal           # tables and figures
```

Raw data are in `resultados/principal/execucoes/<id>/`: `series.csv` (1 s resolution) and `manifesto.json` (configuration, parameters, real fault instants, software versions, summary). Each sweep also has a `resumo.csv` with one line per run.

Software: k6 1.3.0, Toxiproxy 2.12.0, Postgres 17, Prometheus 3.5.0, Go 1.25.0, Linux 6.8 (arm64).

## 12. Limitations of the method

- **One laptop.** Client, service and database share 2 virtual CPUs. Short hiccups of the environment exist (freezes up to 47 ms in accepted runs) and can act as triggers.
- **Short recovery window.** "Stuck" means stuck for the 90 s observed.
- **One fault type, one service-time distribution** (nearly constant, about 42 ms), and bounded retries without a retry budget.
- **Untuned parameters.** W, the margin and the overload window are fixed by the rules above, not tuned per policy. A fixed bound tuned for the shortest timeout would behave differently from the one tested.
