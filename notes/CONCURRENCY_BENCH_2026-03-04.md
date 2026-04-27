# Concurrency Bench Results (2026-03-04)

This run validates value-over-serial for `ductile-gt7` using the stress plugin in Docker.

## Setup
- Branch: `feature/ductile-gt7-concurrency-v1`
- Container limits during test: 6 CPU, 8 GB RAM
- Workloads: `stress cpu` (1s) and `stress io` (20/50/100MB)
- Profiles:
  - serial: `max_workers=1`, `parallelism=1`
  - capped: `max_workers=8`, `parallelism=2`
  - parallel: `max_workers=6`, `parallelism=6`

## CPU (120 jobs, 1s each)
| profile | q_wait p50 | q_wait p95 | run p50 | run p95 | wall |
|---|---:|---:|---:|---:|---:|
| serial (1x1) | 63.995s | 122.363s | 1.082s | 1.090s | 130.123s |
| capped (8x2) | 31.476s | 60.652s | 1.082s | 1.087s | 65.136s |
| parallel (6x6) | 10.849s | 21.014s | 1.121s | 1.164s | 23.447s |

## IO (40 jobs)
### 20MB
| profile | q_wait p50 | run p50 | wall |
|---|---:|---:|---:|
| 1x1 | 3.151s | 0.131s | 5.966s |
| 8x2 | 1.680s | 0.135s | 3.201s |
| 6x6 | 1.137s | 0.190s | 1.970s |

### 50MB
| profile | q_wait p50 | run p50 | wall |
|---|---:|---:|---:|
| 1x1 | 4.401s | 0.209s | 8.761s |
| 8x2 | 2.331s | 0.215s | 4.736s |
| 6x6 | 1.249s | 0.306s | 2.594s |

### 100MB
| profile | q_wait p50 | run p50 | wall |
|---|---:|---:|---:|
| 1x1 | 6.790s | 0.335s | 13.844s |
| 8x2 | 3.556s | 0.346s | 7.355s |
| 6x6 | 1.828s | 0.497s | 3.883s |

## Mini soak (5 minutes)
Workload: every 10s enqueue 10x cpu(1s) + 5x io(50MB) for 5 minutes (450 jobs total).

Outcome:
- 450/450 succeeded
- retries: 0
- errors: 0
- q_wait p50: 1.340s
- q_wait p95: 2.233s

## Conclusion
Per-plugin parallelism provides clear and repeatable improvement vs serial execution.
Queue wait and end-to-end wall time drop significantly under concurrency, with expected modest per-job runtime inflation at higher I/O contention.
