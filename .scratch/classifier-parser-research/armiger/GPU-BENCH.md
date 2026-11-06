# armiger-ultra: CPU vs RTX 3080 (tower, 2026-11-06)

Same 200 labeled-sample request bodies, 3 rounds each after a 20-request warm-up; server benchmark run inside each container against its own `/v1/classify`.

|                                               | CPU (stock image 3.12.2)        | GPU (RTX 3080, patched test image)  |
| --------------------------------------------- | ------------------------------- | ----------------------------------- |
| Server throughput, concurrency 1 / 4 / 8 / 16 | 76.9 / 79.1 / 81.4 / 79.9 req/s | 100.8 / 107.4 / 106.7 / 107.0 req/s |
| Median latency, sequential                    | 10.6 ms                         | 9.7 ms                              |
| In-process total per item                     | 11.2 ms                         | 6.3 ms                              |
| ONNX inference per item (1 run/item)          | 8.3 ms (74% of total)           | 3.9 ms (62%)                        |
| GPU memory                                    | —                               | ~290 MiB                            |

- Throughput is flat with concurrency on both: requests are effectively serialized (Python GIL / per-request Python work), so more client parallelism only adds queueing.
- Predictions identical: 200/200 same category, max confidence difference 0.000466.
- GPU gain is limited to ~1.3x at the server: ONNX Runtime adds 13 CPU↔GPU memcpy nodes (some ops stay on CPU), batch size is 1 per request, and HTTP/JSON + Python enrichment (~3–4 ms) is unchanged.

GPU test image (`armiger-gpu-test:local`, not upstream-supported): stock image + `onnxruntime-gpu[cuda,cudnn]==1.31.1` (replacing `onnxruntime`), every hardcoded `providers=["CPUExecutionProvider"]` in the armiger package patched to `["CUDAExecutionProvider", "CPUExecutionProvider"]`, run with `--gpus all`, `LD_LIBRARY_PATH=<site-packages>/nvidia/cu13/lib:<site-packages>/nvidia/cudnn/lib` (otherwise CUDA silently falls back to CPU), entrypoint `python -m armiger serve`. pip install needs >6 GiB container memory (OOM-killed at 6g).

Conclusion: CPU is enough. ~80 req/s ≈ 6.9M torrents/day single-instance, far above crawl rates, and the shadow action calls armiger once per unknown torrent. The GPU path needs an unofficial patched image and shares the 3080 with other workloads for a 1.3x gain.
