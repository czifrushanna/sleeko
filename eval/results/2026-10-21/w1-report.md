## Pipeline `W0+W1`

Solved: images with at least one successful repetition (pass@k), with its 95% Wilson interval. Per attempt: successful attempts over attempts, what one call gives. Majority: images where most repetitions succeed (sensitivity). ΔV, fix rate and ΔS summarise the successful attempts.

| Group | Images | Attempts | Solved | 95% CI (Wilson) | Per attempt | Majority | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |
|---|---:|---:|---:|---|---:|---:|---|---|---:|---|
| all | 56 | 168 | 9 (16.1%) | [8.7%, 27.8%] | 7.1% | 5.4% | 11.5% (9.4%–45.0%) | 21.4% (4.9%–65.6%) | 0 | 3.7% (-2.0%–10.6%) |
| alpine | 25 | 75 | 3 (12.0%) | [4.2%, 30.0%] | 5.3% | 4.0% | 60.7% (48.7%–70.5%) | 60.3% (47.7%–70.2%) | 0 | 0.8% (-2.0%–3.4%) |
| debian | 25 | 75 | 5 (20.0%) | [8.9%, 39.1%] | 9.3% | 8.0% | 9.5% (9.4%–19.7%) | 10.3% (4.1%–56.9%) | 0 | 9.3% (5.4%–14.4%) |
| redhat | 1 | 3 | 1 (100.0%) | [20.7%, 100.0%] | 33.3% | 0.0% | 0.0% (0.0%–0.0%) | 3.3% (3.3%–3.3%) | 0 | -4.1% (-4.1%–-4.1%) |
| scratch | 2 | 6 | 0 (0.0%) | [0.0%, 65.8%] | 0.0% | 0.0% | – | – | 0 | – |
| ubuntu | 3 | 9 | 0 (0.0%) | [0.0%, 56.1%] | 0.0% | 0.0% | – | – | 0 | – |
| O′ end of life | 30 | 90 | 4 (13.3%) | [5.3%, 29.7%] | 5.6% | 3.3% | 60.7% (39.8%–60.7%) | 60.3% (32.5%–60.3%) | 0 | 3.2% (-3.1%–4.1%) |
| O′ supported | 26 | 78 | 5 (19.2%) | [8.5%, 37.9%] | 9.0% | 7.7% | 9.4% (4.8%–9.8%) | 5.4% (3.0%–45.5%) | 0 | 9.3% (-0.0%–11.8%) |

| Failure | Attempts | Share of attempts |
|---|---:|---:|
| G1_parse | 0 | 0.0% |
| G2_no_invention | 0 | 0.0% |
| G3_versions_exist | 6 | 3.6% |
| G4_build | 70 | 41.7% |
| G5_behaviour | 13 | 7.7% |
| G6_no_new_severe | 39 | 23.2% |
| optimizer produced no candidate | 5 | 3.0% |

Not measured (harness errors or invalid items, excluded above): 3 attempts.

End of life (as the scanner reports it; V misses findings disclosed since): O′ is past its end of life in 37 of 93 attempts with C scanned; C moved off it in 9 (24.3% of those), and onto it from a supported O′ in 2.

### Verify and repair (W1)

163 attempts verified; the generated candidate failed a check in 64, and repair fixed 56 of those.

| Check | Generated candidate fails | Kept candidate fails |
|---|---:|---:|
| parse | 0 (0.0%) | 0 (0.0%) |
| context_copies | 1 (0.6%) | 0 (0.0%) |
| dropped_run | 0 (0.0%) | 0 (0.0%) |
| base_tag_exists | 31 (19.0%) | 5 (3.1%) |
| apt_version_exists | 10 (6.1%) | 1 (0.6%) |
| pip_version_exists | 0 (0.0%) | 0 (0.0%) |
| fix_version | 1 (0.6%) | 1 (0.6%) |
| runtime_contract | 29 (17.8%) | 3 (1.8%) |
| hadolint | 5 (3.1%) | 0 (0.0%) |

Repair rounds used: 0 round(s): 100, 1 round(s): 45, 2 round(s): 18.

