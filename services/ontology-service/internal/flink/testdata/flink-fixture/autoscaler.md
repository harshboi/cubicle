---
title: Autoscaler
---

# Autoscaler

The Kubernetes operator autoscaler adjusts job resources from observed load.

## Stable recommendations

FLINK-39743 documents the expected behavior for stable target utilization
recommendations. The operator should avoid noisy action proposals when the
recommended parallelism does not materially change.
