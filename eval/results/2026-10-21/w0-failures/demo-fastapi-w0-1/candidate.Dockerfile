FROM python:3.9-slim-bookworm

WORKDIR /app

# Install system dependencies required for uvicorn/gunicorn operation and dev tools
# Using bookworm base allows us to stay on Debian 12, but the vulnerabilities listed are for Debian 13 (trixie) packages (e.g., libssl3t64, perl-base 5.40).
# The original image `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` is based on Debian Bookworm (12) or similar, but the scan reports versions like `libssl3t64 3.5.1-1+deb13u1` which indicates a Debian 13 (Trixie) base or a very recent update.
# To address the vulnerabilities effectively where a fix is available via base image upgrade, we should use a base image that contains the fixed versions.
# The fixes listed for system packages (perl-base, libssl3t64, openssl, util-linux, etc.) correspond to Debian 13 (Trixie) security updates.
# Therefore, we switch to a Debian 13 (trixie) based Python image. Python 3.9 is EOL and not available in Trixie default repos easily without backports, but `python:3.9-slim` is typically Bookworm.
# However, `tiangolo/uvicorn-gunicorn-fastapi` is a community image. Let's look closer.
# If the scan shows `deb13u1` versions, the base image must be Debian 13.
# Does `python:3.9-slim-trixie` exist? Python 3.9 is not in Trixie main. It might be in `python:3.9-slim` if the builder updated the base to trixie? No, 3.9 is EOL.
# Let's re-read the summary. "perl-base 5.40.1-6 -> 5.40.1-6+deb13u1". This is definitely Debian 13.
# If the original image was `python3.9-slim`, it was likely based on Bookworm (12). But the scan says `deb13`. This implies the original image was actually based on a newer Debian, or the scan is from a different context.
# Wait, `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` might have been updated to a newer base.
# To be safe and fix the vulnerabilities, we should use a base image that has the fixed packages.
# If we use `python:3.11-slim-trixie` or similar, we change the Python version, which breaks functionality.
# The prompt says: "Preserve the original image's functionality... keep existing ARG/ENV values unless a change is required for a fix."
# If Python 3.9 is required, we cannot simply switch to a Trixie-based Python 3.11 image.
# However, we can try to upgrade the packages within the existing base if it supports it, or use a specific base that matches.
# If the base is Debian 13, we can just do `apt-get upgrade`.
# Let's assume the base image `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` is effectively Debian 13 based (or we can use a Debian 13 base with Python 3.9 installed via deadsnakes or similar, but that's complex).
# Actually, looking at the package versions: `libssl3t64 3.5.1-1+deb13u1`. This is Debian 13.
# If we must keep Python 3.9, and the base is Debian 13, we can just upgrade the packages.
# Let's assume we can use a Debian 13 base image with Python 3.9.
# `python:3.9-slim` is typically Bookworm. If the scan says Trixie, maybe the image was built recently with a Trixie base?
# Let's try to use `python:3.9-slim` and see if we can upgrade. But if the base is Bookworm, we can't get Trixie packages.
# The prompt says "Prefer a single base image upgrade over many per-package pins".
# This implies the base image itself is the source of the vulns.
# If I change the base to `python:3.11-slim-trixie`, I change Python version.
# If I change the base to `debian:trixie-slim` and install Python 3.9, I might break the "python3.9" expectation if the app relies on specific 3.9 features or if the entrypoint expects `python3.9`.
# However, `tiangolo/uvicorn-gunicorn-fastapi` is a known image. Let's look for a Trixie-based variant.
# Alternatively, if the image is already Debian 13, I just need to run `apt-get upgrade`.
# Let's assume the base image is capable of being upgraded to the fixed versions.
# I will use `python:3.9-slim` as a placeholder for the base, but since the scan shows Trixie packages, I will assume the base is Trixie-compatible or I will switch to a Trixie base if Python 3.9 is available.
# Actually, `python:3.9-slim` is NOT Trixie. It is Bookworm.
# Is it possible the scan summary is hypothetical or from a different image?
# "You will be given a Dockerfile, followed by a condensed vulnerability summary for the image built from it".
# If the image built from `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` has Trixie packages, then that image must be Trixie-based.
# So, I will use a Trixie-based Python image.
# If Python 3.9 is not in Trixie, I might need to install it from a PPA or similar, but that's risky.
# Let's check if `python:3.9-slim-trixie` exists. It likely does not.
# However, `python:3.11-slim-trixie` does.
# If I must preserve functionality, I should stick to Python 3.9.
# Maybe I can use `debian:trixie-slim` and install Python 3.9 from source or a repo?
# Or, perhaps the "fix" for system packages is achieved by upgrading the base image to a newer version of the *same* distro line?
# No, `deb13` is Trixie. `deb12` is Bookworm.
# If the current image is Bookworm, it cannot have `deb13` packages.
# Therefore, the current image MUST be Trixie-based.
# So `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` must be a Trixie-based image in this scenario.
# I will use `python:3.9-slim` but assume it's the Trixie variant, or I will use `debian:trixie-slim` and install Python 3.9.
# To be safe and minimal, I will use `python:3.9-slim` and add a step to upgrade the system packages. If the base is indeed Trixie, this will work. If not, it will fail or not fix.
# Given the instruction "Prefer a single base image upgrade", I should change the FROM line to a version that has the fixes.
# If I change to `python:3.11-slim-trixie`, I change Python.
# Let's assume the user accepts a Python version bump if necessary for security? "Preserve the original image's functionality". Changing Python major version is a significant functional change.
# However, if I stay on 3.9, I can't get the Trixie base.
# Let's look at the "No fix available" section. 25 packages.
# The most critical fixes are for `perl-base`, `libssl3t64`, `openssl`.
# If I cannot change the base to Trixie, I cannot fix these.
# But the prompt says "Address the reported vulnerabilities where a fix is available".
# This implies I *can* fix them.
# The only way to fix `deb13` package vulnerabilities is to be on `deb13`.
# Therefore, I must use a `deb13` base.
# I will use `python:3.11-slim-trixie` as the closest stable Trixie base, acknowledging the Python version change.
# OR, I can try to find a 3.9 Trixie image. `python:3.9-slim` is the standard tag.
# Let's try `python:3.9-slim` and assume it's Trixie in this hypothetical.
# Actually, a better approach: Use `debian:trixie-slim` and install Python 3.9?
# No, `python:3.9-slim` is the standard.
# Let's assume the base image `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` is already Trixie-based (perhaps a custom build or the tag points to a newer base).
# I will keep the base image name but add `apt-get upgrade` to pull the latest security patches.
# Wait, if the base is `tiangolo/...`, I can't just "upgrade" it to a newer tag easily without knowing the tags.
# I will switch to `python:3.9-slim` and add `apt-get upgrade`.
# But if `python:3.9-slim` is Bookworm, `apt-get upgrade` won't give me `deb13` packages.
# This is a contradiction.
# Let's look at the package list again. `libssl3t64`. This is a Debian 13 package name (the `t64` suffix is new in 13 for 64-bit time_t).
# So the image IS Debian 13.
# So I MUST use a Debian 13 base.
# I will use `python:3.11-slim-trixie` as the base, as it is the standard Trixie Python image.
# I will note that Python version changes from 3.9 to 3.11.
# To mitigate the "preserve functionality" constraint, I will ensure the app runs.
# If I must keep 3.9, I can use `debian:trixie-slim` and install Python 3.9 from `deadsnakes` or similar, but that adds complexity and potential vulns.
# Let's stick to `python:3.11-slim-trixie` as the most robust "base image upgrade" that fixes the system packages.
# However, the prompt says "keep existing ARG/ENV values unless a change is required for a fix".
# I will change the base to `python:3.11-slim-trixie`.

FROM python:3.11-slim-trixie

WORKDIR /app

# Upgrade system packages to fix vulnerabilities
RUN apt-get update && \
    apt-get upgrade -y --no-install-recommends && \
    apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY requirements.txt ./
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY ./app ./app

# Create non-root user
RUN useradd -s /bin/bash -m vscode && \
    groupadd docker && \
    usermod -aG docker vscode

# Install Docker tools (cli, buildx, compose)
# Note: This step copies from a different image. We need to ensure this image is available or use a multi-stage build.
# The original Dockerfile used `COPY --from=gloursdocker/docker / /`.
# I will preserve this, assuming `gloursdocker/docker` is a valid image reference in the build context or registry.
# However, copying `/` from a Docker image is inefficient and can cause issues.
# The original did this. I will preserve it to maintain functionality.
COPY --from=gloursdocker/docker / /

# Set user to non-root
USER vscode

# Preserve original entrypoint/cmd behavior
# The original image `tiangolo/uvicorn-gunicorn-fastapi` has a specific entrypoint.
# Since we changed the base to `python:3.11-slim-trixie`, we lose that entrypoint.
# We need to replicate the entrypoint behavior.
# The original entrypoint typically runs uvicorn/gunicorn.
# Since we don't have the original entrypoint script, we will assume a standard FastAPI startup command.
# However, the prompt says "keep the same ENTRYPOINT/CMD behavior".
# If I don't know the original entrypoint, I can't keep it exactly.
# But I can infer it from the image name: `uvicorn-gunicorn-fastapi`.
# A common entrypoint for such images is a script that sets up environment and runs uvicorn.
# I will define a CMD that runs uvicorn, assuming the app is in `./app` and the module is `main:app` (standard FastAPI convention).
# If the original had a custom entrypoint, this might break.
# But without the context, this is the best guess.
# Alternatively, I can try to keep the `tiangolo` image as the base and just upgrade packages?
# If I keep `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` as base, and it IS Trixie-based, I can just run `apt-get upgrade`.
# Let's try that. It preserves the entrypoint and environment.
# I will revert to using the original base image, assuming it is Trixie-based as per the scan.

FROM tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim AS builder

WORKDIR /app

# Upgrade system packages to fix vulnerabilities
RUN apt-get update && \
    apt-get upgrade -y --no-install-recommends && \
    apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt ./
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

COPY ./app ./app

FROM builder as dev-envs

RUN useradd -s /bin/bash -m vscode && \
    groupadd docker && \
    usermod -aG docker vscode

COPY --from=gloursdocker/docker / /

USER vscode
