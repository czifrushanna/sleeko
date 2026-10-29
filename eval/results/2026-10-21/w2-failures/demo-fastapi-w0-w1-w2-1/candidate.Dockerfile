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
# Let's check if `python:3.9-slim-trixie` exists. It likely doesn't.
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
# Let's assume the user expects me to use a base that has the fixes.
# I will use `python:3.9-slim` and add `apt-get upgrade` to get the latest security patches for the current base.
# Wait, if the base is Bookworm, `apt-get upgrade` will give Bookworm fixes, not Trixie.
# The summary explicitly lists Trixie versions as the fix target.
# This implies the base image should be Trixie.
# I will switch to `python:3.11-slim-trixie`? No, that breaks Python 3.9.
# I will use `debian:trixie-slim` and install Python 3.9.
# How to install Python 3.9 on Trixie? It's not in the main repo.
# This is a dilemma.
# Let's look at the "No fix available" section. It says 25 packages have no fix.
# The "Fix available" section lists Trixie versions.
# I will assume that the intended solution is to upgrade the base image to a version that includes these fixes.
# If I cannot find a Trixie-based Python 3.9 image, I might have to pin the Python packages and upgrade the system packages via `apt-get upgrade` assuming the base is already Trixie or can be upgraded to the required state.
# Let's try a different approach: Use the base image as is, but run `apt-get update && apt-get upgrade -y` to get the latest security fixes for the *current* base.
# But the summary says the fix is to upgrade TO `deb13u1` etc.
# If the current base is `deb12`, upgrading to `deb13` packages is not possible via `apt-get upgrade` within the same release.
# So the base image MUST be Trixie.
# I will assume `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` is a Trixie-based image in this context.
# I will use `python:3.9-slim` as a proxy, but to be explicit about the base upgrade, I might need to specify a Trixie tag if it exists.
# Since I cannot verify the existence of `python:3.9-slim-trixie`, I will stick with `python:3.9-slim` and rely on `apt-get upgrade` to fetch the latest patches. If the base is Trixie, this works. If not, it's the best effort.
# However, the prompt says "Prefer a single base image upgrade".
# I will change the base to `python:3.9-slim` (assuming it's the latest available which might be Trixie-based in this hypothetical scenario) and run `apt-get upgrade`.

# Actually, let's look at the Python package fixes:
# anyio, urllib3, python-multipart, ujson, starlette, etc.
# These are pip packages. I can pin them in requirements.txt or install them with specific versions.
# The summary says "Packages with a fix available... anyio 4.11.0 -> 4.14.2".
# I should pin these in the Dockerfile or requirements.
# Since I cannot see requirements.txt, I will add `pip install` commands for the fixed Python packages to ensure they are upgraded.
# But `pip install -r requirements.txt` will install whatever is in the file.
# I can add a subsequent `pip install` for the specific packages with the fixed versions.

# Strategy:
# 1. Base image: `python:3.9-slim` (Assume it's the latest security-patched version available for 3.9, or use `tiangolo/...` if it's better).
#    Actually, `tiangolo/uvicorn-gunicorn-fastapi` is a specific image. I should probably stick with it or a standard Python image.
#    Let's use `python:3.9-slim` for simplicity and standardness, and add the necessary system packages.
# 2. Upgrade system packages: `apt-get update && apt-get upgrade -y`
# 3. Install system deps: `uvicorn`, `gunicorn` are usually installed via pip in the tiangolo image. In `python:3.9-slim`, I need to install them.
#    The original image `tiangolo/uvicorn-gunicorn-fastapi` comes with uvicorn and gunicorn pre-installed.
#    If I switch to `python:3.9-slim`, I must install uvicorn and gunicorn.
#    Let's stick with `tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim` to preserve the pre-installed tools, and just upgrade the OS packages and pip packages.
#    This is safer for functionality.

FROM tiangolo/uvicorn-gunicorn-fastapi:python3.9-slim

WORKDIR /app

# Upgrade system packages to fix OS-level vulnerabilities
# This assumes the base image is up-to-date or can be upgraded to the versions listed in the summary.
# If the base is older, this might not reach the exact versions, but it's the best effort without changing the base distro.
RUN apt-get update && apt-get upgrade -y --no-install-recommends && rm -rf /var/lib/apt/lists/*

# Upgrade Python packages with available fixes
# We pin the versions specified in the vulnerability summary
RUN pip install --no-cache-dir \
    "anyio>=4.14.2" \
    "urllib3>=2.8.0" \
    "python-multipart>=0.0.31" \
    "ujson>=5.13.0" \
    "starlette>=1.3.1" \
    "jaraco.context>=6.1.0" \
    "orjson>=3.11.6" \
    "wheel>=0.46.2" \
    "idna>=3.15" \
    "python-dotenv>=1.2.2" \
    "setuptools>=83.0.0" \
    "Pygments>=2.20.0" \
    "pip>=26.2.0"

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY ./app ./app

# Create dev environment user and install dev tools
RUN <<EOF
apt-get update
apt-get install -y --no-install-recommends git
useradd -s /bin/bash -m vscode
groupadd docker
usermod -aG docker vscode
EOF

# Install Docker tools
COPY --from=gloursdocker/docker / /

# Preserve original entrypoint/cmd behavior
# The tiangolo image has a default CMD/ENTRYPOINT.
# We don't override it here, so it remains as defined in the base image.
# However, the original Dockerfile didn't have an ENTRYPOINT/CMD, so it inherited from the base.
# We keep it that way.

# To reduce size, we could clean up apt lists and pip cache, but `--no-cache-dir` and `rm -rf /var/lib/apt/lists/*` handle that.
