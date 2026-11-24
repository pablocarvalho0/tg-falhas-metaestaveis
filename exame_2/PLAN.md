# Experiment 2 — Server queue policies against the retry loop

> **This is the original plan**, kept as a record of the design decisions. The experiment as actually executed — with Ŝ and W measured in calibration (Ŝ = 42.03 ms, W = 98 ms) instead of the nominal 42 and 100 ms — is described in [methodology.md](methodology.md), and its results in [results.md](results.md). Where they differ, methodology.md is authoritative.

Follow-up to experiment 1, the first LabExame bench (delivered separately). That experiment showed that with retries the service gets stuck after a transient database delay whenever load exceeds 1/(N + 1) of capacity, and that neither backoff with jitter nor cancelling abandoned requests at dequeue fixes it. This experiment compares four server queue policies, under two client retry policies, in a single factorial design.

Only the pieces needed here were carried over from experiment 1's bench (service, load script, orchestrator, infra, analysis).

## 1. Question

Which server queue policy lets the service return to normal after a transient database delay, at loads where a plain fixed queue gets stuck, and at what cost when nothing is wrong? Does the answer depend on whether clients retry after a fixed delay or with exponential backoff and jitter?

## 2. Factors

| Factor | Levels |
|---|---|
| Server queue policy | 1 fixed queue (baseline) · 2 fixed time bound · 3 per-request deadline · 4 adaptive LIFO |
| Client retry policy | fixed delay · backoff with jitter |
| Load (% of μ) | 20 · 50 · 70 · 90 |
| Repetitions | 3 |

4 × 2 × 4 × 3 = **96 runs**. About 2.7 min each, so roughly 4 h 20 of machine time.

- **20 %** is below the old frontier (25 % for 3 retries): even the baseline should recover, so this checks that the new policies do not hurt the easy case.
- **50, 70 and 90 %** are above it, where the baseline gets stuck.

## 3. Clients: two timeout classes

If every client used the same timeout, policy 3 (each request's own deadline) would compute exactly the value policy 2 has hard-coded, and the two would be identical by construction. So every run mixes two client classes:

| Class | Share of new requests | Timeout per attempt | Longest useful wait (timeout − 42 ms service − 10 ms margin) |
|---|---|---|---|
| impatient | 50 % | 100 ms | 48 ms |
| patient | 50 % | 200 ms | 148 ms |

The mean timeout is 150 ms, as in the first experiment. Policy 2 must pick one limit for everyone and is tuned for that mean (W = 100 ms), so it serves some impatient requests too late and drops some patient ones needlessly. Policy 3 uses each request's own budget. Every metric is reported per class.

Each attempt carries two headers:

- `X-Budget-Ms`: the attempt's timeout (100 or 200). The server turns it into a deadline: arrival time + budget. Only policy 3 uses it to decide; all policies use it to count needless drops.
- `X-Client-Class`: `impatient` or `patient`, only for per-class metrics on the server.

### Retry policies

Both retry at most **N = 3** times per request and treat a timeout and a 503 the same way.

| Policy | Wait before retry k |
|---|---|
| fixed delay | constant 50 ms |
| backoff + jitter | uniform in [0, min(1000, 100 · 2^(k−1))] ms |

50 ms is the expected wait before backoff's first retry, so both start equally patient; they differ in that backoff grows and is random.

## 4. Server policies

All four share the same physical queue, **Q = 64**, the same 8 workers and the same work. Q is the memory/admission capacity, not the policy; the policies only decide which request a worker takes and which ones are dropped. A dropped request gets a 503 immediately (if its client is still connected).

Common worker loop:

```
loop:
    item, drop = pick()            # under the queue lock
    if drop: reply 503; continue
    process(item)
```

### 4.1 Fixed queue (baseline)

```
arrival: if len(queue) == 64: reply 503 "full"; else push
pick():  take the oldest; never drop
```

### 4.2 Fixed time bound, W = 100 ms

```
arrival: if len(queue) == 64: reply 503 "full"; else push(enq = now)
pick():  item = oldest
         drop ("expired") if now − item.enq > W
```

W = timeout − service time − margin = 150 − 42 − 10 ≈ 100 ms, for the mean timeout. Expired requests at the head are dropped at no cost, so one free worker clears the whole stale prefix at once.

### 4.3 Per-request deadline (checked at dequeue)

```
arrival: if len(queue) == 64: reply 503 "full"; else push(deadline = now + X-Budget-Ms)
pick():  item = oldest
         drop ("deadline") if now + Ŝ + margin > item.deadline
```

Ŝ = 42 ms (service time measured in calibration), margin = 10 ms. This is "max wait = timeout − service time − margin" with each request's own timeout.

### 4.4 Adaptive LIFO, W = 100 ms

```
arrival: if len(queue) == 64: drop the OLDEST ("evicted"), then push the new one
pick():  overloaded = (now − lastEmpty) > 100 ms        # lastEmpty: last time the queue was seen empty
         if overloaded:
             drop ("stale") every item at the old end with now − enq > W
             item = newest
         else:
             item = oldest
         drop ("expired") if now − item.enq > W
```

**Why admission differs from policies 1–3.** If LIFO kept "reject when full" and only checked waiting time on the item it takes, the bottom of the queue would fill with old requests that are never taken (LIFO always takes the newest): the 64 slots would hold stale requests and every fresh arrival would be refused with 503, exactly the requests LIFO wants to serve. Two rules prevent that:

- **evict the oldest when full**: a fresh arrival always gets in, and the request that leaves is the one least likely to still be useful;
- **purge expired requests from the old end** while overloaded, so stale requests do not hold slots until eviction reaches them.

Policies 1–3 serve the oldest first, so their stale requests sit at the head and are dropped (2, 3) or served (1) as soon as a worker frees; they keep plain "reject when full". LIFO uses the same W as policy 2, so 2 vs 4 differs only in the order of service (plus the admission rule that LIFO needs).

**Overload detector.** First choice: "the queue has not been empty for more than 100 ms" (about 2.5 × the service time). A burst empties within that window; a standing queue never does. Switching back needs the queue to actually empty, which gives natural hysteresis. The detector state is exported as a metric so the pilots can check that it turns on in the stuck state and off after recovery.

**Fallback, if the pilots show it does not work:** a CoDel-style rule, overloaded when the age of the *oldest* queued request stays above 5 ms for a whole 100 ms window (the queue never cleared). It looks at the oldest queued request rather than at the requests being served, because in LIFO mode the served requests are always fresh and would switch the detector off while the queue is still full. Both are implemented behind a setting (`OVERLOAD_DETECTOR=empty|codel`); the pilots decide which one goes into the campaign.

### The comparison ladder

| Comparison | What it isolates |
|---|---|
| 1 → 2 | adding a time bound |
| 2 → 3 | one fixed bound vs each request's own deadline |
| 2 → 4 | FIFO vs LIFO under overload |

## 5. Fixed parameters

| Parameter | Value |
|---|---|
| Workers C, service time T | 8, 40 ms query (≈ 42 ms measured), μ ≈ 192 req/s |
| Queue capacity Q | 64, all policies |
| Client timeouts | 50 % 100 ms, 50 % 200 ms |
| Retries N | 3 |
| Ŝ (policy 3) | service time measured in calibration (42 ms on AC power) |
| margin | 10 ms |
| W (policies 2 and 4) | mean timeout − Ŝ − margin = 150 − 42 − 10 ≈ 100 ms |
| Overload window (policy 4) | 100 ms |
| Fault | +200 ms on every database response, 20 s |
| Phases | warm-up 10 s, steady state 30 s, fault 20 s, recovery 90 s |

## 6. Fairness controls

1. **Same arrivals for every configuration.** The client derives each new request's arrival offset and class from a hash of (seed, global iteration number), with seed = f(load, repetition). All 8 configurations of a (load, repetition) cell see the same sequence of new requests; only retries differ, because they depend on the server.
2. **Shuffled order.** The 32 configurations of each repetition run in a random order with a fixed seed, so slow drift of the machine cannot line up with one policy.
3. **Policy switched at runtime.** The service exposes `POST /admin/policy`; the orchestrator sets the policy before each run (with the service idle). No restarts, so every run starts from the same warmed-up service.
4. **Environment guards** from the first experiment: a run is discarded and repeated if the environment freezes for more than 50 ms or Prometheus samples are missing. The campaign runs with `caffeinate -dimsu` and the display on.
5. **One calibration** of μ and Ŝ with the fixed queue and no fault. The orchestrator then derives W = mean timeout − Ŝ − margin and sends Ŝ, W, margin and the overload window to the service with each policy switch, so the policies use the real service time of the machine, not the nominal one.
6. **Same drop response** (immediate 503) for every policy; both clients handle it identically.

## 7. Metrics

Per run, overall and per class, at 1 s resolution:

| Metric | Why |
|---|---|
| recovery time after the fault | the main question |
| fraction answered in time, last 30 s ÷ steady state; outcome recovered / partial / stuck | the outcome |
| collapsed before the fault | policies 2–4 should also prevent the spontaneous collapses seen before |
| wasted work (processed after the client gave up) | what keeps the loop alive |
| drops by reason: full, evicted, expired, deadline, stale | separates kinds of drop |
| needless drops: dropped while `now + Ŝ + margin ≤ deadline` | where policy 3 should beat policy 2 (patient class) |
| amplification (attempts ÷ new requests) | retry pressure |
| p99 of successful responses, per class | policy 4 should give fresh requests the best tail |
| LIFO mode active (policy 4) | checks the overload detector |
| steady-state drop rate | the cost of each policy when nothing is wrong |

## 8. Hypotheses

1. Policy 1 gets stuck at 50–90 % with both clients (as measured before); 20 % recovers.
2. Policies 2–4 recover at every load, within seconds, with both clients, and prevent the collapses before the fault.
3. Policy 3 beats policy 2 within each class: less wasted work for impatient clients, fewer needless drops for patient ones.
4. Policy 4 recovers fastest and has the best p99 under overload.
5. Backoff vs fixed delay barely matters with policy 1; with 2–4, backoff may smooth the burst of 503s and retries right after the fault.

## 9. Procedure

1. Build and run the unit tests (they run inside the image builds): `./bancada.sh testes`.
2. Calibrate μ and Ŝ: `./bancada.sh calibrar`.
3. **Pilots**: each policy at 70 % with the fixed-delay client, short phases. Check that 1 gets stuck and 2–4 recover, and that the LIFO detector turns on during the fault and off afterwards. If it does not, switch to the CoDel detector (`-detector codel`) and repeat.
4. Main campaign, **on AC power**: `caffeinate -dimsu ./experimentos.sh` (calibration if missing, 96 runs, analysis).
5. Analysis only: `./bancada.sh analise --varredura principal` → `resultados/tabelas_principal.md` and `resultados/figuras/principal_carga*.png`.

### Pilot results (2026-11-21)

Run on battery power, which slowed the machine: μ = 153.5 req/s and Ŝ ≈ 52 ms instead of 192 req/s and 42 ms, and Prometheus missed some scrapes (several runs were discarded and repeated by the guards). The pilots used Ŝ = 42 and W = 100, before W and Ŝ were derived from calibration. They validate the logic, not the numbers.

| Policy (fixed-delay client, 70 %) | Outcome | Notes |
|---|---|---|
| 1 fixed queue | stuck: 0 % answered at the end, amplification 4.0 | as in experiment 1 |
| 2 fixed time bound | recovered in 0 s | relapse 8 s after recovery, for the impatient class only (goodput 3/s, 143 req/s wasted), then recovered again: W was sized for the mean timeout and Ŝ was underestimated, so impatient requests were served late. This is the weakness the per-request deadline is meant to fix. |
| 3 per-request deadline | recovered in 0 s | a burst of deadline drops during steady state (an environment hiccup) lowered the impatient class's steady-state fraction to 94 % |
| 4 adaptive LIFO | recovered in 0 s | **detector works**: LIFO mode on 100 % of the fault, 0 % in steady state and in recovery. The CoDel fallback was not needed. |

**LIFO admission**, as described in 4.4, behaved as intended: during the fault the queue held fresh requests, with about 160 evictions and 240 stale purges per second, and no "full" rejections.

## 10. Changes from experiment 1's bench

| Where | Change |
|---|---|
| service | queue as a locked deque instead of a channel; `QUEUE_POLICY = fifo / timebound / deadline / lifo` with W, Ŝ, margin, overload window and detector; reads `X-Budget-Ms` and `X-Client-Class`; `POST /admin/policy`; drop counters by reason and class; needless-drop counter; LIFO-mode gauge. The cancellation option of the first experiment is removed. |
| load script | two timeout classes; fixed-delay retry; budget and class headers; arrivals and classes from a seeded hash of the iteration number; per-class counters |
| orchestrator | server policy as a sweep dimension, set through the admin endpoint; per-class and per-reason columns in the CSV and summary; the direct-to-Postgres service and the proxy-overhead validation are dropped (already measured in experiment 1) |
| analysis | 4 × 2 grid per load; per-class tables |
