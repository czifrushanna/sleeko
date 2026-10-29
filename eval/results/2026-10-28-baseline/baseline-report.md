## Pipeline `Rebase`

Solved: images with at least one successful repetition (pass@k), with its 95% Wilson interval. Per attempt: successful attempts over attempts, what one call gives. Majority: images where most repetitions succeed (sensitivity). ΔV, fix rate and ΔS summarise the successful attempts.

| Group | Images | Attempts | Solved | 95% CI (Wilson) | Per attempt | Majority | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |
|---|---:|---:|---:|---|---:|---:|---|---|---:|---|
| all | 54 | 54 | 7 (13.0%) | [6.4%, 24.4%] | 13.0% | 13.0% | 0.4% (0.3%–47.3%) | 45.5% (31.0%–56.5%) | 0 | 0.8% (-0.0%–7.6%) |
| alpine | 24 | 24 | 2 (8.3%) | [2.3%, 25.8%] | 8.3% | 8.3% | 29.6% (14.8%–44.4%) | 22.7% (11.4%–34.1%) | 0 | 15.9% (14.7%–17.0%) |
| debian | 25 | 25 | 5 (20.0%) | [8.9%, 39.1%] | 20.0% | 20.0% | 0.4% (0.4%–35.3%) | 50.0% (33.3%–63.0%) | 0 | 0.0% (-0.1%–0.8%) |
| redhat | 1 | 1 | 0 (0.0%) | [0.0%, 79.3%] | 0.0% | 0.0% | – | – | 0 | – |
| scratch | 2 | 2 | 0 (0.0%) | [0.0%, 65.8%] | 0.0% | 0.0% | – | – | 0 | – |
| ubuntu | 2 | 2 | 0 (0.0%) | [0.0%, 65.8%] | 0.0% | 0.0% | – | – | 0 | – |
| O′ end of life | 29 | 29 | 1 (3.4%) | [0.6%, 17.2%] | 3.4% | 3.4% | 59.9% (59.9%–59.9%) | 63.0% (63.0%–63.0%) | 0 | -0.1% (-0.1%–-0.1%) |
| O′ supported | 25 | 25 | 6 (24.0%) | [11.5%, 43.4%] | 24.0% | 24.0% | 0.4% (0.2%–26.6%) | 39.4% (29.8%–48.9%) | 0 | 1.2% (0.2%–10.6%) |

| Failure | Attempts | Share of attempts |
|---|---:|---:|
| G1_parse | 0 | 0.0% |
| G2_no_invention | 0 | 0.0% |
| G3_versions_exist | 0 | 0.0% |
| G4_build | 3 | 5.6% |
| G5_behaviour | 3 | 5.6% |
| G6_no_new_severe | 9 | 16.7% |
| optimizer produced no candidate | 0 | 0.0% |

Not measured (harness errors or invalid items, excluded above): 3 attempts.

End of life (as the scanner reports it; V misses findings disclosed since): O′ is past its end of life in 27 of 51 attempts with C scanned; C moved off it in 4 (14.8% of those), and onto it from a supported O′ in 2.

### Base image candidates (W2)

32 attempts were offered a base image table.

| Final base | Attempts | Share |
|---|---:|---:|
| kept the current base | 6 | 18.8% |
| changed to a table candidate | 26 | 81.2% |
| changed to a base not in the table | 0 | 0.0% |
| unreadable | 0 | 0.0% |

The chosen base has the table's lowest V in 32 attempts (100.0%).

