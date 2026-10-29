## Pipeline `W0`

Solved: images with at least one successful repetition (pass@k), with its 95% Wilson interval. Per attempt: successful attempts over attempts, what one call gives. Majority: images where most repetitions succeed (sensitivity). ΔV, fix rate and ΔS summarise the successful attempts.

| Group | Images | Attempts | Solved | 95% CI (Wilson) | Per attempt | Majority | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |
|---|---:|---:|---:|---|---:|---:|---|---|---:|---|
| all | 56 | 168 | 7 (12.5%) | [6.2%, 23.6%] | 5.4% | 3.6% | 12.8% (9.5%–39.8%) | 10.3% (5.4%–60.3%) | 0 | 4.1% (1.6%–14.4%) |
| alpine | 25 | 75 | 2 (8.0%) | [2.2%, 25.0%] | 4.0% | 4.0% | 60.7% (36.8%–60.7%) | 60.3% (35.0%–60.3%) | 0 | 3.2% (0.8%–3.7%) |
| debian | 25 | 75 | 5 (20.0%) | [8.9%, 39.1%] | 8.0% | 4.0% | 9.8% (9.4%–24.5%) | 7.8% (3.4%–26.9%) | 0 | 12.7% (4.0%–14.4%) |
| redhat | 1 | 3 | 0 (0.0%) | [0.0%, 79.3%] | 0.0% | 0.0% | – | – | 0 | – |
| scratch | 2 | 6 | 0 (0.0%) | [0.0%, 65.8%] | 0.0% | 0.0% | – | – | 0 | – |
| ubuntu | 3 | 9 | 0 (0.0%) | [0.0%, 56.1%] | 0.0% | 0.0% | – | – | 0 | – |
| O′ end of life | 30 | 90 | 3 (10.0%) | [3.5%, 25.6%] | 4.4% | 3.3% | 50.3% (37.2%–60.7%) | 46.4% (26.9%–60.3%) | 0 | 3.7% (0.4%–13.1%) |
| O′ supported | 26 | 78 | 4 (15.4%) | [6.2%, 33.5%] | 6.4% | 3.8% | 9.5% (9.4%–10.2%) | 5.4% (2.7%–9.7%) | 0 | 11.0% (1.6%–14.4%) |

| Failure | Attempts | Share of attempts |
|---|---:|---:|
| G1_parse | 0 | 0.0% |
| G2_no_invention | 2 | 1.2% |
| G3_versions_exist | 39 | 23.2% |
| G4_build | 89 | 53.0% |
| G5_behaviour | 19 | 11.3% |
| G6_no_new_severe | 35 | 20.8% |
| optimizer produced no candidate | 2 | 1.2% |

Not measured (harness errors or invalid items, excluded above): 3 attempts.

End of life (as the scanner reports it; V misses findings disclosed since): O′ is past its end of life in 28 of 75 attempts with C scanned; C moved off it in 8 (28.6% of those), and onto it from a supported O′ in 2.

