FROM rust:1.27.2-slim as builder
WORKDIR /root
COPY . /root
RUN cargo build --release

FROM ubuntu:22.04
EXPOSE 53/udp
WORKDIR /root
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /root/target/release/updns /root/updns
ENV LOG=info,warn,error
RUN useradd --create-home --shell /usr/sbin/nologin appuser && chown -R appuser:appuser /root
USER appuser
CMD ["./updns"]
