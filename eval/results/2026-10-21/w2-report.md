## Pipeline `W0+W1+W2`

Solved: images with at least one successful repetition (pass@k), with its 95% Wilson interval. Per attempt: successful attempts over attempts, what one call gives. Majority: images where most repetitions succeed (sensitivity). ΔV, fix rate and ΔS summarise the successful attempts.

| Group | Images | Attempts | Solved | 95% CI (Wilson) | Per attempt | Majority | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |
|---|---:|---:|---:|---|---:|---:|---|---|---:|---|
| all | 56 | 168 | 11 (19.6%) | [11.3%, 31.8%] | 11.3% | 10.7% | 9.4% (0.1%–30.6%) | 9.7% (1.3%–63.0%) | 0 | -0.1% (-2.6%–6.2%) |
| alpine | 25 | 75 | 5 (20.0%) | [8.9%, 39.1%] | 12.0% | 12.0% | 12.8% (0.0%–12.8%) | 9.7% (0.0%–9.7%) | 0 | -2.1% (-2.1%–11.2%) |
| debian | 25 | 75 | 5 (20.0%) | [8.9%, 39.1%] | 12.0% | 12.0% | 9.4% (2.2%–32.0%) | 63.0% (2.4%–80.0%) | 0 | 0.0% (-0.1%–1.6%) |
| redhat | 1 | 3 | 1 (100.0%) | [20.7%, 100.0%] | 33.3% | 0.0% | 0.0% (0.0%–0.0%) | 3.3% (3.3%–3.3%) | 0 | -4.1% (-4.1%–-4.1%) |
| scratch | 2 | 6 | 0 (0.0%) | [0.0%, 65.8%] | 0.0% | 0.0% | – | – | 0 | – |
| ubuntu | 3 | 9 | 0 (0.0%) | [0.0%, 56.1%] | 0.0% | 0.0% | – | – | 0 | – |
| O′ end of life | 30 | 90 | 3 (10.0%) | [3.5%, 25.6%] | 4.4% | 3.3% | 59.9% (52.2%–69.9%) | 63.0% (55.4%–72.2%) | 0 | -1.6% (-4.3%–-0.1%) |
| O′ supported | 26 | 78 | 8 (30.8%) | [16.5%, 50.0%] | 19.2% | 19.2% | 2.4% (0.1%–12.8%) | 4.8% (0.2%–23.0%) | 0 | 1.6% (-2.1%–9.3%) |

| Failure | Attempts | Share of attempts |
|---|---:|---:|
| G1_parse | 0 | 0.0% |
| G2_no_invention | 0 | 0.0% |
| G3_versions_exist | 3 | 1.8% |
| G4_build | 72 | 42.9% |
| G5_behaviour | 9 | 5.4% |
| G6_no_new_severe | 38 | 22.6% |
| optimizer produced no candidate | 1 | 0.6% |

Not measured (harness errors or invalid items, excluded above): 3 attempts.

End of life (as the scanner reports it; V misses findings disclosed since): O′ is past its end of life in 42 of 95 attempts with C scanned; C moved off it in 11 (26.2% of those), and onto it from a supported O′ in 4.

### Verify and repair (W1)

167 attempts verified; the generated candidate failed a check in 40, and repair fixed 35 of those.

| Check | Generated candidate fails | Kept candidate fails |
|---|---:|---:|
| parse | 2 (1.2%) | 0 (0.0%) |
| context_copies | 0 (0.0%) | 0 (0.0%) |
| dropped_run | 0 (0.0%) | 0 (0.0%) |
| base_tag_exists | 12 (7.2%) | 1 (0.6%) |
| apt_version_exists | 8 (4.8%) | 2 (1.2%) |
| pip_version_exists | 0 (0.0%) | 0 (0.0%) |
| fix_version | 1 (0.6%) | 0 (0.0%) |
| runtime_contract | 25 (15.0%) | 4 (2.4%) |
| hadolint | 2 (1.2%) | 0 (0.0%) |

Repair rounds used: 0 round(s): 129, 1 round(s): 28, 2 round(s): 10.

### Base image candidates (W2)

101 attempts were offered a base image table.

| Final base | Attempts | Share |
|---|---:|---:|
| kept the current base | 26 | 25.7% |
| changed to a table candidate | 75 | 74.3% |
| changed to a base not in the table | 0 | 0.0% |
| unreadable | 0 | 0.0% |

The chosen base has the table's lowest V in 93 attempts (92.1%).

