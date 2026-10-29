Per attempt, means unless stated; tokens counted with the Qwen/Qwen3.8-27B tokenizer.

| Pipeline | Attempts | Latency mean | median | 90th pct. | Model calls | Input tokens | Output tokens |
|---|---:|---:|---:|---:|---:|---:|---:|
| W0 | 168 | 14.5 s | 7.8 s | 38.4 s | 1.00 | 2,435 | 453 |
| W0+W1 | 168 | 27.4 s | 16.3 s | 61.7 s | 1.49 | 4,057 | 652 |
| W0+W1+W2 | 168 | 26.6 s | 16.9 s | 50.7 s | 1.30 | 3,956 | 653 |
